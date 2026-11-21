<!-- SPDX-License-Identifier: AGPL-3.0-only -->
# License/provenance review — 2026-11-21

## Active distribution profile: native Go, static Linux, pinned distroless

Independent release/security and source/license reviews approved the exact
profile at `594de2b5ee7a274af8be0a28f68d9008a011841b`: Go 1.27.4's native
eleven-module static closure and the amd64/arm64 payload of
`gcr.io/distroless/static-debian13@sha256:aafe993665bf6200db38a02c11a9351e782a84b6a45e7f33b6695638c88e3931`.
The decision is recorded in `distroless/inventory.json`. It is scoped approval,
not general legal advice, clearance for other pins/payloads, or publication
authorization. Original grants, recursive notices, base/source evidence and
Corresponding Source/network-offer obligations remain in force.

The immutable index, both architecture manifests/configurations, exact flattened
file/type/link/ownership inventory and retained package documentation/common
licenses are under `distroless/`. The base's CA bundle is the active runtime
trust store. GoReleaser-ko installs recursive legal data under `/var/run/ko`.
Crane exports the image layers directly, excluding Docker-injected runtime
metadata. The separate distroless audit checks exact inventoried base contents,
ko asset paths and narrowly constrained data-parent headers; scratch/archive
checks remain strict. Final image binaries must be byte-identical to the
corresponding release archive binaries.

The native codec introduced at `2cadac2` replaces the Deno-compiled setup
helper. **The old Deno Rust/native binary-closure blocker does not apply when
no Deno/helper/runtime is distributed. It has NOT been resolved for Deno.**
The material below headed “Historical Deno/test provenance” remains useful
test/source evidence only; distributing those text notices is not distributing
a Deno runtime and does not require solving its binary dependency closure.

The active reviewed source closure uses Go 1.27.4, `CGO_ENABLED=0`,
`GOOS=linux`, `GOARCH=amd64/arm64`, tags `netgo,osusergo`, and **11 linked
third-party modules**, recorded in `go-runtime/inventory.json`. `go mod verify`
passed. The architecture-specific package closures contain no CgoFiles.
Actual artifacts must match that closure and have no ELF interpreter/dynamic
segment; source build settings alone are not proof. The static SQLite driver
contains translated musl/SQLite code, not dynamically linked system libc.
Its existing flattened third-party notices explicitly account for those origins.

`go-runtime/inline-notices.txt` additionally preserves 1,757 copyright/permission
comment blocks from both target source closures, including Go's Plan 9 MIT
assembly origins, Sun math permissions, Cephes math attribution and Fiat crypto
notices. Standard-library vendored notices are retained separately. This fixes
the earlier overreliance on Go's top-level BSD license. This is a conservative
source closure (some portions can be eliminated by the linker), not a claim
that a scanner proves every possible legal question settled.

The native adapted protocol code retains the upstream MIT attribution for the
Setup URI generator, commonlib 0.2.0 and octagonal-wheels 0.1.57. Their exact
MIT notices remain in `setup-helper.txt` and `dependencies/`. Its first-party
modifications and the other first-party code use **AGPL-3.0-only**. htmx remains
0BSD. The reviewed linked modules/retained inline permissions present no
identified incompatibility with AGPLv3-only; third-party grants are not replaced.

### Historical custom CA data and scratch image

The following evidence remains retained historical source material. It is not
the current distroless runtime trust store or its active audit profile.

`ca-bundle/provenance.json` records the CA bundle actually extracted from the
digest-pinned Go builder: Debian `ca-certificates` **20250419~deb12u2**, 148
Mozilla source certificates; bundle SHA-256
`e9c7b63ee2623386e9b0a3ee32f499d01a8ea23dc83b8383ddff9f8e749543b7`.
The pinned image index and amd64 child manifest, layer digests and original
paths are recorded. Image layers were digest-checked and read, not executed.
The image build's `$BUILDPLATFORM` may differ from target architecture: require
the same CA hash in both final images, or review a different bundle explicitly.

