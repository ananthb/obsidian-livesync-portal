<!-- SPDX-License-Identifier: AGPL-3.0-only -->
# Third-party notices

First-party code/modifications are **AGPL-3.0-only**, not “or later”. Original
third-party grants and copyright/permission/disclaimer notices remain in force.

## Native Go release

| Component | License / retained notice |
|---|---|
| htmx 4.0.2, embedded unmodified local asset | 0BSD, `licenses/htmx.txt` (also embedded as `static/htmx.LICENSE`) |
| Adapted LiveSync Setup URI generator | MIT, `licenses/setup-helper.txt`, verbatim upstream `setup/upstream/LICENSE` |
| Adapted `@vrtmrz/livesync-commonlib` 0.2.0 protocol | MIT, `licenses/dependencies/npm-_vrtmrz_livesync-commonlib_0.2.0-LICENSE` |
| Adapted octagonal-wheels 0.1.57 crypto protocol | MIT, `licenses/dependencies/npm-octagonal-wheels_0.1.57-LICENSE` |
| 11 linked Go modules, including translated modernc SQLite/libc origins | `licenses/go-runtime/inventory.json` and exact module notices in `licenses/dependencies/`; flattened modernc third-party notices include transpiled native origins |
| Go 1.27.4 runtime/standard library | BSD-3-Clause plus original inline grants/attributions, `licenses/go.txt`, `licenses/go-runtime/inline-notices.txt`, `licenses/go-runtime/stdlib-vendor/` |
| Historical custom Mozilla CA evidence (not the current runtime trust store) | MPL-2.0, exact source, Debian copyright and conversion/build source under `licenses/ca-bundle/`; see its provenance manifest |
| Independently fetched Compose service images | Their original licenses; not embedded in or AGPL-relicensed by the portal |

The reviewed binary profile is `CGO_ENABLED=0` static Linux Go with
`netgo,osusergo` tags. No Deno/V8/Rust runtime, external setup helper, glibc or
libgcc is linked into it. Artifacts must be checked, not inferred from settings.

## Distroless image profile: independent review pending

The current ko image uses the digest-pinned distroless static Debian 13 nonroot
base recorded under `licenses/distroless/`. Its package payload includes
base-files, ca-certificates, media-types, netbase, tzdata and tzdata-legacy.
Their complete image-shipped copyright notices/common licenses and package
metadata are retained in `licenses/distroless/base-notices/`, alongside exact
base manifests, configurations and a per-path content/type/link inventory.
The base's CA bundle is the default runtime trust store. These components retain
their original grants; they are not relicensed as first-party AGPL code.

This new image profile has not yet received independent source/license closure
approval. Its mechanical inventory is not legal clearance. Release publication
requires that review to be recorded approved; binary-profile approval alone does
not authorize the additional base components.

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
