<!-- SPDX-License-Identifier: AGPL-3.0-only -->
# Third-party notices

First-party code/modifications are **AGPL-3.0-only**, not “or later”. Original
third-party grants and copyright/permission/disclaimer notices remain in force.

## Native Go release / scratch image

| Component | License / retained notice |
|---|---|
| htmx 4.0.2, embedded unmodified local asset | 0BSD, `licenses/htmx.txt` (also embedded as `static/htmx.LICENSE`) |
| Adapted LiveSync Setup URI generator | MIT, `licenses/setup-helper.txt`, verbatim upstream `setup/upstream/LICENSE` |
| Adapted `@vrtmrz/livesync-commonlib` 0.2.0 protocol | MIT, `licenses/dependencies/npm-_vrtmrz_livesync-commonlib_0.2.0-LICENSE` |
| Adapted octagonal-wheels 0.1.57 crypto protocol | MIT, `licenses/dependencies/npm-octagonal-wheels_0.1.57-LICENSE` |
| 11 linked Go modules, including translated modernc SQLite/libc origins | `licenses/go-runtime/inventory.json` and exact module notices in `licenses/dependencies/`; flattened modernc third-party notices include transpiled native origins |
| Go 1.27.4 runtime/standard library | BSD-3-Clause plus original inline grants/attributions, `licenses/go.txt`, `licenses/go-runtime/inline-notices.txt`, `licenses/go-runtime/stdlib-vendor/` |
| Mozilla CA data in scratch image | MPL-2.0, exact source, Debian copyright and conversion/build source under `licenses/ca-bundle/`; see its provenance manifest |
| Independently fetched Compose service images | Their original licenses; not embedded in or AGPL-relicensed by the portal |

The reviewed release profile is a `CGO_ENABLED=0` static Linux Go binary, with
`netgo,osusergo` tags, and a scratch image containing that binary, intentionally
copied CA data and these notices/source materials. No Deno/V8/Rust runtime,
external setup helper, glibc or libgcc is part of this profile. Artifacts must
be checked, not inferred from build configuration. A different profile needs
review of its actual distributed components.

The native protocol implementation preserves upstream MIT attribution in its
source header. Original generator origin:
`vrtmrz/obsidian-livesync@daa450c6c780698ed5514fcb2d70899291193cfd`,
`utils/setup/generate_setup_uri.ts`, vendored source SHA-256
`2d01e896296b84fdd55db1ec8c23302035c1d561020e3e41adb13d36d6ff01f9`.

## Test/source-only and historical Deno evidence

The upstream Deno interoperability tests and frozen npm graph are not runtime
dependencies of the native binary. Their 159-entry Go-build-list/npm-lock
inventory remains at `licenses/inventory.json`; exact notices and documented
supplements are retained. Declared SPDX metadata is not a substitute for text.

The Deno 2.9.8 release-source legal evidence, 1,130-entry Cargo source lock and
partial native notices under `licenses/deno-runtime/` are **TEST/SOURCE/HISTORICAL
EVIDENCE ONLY**. `licenses/deno.txt` and `licenses/v8.txt` do not prove complete
notice compliance for a Deno executable. That native closure remains unresolved;
reintroducing any compiled Deno helper/runtime invalidates this release profile.

For tr46 0.0.3 test dependencies, the omitted MIT file is retained from its
author's immediately following license-only commit, directly parented by its
exact npm gitHead. Its mapping table matches all 8,179 rows of Unicode 8.0
IDNA data. The additional Unicode copyright/terms are retained in
`licenses/tr46-unicode.txt` and `licenses/evidence/tr46-unicode-header.txt`.

Regenerate/check evidence (source/test regeneration also requires frozen Deno
caching and `go mod download all`):

```sh
python3 scripts/go-license-inventory.py
python3 scripts/license-provenance.py
# Optional full test/source graph regeneration:
deno run --allow-read --allow-write=licenses --allow-run scripts/license-audit.ts
```

See `licenses/PROVENANCE.md` for authoritative origins, CA source obligations,
source-archive/artifact verification and AGPL Corresponding Source/network-UI
requirements. Scoped checks do not establish public source availability or
constitute exhaustive legal advice/clearance.
