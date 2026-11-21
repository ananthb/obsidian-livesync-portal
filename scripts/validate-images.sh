#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
export IMAGE=ghcr.io/ananthb/obsidian-livesync-portal
export RELEASE_OWNER=ananthb RELEASE_REPO=obsidian-livesync-portal
mkdir -p build
for arch in amd64 arm64; do
  export GOARCH="$arch" IMAGE_TAG="snapshot-$arch"
  export PORTAL_DIST=dist
  if [[ "$arch" == arm64 ]]; then PORTAL_DIST=dist-arm64; fi
  goreleaser release --snapshot --clean --skip=publish,sign,sbom
  image=$(docker image inspect --format '{{.Id}}' "goreleaser.ko.local/obsidian-livesync-portal:$IMAGE_TAG")
  [[ "$image" =~ ^sha256:[a-f0-9]{64}$ ]]
  bash scripts/image-smoke.sh "$image" "linux/$arch"
  # Flatten the exact saved image, not a container's injected runtime filesystem.
  docker save "$image" | crane export - "build/image-$arch.tar"
done
unset GOARCH
export PORTAL_DIST=dist
bash scripts/artifact-smoke.sh
bash scripts/check-static.sh dist/portal_linux_amd64_v1/livesync-portal dist/portal_linux_arm64_v8.0/livesync-portal
bash scripts/package-source.sh
version=$(jq -er '.version' dist/metadata.json)
revision=${GITHUB_SHA:-$(git rev-parse HEAD)}
python3 -B scripts/license-provenance.py --release --images --image-profile distroless \
  --archive "dist/obsidian-livesync-portal_${version}_linux_amd64.tar.gz" \
  --archive "dist/obsidian-livesync-portal_${version}_linux_arm64.tar.gz" \
  --source-archive "dist/obsidian-livesync-portal_${version}_source.tar.gz" \
  --revision "$revision" \
  --source-url "https://github.com/ananthb/obsidian-livesync-portal/releases/download/source-$revision/obsidian-livesync-portal_${version}_source.tar.gz" \
  --image-tar build/image-amd64.tar --image-tar build/image-arm64.tar
