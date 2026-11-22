#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
source scripts/release-common.sh
guard_release
# Source publication intentionally checks archives/source before image exports exist.
# shellcheck disable=SC2119
verify_release_artifacts
export SOURCE_RELEASE_TAG="source-$GITHUB_SHA"
source_name="obsidian-livesync-portal_${tag#v}_source.tar.gz"
bundle_name="$source_name.sigstore.json"
identity="https://github.com/$GITHUB_REPOSITORY/.github/workflows/release.yml@refs/tags/$tag"
issuer=https://token.actions.githubusercontent.com
cosign verify-blob --bundle "dist/$bundle_name" --certificate-identity "$identity" --certificate-oidc-issuer "$issuer" "dist/$source_name"
source_metadata() {
  gh api --paginate "repos/$GITHUB_REPOSITORY/releases?per_page=100" --jq '.[] | select(.tag_name == env.SOURCE_RELEASE_TAG or (.draft and .name == ("Corresponding Source " + env.GITHUB_SHA))) | {id, draft}'
}
metadata=$(source_metadata)
title="Corresponding Source $GITHUB_SHA"
body="Exact build source for $GITHUB_SHA. No application binaries are published in this source-only release."
if [[ -z "$metadata" ]]; then
  metadata=$(create_release "$SOURCE_RELEASE_TAG" "$title" "$body")
fi
jq -e -s 'length == 1' <<< "$metadata" >/dev/null
id=$(jq -er '.id' <<< "$metadata")
[[ "$id" =~ ^[0-9]+$ ]]
metadata=$(get_release "$id" "$SOURCE_RELEASE_TAG" "$(jq -r '.draft' <<< "$metadata")" "$title" "$body")
download=$(mktemp -d)
trap 'rm -f "$download/$source_name" "$download/$bundle_name" "$download/public-source.tar.gz" "$download/public-source.sigstore.json"; rmdir "$download"' EXIT
release_asset "$metadata" "dist/$source_name" "$download"
release_asset "$metadata" "dist/$bundle_name" "$download" verified-bundle
# Existing source assets are immutable: reuse identical valid assets, never clobber them.
cmp "dist/$source_name" "$download/$source_name"
cosign verify-blob --bundle "$download/$bundle_name" --certificate-identity "$identity" --certificate-oidc-issuer "$issuer" "$download/$source_name"
guard_release
if [[ "$(jq -r '.draft' <<< "$metadata")" == true ]]; then
  update_release "$id" "$SOURCE_RELEASE_TAG" false "$title" "$body" >/dev/null
fi
# No token or curl configuration: prove the exact source bytes are publicly reachable.
curl --disable --fail --location --proto '=https' --proto-redir '=https' --retry 5 --retry-all-errors --retry-delay 2 \
  --output "$download/public-source.tar.gz" "$source_url"
cmp "dist/$source_name" "$download/public-source.tar.gz"
curl --disable --fail --location --proto '=https' --proto-redir '=https' --retry 5 --retry-all-errors --retry-delay 2 \
  --output "$download/public-source.sigstore.json" "$source_url.sigstore.json"
cosign verify-blob --bundle "$download/public-source.sigstore.json" --certificate-identity "$identity" --certificate-oidc-issuer "$issuer" "$download/public-source.tar.gz"
