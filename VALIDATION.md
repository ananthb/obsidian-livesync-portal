<!-- SPDX-License-Identifier: AGPL-3.0-only -->
# Extraction validation

Status on 2026-11-21. This candidate is not a published release.

The table below records **historical pre-native extraction checks**, not proof
that the newly integrated native-Go release artifacts/images have been rebuilt.
Current legal scope and per-release requirements follow under Remaining gates.

| Check | Result |
|---|---|
| Authoritative Go release feed and toolchain | `go.dev/dl/?mode=json` reported go1.27.4; `GOTOOLCHAIN=go1.27.4 go version` downloaded and verified the toolchain |
| Unit/integration tests | `go test -race -count=1 -cover ./...` passed; 67.3% instrumented statement coverage before final presentation-only changes |
| Static Go checks | `go vet ./...` passed |
| SQLite | Configuration/catalog persistence, restart-stable keys, CAS conflicts, ownership mismatch, exclusive lock, parallel bootstrap/create and partial-failure retry tests passed |
| OIDC | Mock RSA/JWKS/token tests passed for nonce/state/replay/signature/issuer/audience/expiry/subject rejection, required/optional/scalar groups and verified-email policy; PKCE challenge/exchange tested |
| HTTP boundaries | Own-record authorization, CSRF/Origin, protected default, no-store and removed admin routes tested |
| Vault | Configurable KV mount/path, ordinary token, Kubernetes JWT login, token cache, CAS and unsafe/cross-user path tests passed using local dummy servers only |
| LiveSync | Deno 2.9.8 compiled both Linux helpers; Deno contract test and repeated compiled-helper URI decryption with actual pinned commonlib passed |
| Installed archive | Extracted amd64 binary found its sibling helper, produced a decryptable URI, loaded configuration, discovered a local TLS OIDC mock and served health/login/static responses |
| NixOS host adaptation | Only temporary test copies of the generic ELF helpers had their interpreter patched to the host's Nix glibc path; original release archives were not patched |
| GoReleaser | Latest OSS 2.20.3 validated the configuration and built both snapshots; helper is at archive root with executable mode |
| Checksums/SBOM | Both archive SHA-256 checks and both generated SPDX SBOM checks passed; Syft 1.56.0 generated SBOMs with 14 packages each; CI pins Syft 1.57.2 |
| Workflows | actionlint passed; referenced actions pinned to verified release commit SHAs |
| Compose | Config rendering passed with synthetic hostnames; generator produced private configuration accepted by the application import command |
| Image references | All pinned base/Compose registry manifest digests were resolved from their registries |
| Privacy | Forbidden original-deployment hostname/person-string search found no matches in candidate sources; no source-repository git history or remote was imported |
| Secret scan | Gitleaks 8.30.1 directory scan: no leaks found; real credentials and live service calls were not used |
| Dependency notices | 159 Go-build-list/npm-lock dependencies inventoried; zero missing notice entries after explicit documented supplements |

Native-profile legal validation: 91 retained evidence hashes; 11 linked Go
modules for both CGO-disabled Linux targets; 1,757 inline copyright/permission
blocks retained; 148 CA certificates matched to original Mozilla data in the
exact Debian source package. Real native binaries in temporary legal test
archives and synthetic scratch exports passed the verifier. Exact and vendored
source-archive verifier tests passed. Missing notices, accidental helper,
dirty-source and absent-release-input tests were rejected. These verifier tests
are not substitutes for actual Docker-built image and clean-release checks.

## Remaining gates

1. **Docker execution:** this workspace has no Docker daemon/socket. Actual
   container builds, final-image smoke, Compose startup and browser end-to-end
   login were not run. CI includes these checks; run them before publication.
   ARM64 helpers/binaries were cross-built but not executed here. Release CI
   uses QEMU to smoke each architecture's final image.
2. **License review:** the native codec introduced at `2cadac2` removes the
   Deno runtime from the intended distribution. The old unresolved Deno native
   closure is test/source/historical evidence only, NOT approved for runtime
   redistribution. The new scoped profile covers 11 linked Go modules, both
   CGO-disabled static targets, upstream MIT adaptations, Go inline notices,
   embedded htmx and scratch's separately licensed Mozilla CA data/source.
   Actual rebuilt archives, both flattened images, exact matching source
   archive and public version-specific source/network-UI offer must be checked
   before publication. Old helper-bearing snapshots are invalid. See
   `licenses/PROVENANCE.md` and `scripts/license-provenance.py --help`.
   `go-licenses` 2.0.1 also fails to recognize the downloaded Go 1.27.4 standard
   library correctly in this environment; the retained Go-module source-notice
   inventory is not presented as a successful go-licenses scan. See
   `THIRD_PARTY_NOTICES.md`.
3. **Publishing identity:** exact GitHub owner/repository and GHCR visibility
   must be confirmed. No repository, registry package or public push was made.
   `PUBLIC_RELEASE_APPROVED` defaults unset, so release publishing is disabled.
4. **Keyless signing:** the GitHub OIDC/cosign publication and verification
   workflows have been authored/linted, not exercised with real GitHub OIDC
   credentials. No signing or registry push was attempted locally. Validate
   the trusted workflow identity with the first approved release.

The next independent review should inspect source privacy, dependency notice
completeness, Compose/image execution and signed artifact verification before
the publication gate is lifted.
