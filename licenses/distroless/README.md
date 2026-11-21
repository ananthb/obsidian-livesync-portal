# Distroless image evidence

The image profile is **pending independent legal review**. Mechanical CI is not
publication approval. Release publication also requires this review to be recorded
as approved in `inventory.json`.

Base: `gcr.io/distroless/static-debian13@sha256:aafe993665bf6200db38a02c11a9351e782a84b6a45e7f33b6695638c88e3931`.
The immutable index and amd64/arm64 manifests/configs are in `evidence/`.
`inventory.json` records every flattened base path, type, permissions, owner,
file hash and link target. The final-image gate requires these exact contents;
it does not permit arbitrary OS files or symlinks. Ko replaces `/var/run` with
its data directory; the audit checks this exception explicitly.

`base-notices/` retains the base's complete package status, documentation and
common-license files. The original image retains those files at their standard
paths too. Packages are base-files, ca-certificates, media-types, netbase,
tzdata and tzdata-legacy. Debian package versions and source package names are
recorded verbatim in `base-notices/var/lib/dpkg/status.d/`.

Upstream build source: https://github.com/GoogleContainerTools/distroless
Debian source archive: https://snapshot.debian.org/

The base's `/etc/ssl/certs/ca-certificates.crt` is the runtime trust store.
The older CA evidence under `licenses/ca-bundle/` is retained for historical
source evidence, not installed as the image's runtime trust store.

Re-recording an inventory requires digest-verified `crane export` files for both
architectures and `python3 -B scripts/record-distroless.py EXPORT_DIRECTORY`.
This is a maintainer evidence operation, never a CI approval step.