Mozilla certificate data is **MPL-2.0**, separately licensed data, not AGPL.
The full Debian copyright file, MPL text, original PEM files and exact Debian
source package (including Mozilla certdata.txt and conversion/build scripts)
are retained under `ca-bundle/`. The source tar SHA-256 matches the authoritative
Debian-security `.dsc`; its PGP signature was not independently authenticated.
Keeping this directory recursively in each final image/archive makes the
source and attribution available with the data, rather than relying on a
mutable download link. GPL-2+ terms in the Debian copyright apply to its utility
scripts: they are retained inside the complete source archive, not installed
as executable system utilities or linked into the application. GPL-2 text is
also retained. The aggregate does not relicense the Go binary under GPL-2.

For that historical **scratch** payload (one static app, CA data, legal/source
materials, empty writable directories), no Debian glibc, libgcc, shell or
package-manager binaries are conveyed. Their old final-image source/relinking
obligations therefore dropped out. The active distroless profile has its own
separately inventoried/reviewed base and preserved source obligations described
above; this historical scratch finding does not clear arbitrary OS additions.
Build-stage tools are not automatically part of the distributed image.

### Per-release gates and integration requirements

1. Rebuild both archives without `setup-uri`, `deno`, `denort`, native shared
   libraries or any other unreviewed executable. Keep the complete recursive
   legal tree; old Deno-era snapshots must not be published.
2. Publish a checksum-bound source archive for the **same exact commit** as
   both binaries. Include all tracked build/configuration/legal files,
   `go.mod`/`go.sum`, first-party native codec and upstream source-test fixtures.
   Never substitute GitHub's changing default branch. The verifier compares
   tracked archive files byte-for-byte against `git archive <revision>`;
   optional `vendor/` additions must exactly match `go mod vendor` and the
    packaged `build-inputs/manifest.json` must match the exact source commit,
    Go/GoReleaser/ko versions and pinned base image evidence.
   It also checks VCS revision and clean-state build information in both binaries.
3. Provide a public version-specific Corresponding Source URL in the release
   and deployment/network UI. Versioned builds should default `SourceURL` to
   their matching source release asset; development/snapshot or modified
   deployments must configure the appropriate exact source offer explicitly.
   Verify the link is actually offered and matches deployed modifications.
   A syntactically valid URL in a local check does
   **not** prove public source is hosted or the network UI displays it. This
   audit does not authorize a push or prove a published source-delivery path.
4. Verify both flattened final image exports with `--image-profile distroless`
    against the pinned base inventory, static binaries, recursive notices and
    exact archive binaries. OCI layer archives
   are not flattened exports and must not be passed as though they were.

Commands (Python stdlib plus the installed Go toolchain):

```
python3 scripts/go-license-inventory.py
python3 scripts/verify-ca-provenance.py
python3 scripts/license-provenance.py
python3 scripts/license-provenance.py --archive release-amd64.tar.gz --archive release-arm64.tar.gz
python3 scripts/license-provenance.py --release \
  --archive release-amd64.tar.gz --archive release-arm64.tar.gz \
  --source-archive source.tar.gz --revision FULL_COMMIT \
  --source-url https://github.com/ananthb/obsidian-livesync-portal/releases/download/source-FULL_COMMIT/obsidian-livesync-portal_VERSION_source.tar.gz
# A canonical /releases/download/vVERSION/..._VERSION_source.tar.gz URL is
# also accepted when its filename matches the verified source archive.
# Add --images --image-tar exported-amd64.tar --image-tar exported-arm64.tar
# plus --image-profile distroless for the active pinned image profile.
```

The gate is scoped local evidence, not legal advice, exhaustive clearance or
proof of public source availability. A bare `--release` fails because it lacks
actual artifacts/source. No runtime Deno blanket blocker remains in this profile.
Use the normal invocation for a pre-build evidence gate. Run `--release` only
after packaging both architectures and the matching source archive. Existing
workflow calls that use bare `--release` before building must be moved/updated
by the integration owner; this audit does not edit workflows.

