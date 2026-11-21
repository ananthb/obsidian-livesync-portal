#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
set -euo pipefail
image=${1:?image required}
platform=${2:-linux/amd64}
case "$platform" in linux/amd64) arch=amd64;; linux/arm64) arch=arm64;; *) exit 1;; esac
root=$(cd "$(dirname "$0")/.." && pwd)
mkdir -p "$root/build"
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go test -c -o "$root/build/image-smoke-$arch.test" .
[[ "$(docker image inspect --format '{{.Config.User}}' "$image")" == 65532:65532 ]]
volume=$(docker volume create)
trap 'docker volume rm "$volume"' EXIT
common=(--platform "$platform" --pull=never --rm --network none --read-only
  --cap-drop ALL --security-opt no-new-privileges --env PORTAL_IMAGE_TEST=1
  --tmpfs "/tmp:rw,noexec,nosuid,size=64m"
  --mount "type=bind,src=$root/build/image-smoke-$arch.test,dst=/smoke.test,readonly"
  --entrypoint /smoke.test)
# A fresh named volume must inherit the base's nonroot home ownership, then persist.
for attempt in 1 2; do
  echo "Named-volume runtime smoke: $platform attempt $attempt"
  docker run "${common[@]}" --mount "type=volume,src=$volume,dst=/home/nonroot" "$image" -test.run '^TestInstalledImage$' -test.v
done
# Match a kubelet fsGroup-owned PVC's POSIX permissions without a root app process.
docker run "${common[@]}" --env PORTAL_TEST_DATA=/data \
  --tmpfs /data:rw,noexec,nosuid,uid=0,gid=65532,mode=0770,size=64m \
  "$image" -test.run '^TestInstalledImage$' -test.v
