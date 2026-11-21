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
if [[ -z "$id" ]]; then
  gh release create "$tag" --repo "$GITHUB_REPOSITORY" --draft --target "$GITHUB_SHA" --title "$tag" --notes "Built from commit $GITHUB_SHA. Corresponding Source: $source_url . Verify the signed checksums and immutable image digest."
  id=$(draft_id)
fi
[[ "$id" =~ ^[0-9]+$ ]]
# A failed attempt may leave a draft; replace its assets, never a finalized release.
draft_id >/dev/null
gh api --method PATCH "repos/$GITHUB_REPOSITORY/releases/$id" -F draft=true \
  -f "target_commitish=$GITHUB_SHA" -f "name=$tag" \
  -f "body=Built from commit $GITHUB_SHA. Corresponding Source: $source_url . Verify the signed checksums and immutable image digest."
old_assets=$(gh api --paginate "repos/$GITHUB_REPOSITORY/releases/$id/assets" --jq '.[].id')
while IFS= read -r asset; do
  [[ -z "$asset" ]] || gh api --method DELETE "repos/$GITHUB_REPOSITORY/releases/assets/$asset"
done <<< "$old_assets"
mapfile -t files < <(awk '{print "dist/" $2}' dist/checksums.txt)
gh release upload "$tag" --repo "$GITHUB_REPOSITORY" "${files[@]}" \
  dist/checksums.txt dist/checksums.txt.sigstore.json dist/image-digest.txt dist/image-digest.txt.sigstore.json
gh release upload "$tag" --repo "$GITHUB_REPOSITORY" "dist/obsidian-livesync-portal_${tag#v}_source.tar.gz.sigstore.json"
download=$(mktemp -d)
trap 'rm -f "$download"/*; rmdir "$download"' EXIT
gh release download "$tag" --repo "$GITHUB_REPOSITORY" --dir "$download"
(cd "$download" && verify_assets)
guard_release
crane copy "$image@$digest" "$image:$tag"
[[ "$(crane digest "$image:$tag")" == "$digest" ]]
guard_release
verify_public_source
gh api --method PATCH "repos/$GITHUB_REPOSITORY/releases/$id" -F draft=false
