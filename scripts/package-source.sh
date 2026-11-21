#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
commit=${GITHUB_SHA:-$(git rev-parse HEAD)}
version=$(jq -er '.version' dist/metadata.json)
[[ "$commit" =~ ^[a-f0-9]{40}$ && "$version" =~ ^[a-zA-Z0-9.+-]+$ ]]
[[ "$(git rev-parse HEAD)" == "$commit" ]]
git diff --quiet
git diff --cached --quiet
source_dir=$(mktemp -d)
trap 'rm -rf -- "$source_dir"' EXIT
git archive "$commit" | tar -x -C "$source_dir"
(cd "$source_dir" && GOTOOLCHAIN=go1.27.4 go mod vendor)
mkdir -p "$source_dir/build-inputs"
base=$(jq -er '.image + "@" + .index' licenses/distroless/inventory.json)
jq -n --arg commit "$commit" --arg version "$version" --arg base "$base" \
  '{source_commit:$commit,version:$version,go:"1.27.4",goreleaser:"2.20.3",ko:"0.19.1",base_image:$base,base_evidence:"licenses/distroless/",go_dependencies:"vendor/",cgo_enabled:0,build_tags:"netgo,osusergo",ldflags:("-s -w -X main.sourceVersion="+$version+" -X main.sourceRevision="+$commit),test_only_oracle:"Deno 2.9.8; frozen setup/deno.lock"}' \
  > "$source_dir/build-inputs/manifest.json"
archive="obsidian-livesync-portal_${version}_source.tar.gz"
[[ ! -e "dist/$archive" ]]
timestamp=$(git show -s --format=%ct "$commit")
tar --sort=name --mtime="@$timestamp" --owner=0 --group=0 --numeric-owner -C "$source_dir" -cf - . | gzip -n > "dist/$archive"
(cd dist && sha256sum "$archive" >> checksums.txt)
