#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
# Exercise the release Dockerfile and exact packaged binaries without registry writes.
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
version=$(jq -er '.version' dist/metadata.json)
revision=${GITHUB_SHA:-$(git rev-parse HEAD)}
container=''
trap 'if [[ -n "$container" ]]; then docker rm "$container"; fi' EXIT
mkdir -p build
for arch in amd64 arm64; do
  dir="build/validate-image-$arch"
  mkdir "$dir"
  tar -xzf "dist/obsidian-livesync-portal_${version}_linux_${arch}.tar.gz" -C "$dir"
  cp Dockerfile.release "$dir/Dockerfile"
  image="obsidian-livesync-portal:validate-$arch"
  docker buildx build --platform "linux/$arch" --load -t "$image" "$dir"
  bash scripts/image-smoke.sh "$image" "linux/$arch"
  container=$(docker create --platform "linux/$arch" "$image")
  docker export --output "build/validate-image-$arch.tar" "$container"
  python3 -c 'import sys, tarfile
with tarfile.open(sys.argv[1]) as archive:
    for member in archive:
        if member.issym() or member.islnk() or member.name == "dev/console":
            print(f"Export metadata evidence: {member.name!r} type={member.type!r} target={member.linkname!r} size={member.size} mode={member.mode:o}", flush=True)
' "build/validate-image-$arch.tar"
  docker rm "$container"
  container=''
done
python3 scripts/license-provenance.py --release --images \
  --archive "dist/obsidian-livesync-portal_${version}_linux_amd64.tar.gz" \
  --archive "dist/obsidian-livesync-portal_${version}_linux_arm64.tar.gz" \
  --source-archive "dist/obsidian-livesync-portal_${version}_source.tar.gz" \
  --revision "$revision" \
  --source-url "https://github.com/ananthb/obsidian-livesync-portal/releases/download/source-$revision/obsidian-livesync-portal_${version}_source.tar.gz" \
  --image-tar build/validate-image-amd64.tar \
  --image-tar build/validate-image-arm64.tar
