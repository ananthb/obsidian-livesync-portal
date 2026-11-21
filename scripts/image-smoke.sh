#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
set -euo pipefail
image=${1:?image required}
platform=${2:-linux/amd64}
case "$platform" in linux/amd64) arch=amd64;; linux/arm64) arch=arm64;; *) exit 1;; esac
root=$(cd "$(dirname "$0")/.." && pwd)
mkdir -p "$root/build"
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go test -c -o "$root/build/image-smoke-$arch.test" .
docker run --platform "$platform" --pull=never --rm --network none --read-only --user 65532:65532 --cap-drop ALL --security-opt no-new-privileges --tmpfs /tmp:rw,noexec,nosuid,size=64m --tmpfs /data:rw,noexec,nosuid,uid=65532,gid=65532,size=64m --mount "type=bind,src=$root/build/image-smoke-$arch.test,dst=/smoke.test,readonly" --entrypoint /smoke.test "$image" -test.run '^TestInstalledImage$' -test.v
