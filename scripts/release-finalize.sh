#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
source scripts/release-common.sh
guard_release
verify_release_artifacts --images --image-profile distroless --image-tar build/image-amd64.tar --image-tar build/image-arm64.tar
identity="https://github.com/$GITHUB_REPOSITORY/.github/workflows/release.yml@refs/tags/$tag"
issuer=https://token.actions.githubusercontent.com
digest=$(metadata_digest build/manifest.json)
[[ "$(cat dist/image-digest.txt)" == "$image@$digest" ]]
verify_assets() {
  cosign verify-blob --bundle checksums.txt.sigstore.json --certificate-identity "$identity" --certificate-oidc-issuer "$issuer" checksums.txt
  sha256sum --check checksums.txt
  cosign verify-blob --bundle image-digest.txt.sigstore.json --certificate-identity "$identity" --certificate-oidc-issuer "$issuer" image-digest.txt
  [[ "$(cat image-digest.txt)" == "$image@$digest" ]]
}
(cd dist && verify_assets)
for file in build/image-amd64.json build/image-arm64.json build/manifest.json; do
  ref=$(metadata_digest "$file")
  cosign verify --certificate-identity "$identity" --certificate-oidc-issuer "$issuer" "$image@$ref"
  cosign verify-attestation --type https://slsa.dev/provenance/v1 --certificate-identity "$identity" --certificate-oidc-issuer "$issuer" "$image@$ref"
done
bash scripts/publish-source.sh

id=$(draft_id)
body="Built from commit $GITHUB_SHA. Corresponding Source: $source_url . Verify the signed checksums and immutable image digest."
if [[ -z "$id" ]]; then
  metadata=$(create_release "$tag" "$tag" "$body")
  id=$(jq -er '.id' <<< "$metadata")
fi
[[ "$id" =~ ^[0-9]+$ ]]
metadata=$(update_release "$id" "$tag" true "$tag" "$body")
mapfile -t files < <(awk '{print "dist/" $2}' dist/checksums.txt)
download=$(mktemp -d)
trap 'rm -f "$download"/*; rmdir "$download"' EXIT
for file in "${files[@]}" dist/checksums.txt dist/checksums.txt.sigstore.json \
  dist/image-digest.txt dist/image-digest.txt.sigstore.json "dist/obsidian-livesync-portal_${tag#v}_source.tar.gz.sigstore.json"; do
  # Recheck identity/state before every upload, including on partial retries.
  metadata=$(get_release "$id" "$tag" true "$tag" "$body")
  release_asset "$metadata" "$file" "$download"
done
(cd "$download" && verify_assets)
verify_release_asset_set "$id" "${files[@]}" dist/checksums.txt dist/checksums.txt.sigstore.json \
  dist/image-digest.txt dist/image-digest.txt.sigstore.json "dist/obsidian-livesync-portal_${tag#v}_source.tar.gz.sigstore.json"
guard_release
crane copy "$image@$digest" "$image:$tag"
[[ "$(crane digest "$image:$tag")" == "$digest" ]]
guard_release
verify_public_source
update_release "$id" "$tag" false "$tag" "$body" true >/dev/null