Historical native/scratch validation performed locally: both architecture binaries were
built with the reviewed toolchain/tags and inspected; legal-only test packages
containing each real binary and all notices passed, as did two synthetic
flattened scratch exports containing the exact CA data. These are verifier
tests, **not Docker-built final images or a production release**. Missing-MIT,
accidental-helper, dirty-VCS and empty-release checks failed as intended. Exact
`git archive` source and generated `vendor/` plus CA/build-input source packages
passed. Actual clean-revision release artifacts, Docker-built images and public
source/network-UI availability still needed end-to-end validation at that stage.
The active ko/distroless profile subsequently passed actual amd64 and QEMU ARM64
execution, named-volume/fsGroup-equivalent storage checks, exact image/archive
audits and Corresponding Source validation in GitHub runs `44943380779` and
`44943384177` at the reviewed commit. Public release signing/source delivery
and deployment/network-UI availability remain separate post-authorization checks.

## Historical Deno/test provenance (not native-Go binary clearance)

The original extraction was reviewed at `c2db9c0`. Its source/test dependency
notices and release-source evidence are preserved without claiming permission
or notice completeness for distributing Deno-compiled artifacts.

### Historical first-party licensing and compatibility review

First-party licensing is **AGPL-3.0-only**, not AGPL-3.0-or-later. The source
headers, README statement and `REUSE.toml` use that exact identifier. The stock
AGPL text's illustrative “or later” example is not a project license grant;
do not edit the FSF license text. Third-party headers and notices retain their
original licenses and must not be overwritten with AGPL identifiers.

The inventoried npm declarations are MIT, ISC, 0BSD, BSD-2-Clause,
BSD-3-Clause, Apache-2.0, BlueOak-1.0.0, Python-2.0, Unlicense and
`(WTFPL OR MIT)` (use the MIT option for spark-md5). These licenses do not
present an identified AGPLv3 compatibility conflict. Retain their actual
copyright/permission/disclaimer texts; Apache-2.0 also requires retaining
applicable upstream NOTICE text and identifying modifications when applicable.
The retained modernc SQLite flattened third-party notice includes transpiled
native origins; it is not merely an SPDX label for the Go wrapper.

This compatibility finding does **not** clear the unreviewed Deno native graph.
Separate processes and independently distributed Compose service images do
not automatically relicense each other. Do not describe the Debian base, Deno,
Obsidian, CouchDB or other upstream software as first-party AGPL code.

AGPL distribution requires the actual Corresponding Source for the matching
binary, including first-party helper source, modifications and scripts needed
to build/install/run it (AGPL sections 1 and 6). A source link to a different
commit or an SBOM is not a substitute. Modified network deployments must offer
their corresponding source under section 13. The exact source/build inputs
and versioned upstream source references must accompany an approved release;
this audit has not tested a published release/source-delivery path.

## tr46 0.0.3

The npm registry's authoritative version metadata declares MIT and identifies
gitHead `a8009f9ce80ff5dbe71dd71e203afe4e4c878d28`:
<https://registry.npmjs.org/tr46/0.0.3>.
The npm tarball SHA-256 is
`164ae1eb32cea353551bbc7f9358dcaae4ffabbe65ec37a92ca464a9570a2a0a`;
its package.json and that gitHead both declare MIT. The tarball has no license
file. This is not resolved by guessing a generic MIT copyright owner.

