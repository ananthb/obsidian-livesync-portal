#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
set -euo pipefail
# Invoke the real binary via PATH in a later shell, not the action's absolute path.
command -v goreleaser
version=$(goreleaser --version)
printf '%s\n' "$version"
grep -Eq '^GitVersion:[[:space:]]+2\.20\.3$' <<< "$version"
goreleaser check
