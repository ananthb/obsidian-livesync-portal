#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
set -euo pipefail
for binary in "$@"; do
  readelf --file-header "$binary" >/dev/null
  headers=$(readelf --program-headers "$binary")
  if grep -q INTERP <<< "$headers"; then
    echo "Unexpected ELF interpreter: $binary" >&2
    exit 1
  fi
  dynamic=$(readelf --dynamic "$binary")
  if grep -q NEEDED <<< "$dynamic"; then
    echo "Unexpected dynamic runtime dependency: $binary" >&2
    exit 1
  fi
done
