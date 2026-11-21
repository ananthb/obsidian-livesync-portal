#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
(cd setup && deno cache --frozen interop.ts known_answer.ts main_test.ts)
SETUP_INTEROP_REQUIRED=1 go test -count=1 -run '^TestSetupUpstreamInteroperability$' .
(cd setup && deno test --frozen --cached-only --allow-env=__MINIMATCH_TESTING_PLATFORM__ main_test.ts)
