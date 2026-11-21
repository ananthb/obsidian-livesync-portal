#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
GOTOOLCHAIN=go1.27.4 go mod download
GOTOOLCHAIN=go1.27.4 go mod verify
# Deno is only an interoperability oracle, never an application build input.
if [[ "${1:-}" == --interop ]]; then
  (cd setup && deno cache --frozen interop.ts known_answer.ts main_test.ts)
fi
