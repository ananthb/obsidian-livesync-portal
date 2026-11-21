# Distroless image evidence

Independent release/security and source/license reviews **approved this exact
profile** at source commit `594de2b5ee7a274af8be0a28f68d9008a011841b`, including
the pinned base and native eleven-module static Go closure. The scoped decision
is recorded in `inventory.json`. It is not general legal advice or approval of
other pins/payloads. Preserve the original grants, recursive notices and exact
source obligations. Repository publication authorization remains a separate gate.

Base: `gcr.io/distroless/static-debian13@sha256:aafe993665bf6200db38a02c11a9351e782a84b6a45e7f33b6695638c88e3931`.
The immutable index and amd64/arm64 manifests/configs are in `evidence/`.
`inventory.json` records every flattened base path, type, permissions, owner,
file hash and link target. The final-image gate requires these exact contents;
it does not permit arbitrary OS files or symlinks. Ko writes root-owned mode-0555
headers for `/var` and `/var/run`; the audit checks these exceptions explicitly.

`base-notices/` retains the base's complete package status, documentation and
common-license files. The original image retains those files at their standard
paths too. Packages are base-files, ca-certificates, media-types, netbase,
tzdata and tzdata-legacy. Debian package versions and source package names are
recorded verbatim in `base-notices/var/lib/dpkg/status.d/`.
The retained status copies are shared package evidence; the immutable per-architecture
manifests/configurations and filesystem inventories remain authoritative for each image.

Upstream build source: https://github.com/GoogleContainerTools/distroless
Debian source archive: https://snapshot.debian.org/

The base's `/etc/ssl/certs/ca-certificates.crt` is the runtime trust store.
The older CA evidence under `licenses/ca-bundle/` is retained for historical
source evidence, not installed as the image's runtime trust store.

Re-recording an inventory requires digest-verified `crane export` files for both
architectures and `python3 -B scripts/record-distroless.py EXPORT_DIRECTORY`.
This is a maintainer evidence operation, never a CI approval step.
