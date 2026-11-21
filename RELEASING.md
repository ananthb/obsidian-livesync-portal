<!-- SPDX-License-Identifier: AGPL-3.0-only -->
# Images and releases

Use Go 1.27.4, GoReleaser OSS 2.20.3 and Docker. Images are built exclusively by
GoReleaser's `kos` integration (embedded ko 0.19.1).

```sh
bash scripts/build-local.sh
docker compose --env-file .env -f examples/compose.yaml up -d
```

Snapshots load `goreleaser.ko.local/obsidian-livesync-portal:local` into the local
Docker daemon, without registry credentials or pushes. CI uses the same config
separately with `GOARCH=amd64` and `GOARCH=arm64`; ARM64 executes under QEMU.
`scripts/validate-images.sh` also requires crane 0.22.1 and binfmt/QEMU support.

## Runtime contract

- Executable: `/ko-app/obsidian-livesync-portal`.
- UID/GID: `65532:65532`; the application never runs as root.
- Legal evidence: `$KO_DATA_PATH`, normally `/var/run/ko`.
- CA trust: the pinned distroless `/etc/ssl/certs/ca-certificates.crt`.
- Configuration: `PORTAL_CONFIG=/run/secrets/portal_config` for Compose.
- Compose SQLite: named volume at `/home/nonroot`, database `portal.db`.
- Kubernetes SQLite: `/data/portal.db` remains supported with a PVC at `/data`,
  `runAsUser/runAsGroup: 65532`, and pod `fsGroup: 65532`. The storage driver must
  honor fsGroup. Use a writable `/tmp` emptyDir with a read-only root filesystem.

A fresh Compose volume inherits the base's 65532-owned home. CI tests fresh and
reused named volumes plus fsGroup-equivalent POSIX permissions, not any specific
cluster's storage driver. Existing volumes with a different database location
need explicit migration; do not delete them.

The immutable distroless static Debian 13 base index, architecture descriptors,
complete file hashes/types/link targets and retained package notices are in
`licenses/distroless/`. The base's CA store is intentional; old custom-CA evidence
remains historical legal data, not a runtime trust override. Independent legal
review of this new profile is pending and blocks release publication.

## Publication ordering

1. The repository approval variable must authorize publication. Validate the
   immutable release tag, legal evidence, interoperability, race tests and vet.
2. GoReleaser with `--skip=publish,ko` builds archives, SPDX SBOMs and checksums.
   Smoke the installed archive binary, inspect both static ELFs, and package the
   exact clean source commit with vendored dependencies and base evidence.
3. Require independently approved base evidence. Sign the source archive, publish
   its immutable `source-COMMIT` release, then anonymously download and verify
   the exact source bytes and signature before any image push.
4. A second GoReleaser invocation uses a separate dist directory to build/push
   ko candidates once. GitHub release publication is disabled in GoReleaser.
   Read the immutable index digest from its artifact metadata, require exactly
   amd64/ARM64 descriptors, and test each child digest offline.
5. Crane exports those exact images without Docker-injected container metadata.
   Audit the complete pinned base, recursive legal data, static Go module/ELF
   closure, exact source, and byte-identical archive/image binaries.
6. Keyless-sign child/index digests, checksums and the image-digest asset. Add
   SLSA provenance attestations bound to the workflow identity and exact source.
7. Verify signatures/attestations. Upload an application draft, download and
   verify its assets, promote the exact tested index without rebuilding, recheck
   tag/source state, and finalize the application release last.

Candidates may be visible after source publication and before image tests pass;
they are not releases. A failed attempt never finalizes an application release.
Retries use unique run/attempt candidates and may replace draft assets, never
finalized releases or nonidentical published source. Protect `v*` and `source-*`
tags against updates/deletion. Trust signed digests, not mutable image tags.

The `verify release` workflow requires an independently trusted expected digest
and validates checksum bundles, image signatures and provenance using the exact
release-workflow certificate identity. No `latest` image tag is published.