The author's next commit,
[`3a6f29721e7063b9ffd421e461a54beae6170001`](https://github.com/jsdom/tr46/commit/3a6f29721e7063b9ffd421e461a54beae6170001),
is **directly parented by that exact npm gitHead** and adds only LICENSE.md:
no code changes. It is authored by Sebastian Mayr, says “Add MIT license”,
and closes [upstream issue 4](https://github.com/jsdom/tr46/issues/4), whose
request is specifically to supply the omitted MIT copyright/license text.
The retained `tr46.txt` is byte-identical to this file, not just inferred
from a later version. `evidence/tr46-license-commit.json` preserves the
parent relationship and sole-file diff. This closes the historical **MIT
notice provenance** gap for the unmodified 0.0.3 code.

The deeper review found a separate data origin missing from the prior inventory:
`lib/mappingTable.json` derives from Unicode's IDNA data. The package's
upstream generator strips comments from IdnaMappingTable.txt. Reproducing that
transformation against
<https://www.unicode.org/Public/idna/8.0.0/IdnaMappingTable.txt>
matches **all 8,179 rows** of the npm tarball's generated table exactly.
The original 1991–2015 Unicode copyright header is preserved in
`evidence/tr46-unicode-header.txt`; `tr46-unicode.txt` retains Unicode's full
currently offered Unicode License V3 from <https://www.unicode.org/license.txt>
(retrieved 2026-11-21). This data supplement is explicitly separate from the
author's MIT grant. The table itself is not vendored in this project; the
verification command below prints its source SHA-256:
`5e9f5929130b713e698162ac5b60a99ccfb831606686b1c50777cd920b55dee2`.
The copyright/permission text must travel with the derived data in the helper.

## Deno release, Rust and native origins

Authoritative release:
<https://github.com/denoland/deno/releases/tag/v2.9.8>.
The upstream GitHub tag resolves to
`fe7ed012d92c642fd8e5607ac5a45d517538b5c5` (retained API evidence).
The **actual release asset**, not an unrelated branch snapshot, was retrieved:
<https://github.com/denoland/deno/releases/download/v2.9.8/deno_src.tar.gz>.
SHA-256: `b57d0281c1647effc735f242ba52bc409747fc6ed6ad4946e76f062c67e113eb`.
This hash records retrieved bytes; it is not a claim of an independently signed
upstream source attestation.

`deno-runtime/` retains its LICENSE.md, Cargo.lock, Cargo.toml,
rust-toolchain.toml, WebGPU MIT notice, Microsoft MIT notice for the Node
TypeScript definitions, and the Undici MIT notice. These definition notices
are not a Node runtime/polyfill third-party license bundle.
The release locks **1,130 Cargo packages**. The new native inventory deliberately
lists the entire source lock with unresolved review status: dev, build and
non-Linux dependencies are not silently called linked runtime components.
The existing 159 Go/npm entries do **not** include these 1,130 entries and do
not count as review of them. Selected native crates have checksum-verified
crate-source license/NOTICE/COPYING files copied recursively, not only their
top-level license labels. These are partial collections, not a closure proof.

In particular the lock pins Rusty V8 **150.4.0**, not a guessed current V8.
Its published crate SHA-256 matches Cargo.lock:
`42a978ff11f15b24e5c05a7123cf2b68f41e763546699781a924ef4e2cf43a49`.
The crate's VCS metadata identifies Rusty V8 commit
`5c15a6995c9bb4bacd3e341b59fff32c909c80bf` with `dirty: true`; do not call that
metadata a clean reproducible-build attestation. The corresponding upstream
tree pins the V8 submodule to
`ac1e23989121713ca642f6650b34deff7b686896`. `v8.txt` matches the LICENSE from
that exact V8 revision; Rusty V8's own MIT notice is separately retained.
V8's BSD text explicitly excludes externally maintained libraries and is not
their notice bundle. Rusty V8's published crate omits LICENSE files, so its
MIT metadata alone is insufficient. The pinned source reference matters.

The release specifies Rust 1.95.0; its MIT, Apache-2.0 and COPYRIGHT files
are retained separately. This is **not** a complete Rust standard-library
third-party notice bundle or proof of the toolchain used for both denort assets.

### Unresolved blocker IF Deno runtime/helper binaries are distributed again

No complete upstream native legal bundle was present in the Deno release asset
list or release source root. It would be incorrect to infer completeness from
that absence, Deno's MIT license, retained crate files, or a Go-binary SBOM.
Before publication, reconcile both release `denort-*-unknown-linux-gnu`
artifacts to their exact enabled Rust dependency/features and native build
inputs; account for inline notices and bundled/generated data as well as
license-named files. Explicit unresolved examples include V8's pinned ICU,
Abseil, libc++/ABI/unwind, simdutf and other native submodules, their nested
notices and LLVM exceptions; Node-derived polyfill inline notices; native
crypto's vendored assembly and mixed
licenses (ring/AWS-LC); graphics/font/image libraries if linked; Rust runtime
third-party code; and final Debian glibc/libgcc/system package source obligations.
Not every item in this conservative list is necessarily linked; exclusion
must be supported by the actual target build, not assumptions about APIs used.
`deno_core_icudata` 0.77.0 is a concrete warning: its crate declares MIT,
ships `src/icudtl.dat` from Rusty V8's ICU data, and has no license-named file.
The wrapper's MIT label does not account for ICU data provenance/terms.

No incompatible native license has been established, but **permission/notice
completeness for all embedded native code has not been established either**.
Do not release Deno-compiled helper archives or images on the strength of this
audit. This restriction does not apply to the reviewed native-Go-only profile.

### Historical Deno-era distribution inclusion findings

* The upstream MIT helper license originally lived only at
  `setup/upstream/LICENSE`, outside the archive/image inclusion globs. It is now
  duplicated verbatim as `licenses/setup-helper.txt`, which those globs include.
* `.goreleaser.yaml` includes `LICENSE`, `THIRD_PARTY_NOTICES.md`, and
  `licenses/**`; ko data includes the complete `licenses/` tree. Keep that
  recursive structure, including evidence and nested crate notices. No app,
  image recipe or workflow was changed by this license review.
* Existing `dist/` archives and previously built images predate these additions.
  Rebuild, inspect both architecture archives and final images, and compare all
  legal-file hashes against `licenses/provenance.json`. Checking the source
  glob alone does not prove artifact inclusion. The two current identity-updated
  snapshot archives were inspected: **52 of the 54 newly recorded legal/evidence
  assets are absent in each**. Do not publish those existing snapshots.
* Include `python3 scripts/license-provenance.py --release` as a release gate
  only after reconciling its blockers. It currently exits nonzero intentionally;
  the normal invocation verifies retained evidence, not legal clearance.
* Preserve `/usr/share/doc/*/copyright` in Debian images, but also fulfill the
  actual installed GPL/LGPL package source and relinking obligations. Keeping
  copyright files alone is insufficient. Record exact package/source versions
  from both final images and provide the applicable source-delivery mechanism.

### Historical evidence revalidation

```
python3 scripts/license-provenance.py
python3 scripts/license-provenance.py --release  # fails without actual release/source inputs
python3 scripts/license-provenance.py --archive /path/to/rebuilt-release.tar.gz
python3 scripts/verify-tr46-provenance.py /path/to/tr46-0.0.3.tgz /path/to/IdnaMappingTable-8.0.0.txt
```

`provenance.json` hashes the retained assets and records authoritative source
URLs, archive-member names and crate archive checksums where applicable. Hashes
verify byte preservation, not permission from every possible rightsholder.
The downloaded source archives were used as data, not executed. No live service,
private credentials, publication operation or hand-installed tool was needed.

Audit checks passed: 54 retained-asset SHA-256 checks, all 159 Go/npm notice-path
checks, both Python scripts' syntax checks, tr46's 8,179-row transformation and
license-only commit relationship, and first-party Go exact SPDX headers.
The release gate failed as intended; an existing snapshot archive failed legal
file equality as intended. A temporary legal-only test archive passed all 225
current legal-file byte comparisons (this is a verifier test, not a rebuilt
release). Deno's source-notice inventory script was updated for the Unicode
supplement and to exit nonzero on missing notices, but was not rerun here.
