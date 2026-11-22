#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
# REST operations stay bound to a validated release ID, never a draft tag lookup.
# https://docs.github.com/en/rest/releases/releases#update-a-release
validate_release() {
  local metadata=$1 release_tag=$2 id=$3 draft=$4 title=$5 body=$6
  jq -e --arg tag "$release_tag" --arg id "$id" --arg sha "$GITHUB_SHA" \
    --argjson draft "$draft" --arg title "$title" --arg body "$body" --arg repo "$GITHUB_REPOSITORY" '
    (.id | type == "number" and . > 0 and floor == .) and
    ($id == "" or (.id | tostring) == $id) and .tag_name == $tag and
    .target_commitish == $sha and .draft == $draft and .name == $title and .body == $body and
    .upload_url == ("https://uploads.github.com/repos/" + $repo + "/releases/" + (.id | tostring) + "/assets{?name,label}")
  ' <<< "$metadata" >/dev/null || { echo "Invalid release identity/state for $release_tag (expected ID $id); refusing further operations" >&2; return 1; }
}

get_release() {
  local metadata
  metadata=$(gh api "repos/$GITHUB_REPOSITORY/releases/$1") || return 1
  validate_release "$metadata" "$2" "$1" "$3" "$4" "$5" || return 1
  printf '%s\n' "$metadata"
}

create_release() {
  local metadata
  metadata=$(gh api --method POST "repos/$GITHUB_REPOSITORY/releases" \
    -f "tag_name=$1" -f "target_commitish=$GITHUB_SHA" -f "name=$2" -f "body=$3" \
    -F draft=true -f make_latest=false) || return 1
  validate_release "$metadata" "$1" '' true "$2" "$3" || return 1
  # Use the returned ID even if list/tag lookup has not become consistent yet.
  get_release "$(jq -r '.id' <<< "$metadata")" "$1" true "$2" "$3"
}

update_release() {
  local id=$1 release_tag=$2 draft=$3 title=$4 body=$5 metadata
  get_release "$id" "$release_tag" true "$title" "$body" >/dev/null || return 1
  # draft is boolean; make_latest is deliberately a string per GitHub's contract.
  metadata=$(gh api --method PATCH "repos/$GITHUB_REPOSITORY/releases/$id" \
    -f "tag_name=$release_tag" -f "target_commitish=$GITHUB_SHA" -f "name=$title" -f "body=$body" \
    -F "draft=$draft" -f "make_latest=${6:-false}") || return 1
  validate_release "$metadata" "$release_tag" "$id" "$draft" "$title" "$body" || return 1
  get_release "$id" "$release_tag" "$draft" "$title" "$body"
}

release_asset() {
  local metadata=$1 file=$2 destination=$3 name size assets asset count id upload encoded mime result
  name=$(basename "$file")
  [[ "$name" =~ ^[a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?$ && ${#name} -le 255 ]] || return 1
  [[ "$file" == "dist/$name" && -f "$file" && ! -L "$file" ]] || return 1
  size=$(wc -c < "$file")
  [[ "$size" -gt 0 && "$size" -lt 2147483648 ]] || return 1
  id=$(jq -er '.id' <<< "$metadata") || return 1
  assets=$(gh api --paginate "repos/$GITHUB_REPOSITORY/releases/$id/assets" --jq '.[]') || return 1
  asset=$(jq -cs --arg name "$name" '[.[] | select(.name == $name)]' <<< "$assets") || return 1
  count=$(jq 'length' <<< "$asset")
  if [[ "$count" == 0 ]]; then
    [[ "$(jq -r '.draft' <<< "$metadata")" == true ]] || { echo "Missing immutable published asset: $name" >&2; return 1; }
    upload=$(jq -er '.upload_url' <<< "$metadata") || return 1
    upload=${upload%\{*}
    encoded=$(jq -rn --arg name "$name" '$name | @uri')
    case "$name" in
      *.tar.gz) mime=application/gzip;;
      *.json) mime=application/json;;
      *.txt) mime=text/plain;;
      *) mime=application/octet-stream;;
    esac
    result=$(gh api --method POST "$upload?name=$encoded" -H "Content-Type: $mime" --input "$file") || return 1
    asset="[$result]"
  fi
  jq -e --arg name "$name" --argjson size "$size" --arg mode "${4:-exact}" '
    length == 1 and (.[0] | .name == $name and
    (.size == $size or ($mode == "verified-bundle" and (.size | type == "number" and . > 0 and . < 2147483648))) and .state == "uploaded" and
    (.id | type == "number" and . > 0 and floor == .))' <<< "$asset" >/dev/null || return 1
  id=$(jq -r '.[0].id' <<< "$asset")
  gh api "repos/$GITHUB_REPOSITORY/releases/assets/$id" -H 'Accept: application/octet-stream' > "$destination/$name" || return 1
  # No clobber/delete: a retry reuses exact bytes or fails for explicit review.
  # A pre-existing source signature may differ (keyless signing is nondeterministic);
  # publish-source verifies that retained bundle against the exact source bytes.
  if [[ "${4:-exact}" == exact ]]; then
    cmp "$file" "$destination/$name"
  else
    [[ "$4" == verified-bundle && "$name" == *.sigstore.json ]]
  fi
}

verify_release_asset_set() {
  local id=$1 assets expected
  shift
  expected=$(printf '%s\n' "$@" | sed 's|^dist/||' | jq -Rsc 'split("\n")[:-1] | sort') || return 1
  assets=$(gh api --paginate "repos/$GITHUB_REPOSITORY/releases/$id/assets" --jq '.[].name') || return 1
  jq -e -Rs --argjson expected "$expected" '(split("\n")[:-1] | sort) == $expected' <<< "$assets" >/dev/null || {
    echo 'Release contains unexpected or missing assets; manual review required' >&2; return 1;
  }
}
