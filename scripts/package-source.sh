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
mkdir -p "$source_dir/build-inputs/etc/ssl/certs" "$source_dir/build-inputs/usr/share/doc/ca-certificates"
builder=golang:1.27.4-bookworm@sha256:4fa7c968ba93f1635670522dbb01a2c78150443e1385746924566079f36570ea
# The legal owner retained the exact digest-verified builder CA data and notice.
cp "$source_dir/licenses/ca-bundle/certificates.pem" "$source_dir/build-inputs/etc/ssl/certs/ca-certificates.crt"
cp "$source_dir/licenses/ca-bundle/debian-copyright" "$source_dir/build-inputs/usr/share/doc/ca-certificates/copyright"
ca_hash=$(sha256sum "$source_dir/build-inputs/etc/ssl/certs/ca-certificates.crt" | awk '{print $1}')
jq -n --arg commit "$commit" --arg version "$version" --arg builder "$builder" --arg ca "$ca_hash" \
  '{source_commit:$commit,version:$version,go:"1.27.4",builder_image:$builder,ca_bundle_sha256:$ca,go_dependencies:"vendor/",cgo_enabled:0,build_tags:"netgo,osusergo",ldflags:("-s -w -X main.sourceVersion="+$version+" -X main.sourceRevision="+$commit),test_only_oracle:"Deno 2.9.8; frozen setup/deno.lock"}' \
  > "$source_dir/build-inputs/manifest.json"
archive="obsidian-livesync-portal_${version}_source.tar.gz"
[[ ! -e "dist/$archive" ]]
timestamp=$(git show -s --format=%ct "$commit")
tar --sort=name --mtime="@$timestamp" --owner=0 --group=0 --numeric-owner -C "$source_dir" -cf - . | gzip -n > "dist/$archive"
(cd dist && sha256sum "$archive" >> checksums.txt)
