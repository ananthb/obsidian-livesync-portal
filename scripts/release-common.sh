#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
set -euo pipefail
: "${GITHUB_REPOSITORY:?}" "${GITHUB_SHA:?}" "${GITHUB_REF_NAME:?}"
tag=$GITHUB_REF_NAME
[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?$ && ${#tag} -le 128 ]]
[[ "$GITHUB_SHA" =~ ^[a-f0-9]{40}$ ]]
export image="ghcr.io/${GITHUB_REPOSITORY,,}"
export source_url="https://github.com/$GITHUB_REPOSITORY/releases/download/source-$GITHUB_SHA/obsidian-livesync-portal_${tag#v}_source.tar.gz"

assert_tag_commit() {
  local object kind sha
  object=$(gh api "repos/$GITHUB_REPOSITORY/git/ref/tags/$tag") || return 1
  for _ in {1..8}; do
    kind=$(jq -er '.object.type' <<< "$object") || return 1
    sha=$(jq -er '.object.sha' <<< "$object") || return 1
    if [[ "$kind" == commit ]]; then
      [[ "$sha" == "$GITHUB_SHA" ]] || { echo 'Release tag moved away from the checked-out commit' >&2; return 1; }
      return
    fi
    [[ "$kind" == tag ]] || return 1
    object=$(gh api "repos/$GITHUB_REPOSITORY/git/tags/$sha") || return 1
  done
  return 1
}

draft_id() {
  local release
  release=$(gh api --paginate "repos/$GITHUB_REPOSITORY/releases?per_page=100" --jq '.[] | select(.tag_name == env.GITHUB_REF_NAME) | {id, draft}') || return 1
  if [[ -n "$release" ]]; then
    jq -e -s 'length == 1 and .[0].draft == true' <<< "$release" >/dev/null || { echo 'Refusing to replace a finalized release' >&2; return 1; }
    jq -er '.id' <<< "$release"
  fi
}

guard_release() {
  python3 scripts/license-provenance.py
  assert_tag_commit
  draft_id >/dev/null
}

verify_release_artifacts() {
  python3 scripts/license-provenance.py --release \
    --archive "dist/obsidian-livesync-portal_${tag#v}_linux_amd64.tar.gz" \
    --archive "dist/obsidian-livesync-portal_${tag#v}_linux_arm64.tar.gz" \
    --source-archive "dist/obsidian-livesync-portal_${tag#v}_source.tar.gz" \
    --revision "$GITHUB_SHA" --source-url "$source_url" "$@"
}

verify_public_source() {
  local expected actual
  expected=$(sha256sum "dist/obsidian-livesync-portal_${tag#v}_source.tar.gz" | awk '{print $1}') || return 1
  actual=$(curl --disable --fail --location --proto '=https' --proto-redir '=https' --retry 5 --retry-all-errors --retry-delay 2 \
    "$source_url" | sha256sum | awk '{print $1}') || return 1
  [[ "$actual" == "$expected" ]]
}

metadata_digest() {
  local digest
  digest=$(jq -er '."containerimage.digest" // ."containerimage.descriptor".digest' "$1") || return 1
  [[ "$digest" =~ ^sha256:[a-f0-9]{64}$ ]] || return 1
  printf '%s\n' "$digest"
}

if [[ "${1:-}" == guard ]]; then guard_release; fi
