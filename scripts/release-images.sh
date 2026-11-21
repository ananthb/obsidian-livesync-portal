#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
source scripts/release-common.sh
: "${GITHUB_RUN_ID:?}" "${GITHUB_RUN_ATTEMPT:?}"
guard_release
candidate="candidate-$GITHUB_SHA-$GITHUB_RUN_ID-$GITHUB_RUN_ATTEMPT"
(cd dist && sha256sum --check checksums.txt)
source_archive="obsidian-livesync-portal_${tag#v}_source.tar.gz"
[[ -f "dist/$source_archive" ]]
grep -Fq "  $source_archive" dist/checksums.txt
verify_release_artifacts
cosign sign-blob --yes --bundle "dist/$source_archive.sigstore.json" "dist/$source_archive"
bash scripts/publish-source.sh
mkdir -p build
container=''
trap 'if [[ -n "$container" ]]; then docker rm "$container"; fi' EXIT

for arch in amd64 arm64; do
  dir="build/image-$GITHUB_SHA-$GITHUB_RUN_ID-$GITHUB_RUN_ATTEMPT-$arch"
  mkdir "$dir"
  tar -xzf "dist/obsidian-livesync-portal_${tag#v}_linux_${arch}.tar.gz" -C "$dir"
  cp Dockerfile.release "$dir/Dockerfile"
  docker buildx build --platform "linux/$arch" \
    --label "org.opencontainers.image.source=https://github.com/$GITHUB_REPOSITORY" \
    --label "org.opencontainers.image.revision=$GITHUB_SHA" \
    --label "org.opencontainers.image.documentation=$source_url" \
    --label org.opencontainers.image.licenses=AGPL-3.0-only \
    --push --provenance=mode=max --sbom=true --metadata-file "build/image-$arch.json" \
    -t "$image:$candidate-$arch" "$dir"
  digest=$(metadata_digest "build/image-$arch.json")
  docker pull --platform "linux/$arch" "$image@$digest"
  bash scripts/image-smoke.sh "$image@$digest" "linux/$arch"
  container=$(docker create --platform "linux/$arch" "$image@$digest")
  [[ "$container" =~ ^[a-f0-9]{64}$ ]]
  docker export --output "build/image-$arch.tar" "$container"
  docker rm "$container"
  container=''
done
verify_release_artifacts --images --image-tar build/image-amd64.tar --image-tar build/image-arm64.tar

amd64=$(metadata_digest build/image-amd64.json)
arm64=$(metadata_digest build/image-arm64.json)
docker buildx imagetools create --metadata-file build/manifest.json \
  -t "$image:$candidate" "$image@$amd64" "$image@$arm64"
digest=$(metadata_digest build/manifest.json)
printf '%s@%s\n' "$image" "$digest" > dist/image-digest.txt
for ref in "$image@$amd64" "$image@$arm64" "$image@$digest"; do
  cosign sign --yes "$ref"
done
cosign sign-blob --yes --bundle dist/checksums.txt.sigstore.json dist/checksums.txt
cosign sign-blob --yes --bundle dist/image-digest.txt.sigstore.json dist/image-digest.txt
