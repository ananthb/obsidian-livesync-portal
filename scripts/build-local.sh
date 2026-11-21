#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
export PORTAL_DIST=dist IMAGE=ghcr.io/ananthb/obsidian-livesync-portal IMAGE_TAG=local
export RELEASE_OWNER=ananthb RELEASE_REPO=obsidian-livesync-portal
export GOARCH=${GOARCH:-$(go env GOARCH)}
case "$GOARCH" in amd64|arm64) ;; *) echo 'Only amd64/arm64 images are supported' >&2; exit 1;; esac
goreleaser check
goreleaser release --snapshot --clean --skip=publish,sign,sbom
echo 'Loaded goreleaser.ko.local/obsidian-livesync-portal:local (no registry push)'
