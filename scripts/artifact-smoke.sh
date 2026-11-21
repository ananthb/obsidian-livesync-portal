#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root/dist"
sha256sum --check checksums.txt
cd "$root"
for archive in dist/obsidian-livesync-portal_*_linux_amd64.tar.gz dist/obsidian-livesync-portal_*_linux_arm64.tar.gz; do
  members=$(tar -tzf "$archive")
  if grep -qx setup-uri <<< "$members"; then
    echo 'Obsolete runtime helper found in archive' >&2
    exit 1
  fi
done
installed=$(mktemp -d)
trap 'rm -f "$installed/livesync-portal"; rmdir "$installed"' EXIT
tar -xzf dist/obsidian-livesync-portal_*_linux_amd64.tar.gz -C "$installed" livesync-portal
expected=${PORTAL_EXPECTED_SOURCE_URL:-}
version=$(jq -er '.version' dist/metadata.json)
if [[ "$version" != *SNAPSHOT* && -n "${GITHUB_SHA:-}" ]]; then
  expected="https://github.com/ananthb/obsidian-livesync-portal/releases/download/source-$GITHUB_SHA/obsidian-livesync-portal_${version}_source.tar.gz"
fi
PORTAL_INSTALLED_TEST_BINARY="$installed/livesync-portal" PORTAL_EXPECTED_SOURCE_URL="$expected" go test -count=1 -run '^TestInstalledArtifact$' .
