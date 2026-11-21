#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
source scripts/release-common.sh
: "${GITHUB_RUN_ID:?}" "${GITHUB_RUN_ATTEMPT:?}"
guard_release
jq -e '.review_status == "approved"' licenses/distroless/inventory.json >/dev/null
(cd dist && sha256sum --check checksums.txt)
source_archive="obsidian-livesync-portal_${tag#v}_source.tar.gz"
[[ -f "dist/$source_archive" ]]
grep -Fq "  $source_archive" dist/checksums.txt
verify_release_artifacts
cosign sign-blob --yes --bundle "dist/$source_archive.sigstore.json" "dist/$source_archive"
bash scripts/publish-source.sh
export IMAGE="$image"
export IMAGE_TAG="candidate-$GITHUB_SHA-$GITHUB_RUN_ID-$GITHUB_RUN_ATTEMPT"
export RELEASE_OWNER=${GITHUB_REPOSITORY_OWNER:-${GITHUB_REPOSITORY%/*}}
export RELEASE_REPO=${GITHUB_REPOSITORY#*/}
unset GOARCH GOOS
# Only this invocation builds/pushes images; the source is already public and verified.
mkdir -p build
mv dist build/release-artifacts
goreleaser release --clean --skip=archive,sbom,sign
mv dist build/ko-release
mv build/release-artifacts dist
digest=$(jq -er '[.[] | select(.type == "Docker Manifest" and .extra.ID == "portal")] | if length == 1 then .[0].extra.Digest else error("Expected one ko index") end' build/ko-release/artifacts.json)
[[ "$digest" =~ ^sha256:[a-f0-9]{64}$ ]]
crane manifest "$image@$digest" > build/manifest-index.json
[[ "sha256:$(sha256sum build/manifest-index.json | awk '{print $1}')" == "$digest" ]]
jq -e '(.manifests | length) == 2 and ([.manifests[].platform.architecture] | sort) == ["amd64", "arm64"] and all(.manifests[]; .platform.os == "linux")' build/manifest-index.json >/dev/null
jq -n --arg digest "$digest" '{"containerimage.digest":$digest}' > build/manifest.json
for arch in amd64 arm64; do
  child=$(jq -er --arg arch "$arch" '.manifests[] | select(.platform.architecture == $arch) | .digest' build/manifest-index.json)
  [[ "$child" =~ ^sha256:[a-f0-9]{64}$ ]]
  jq -n --arg digest "$child" '{"containerimage.digest":$digest}' > "build/image-$arch.json"
  docker pull --platform "linux/$arch" "$image@$child"
  bash scripts/image-smoke.sh "$image@$child" "linux/$arch"
  crane export "$image@$child" "build/image-$arch.tar"
done
verify_release_artifacts --images --image-profile distroless --image-tar build/image-amd64.tar --image-tar build/image-arm64.tar
printf '%s@%s\n' "$image" "$digest" > dist/image-digest.txt
base=$(jq -er '.image + "@" + .index' licenses/distroless/inventory.json)
jq -n --arg revision "$GITHUB_SHA" --arg repo "$GITHUB_REPOSITORY" --arg base "$base" \
  --arg source "$source_url" --arg source_hash "$(sha256sum "dist/$source_archive" | awk '{print $1}')" \
  --arg workflow "https://github.com/$GITHUB_REPOSITORY/.github/workflows/release.yml@refs/tags/$tag" \
  --arg invocation "https://github.com/$GITHUB_REPOSITORY/actions/runs/$GITHUB_RUN_ID/attempts/$GITHUB_RUN_ATTEMPT" \
  '{buildDefinition:{buildType:"https://goreleaser.com/customization/package/ko/",externalParameters:{go:"1.27.4",goreleaser:"2.20.3",ko:"0.19.1",base_image:$base},resolvedDependencies:[{uri:("git+https://github.com/"+$repo),digest:{gitCommit:$revision}},{uri:$source,digest:{sha256:$source_hash}}]},runDetails:{builder:{id:$workflow},metadata:{invocationId:$invocation}}}' > build/provenance.json
for ref in "$image@$(metadata_digest build/image-amd64.json)" "$image@$(metadata_digest build/image-arm64.json)" "$image@$digest"; do
  cosign sign --yes "$ref"
  cosign attest --yes --type https://slsa.dev/provenance/v1 --predicate build/provenance.json "$ref"
done
cosign sign-blob --yes --bundle dist/checksums.txt.sigstore.json dist/checksums.txt
cosign sign-blob --yes --bundle dist/image-digest.txt.sigstore.json dist/image-digest.txt
