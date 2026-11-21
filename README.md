<!-- SPDX-License-Identifier: AGPL-3.0-only -->
# Obsidian LiveSync Portal

Self-service CouchDB vaults and Setup URIs for Obsidian Self-hosted LiveSync.
One person owns one dedicated CouchDB server. The portal manages databases,
not people or servers. First login initializes the protected default database.
Extra vaults have independent encryption/recovery keys and durable deletion tombstones.

Public destination: [github.com/ananthb/obsidian-livesync-portal](https://github.com/ananthb/obsidian-livesync-portal).
Release images: `ghcr.io/ananthb/obsidian-livesync-portal`.
**Extraction candidate. Publication awaits independent security/license review.**

## Run

Linux amd64 and arm64. Each archive contains one static `livesync-portal`
executable, configuration examples and notices. No Deno, helper executable,
glibc, libgcc or runtime executable downloads are required. The same binaries
run on glibc-based and musl-based Linux distributions. macOS and Windows
binaries are not currently distributed. Host installations use their system
CA roots, or a configured `SSL_CERT_FILE`, for verified outbound HTTPS.

The project and archive prefix are `obsidian-livesync-portal`. The executable
deliberately keeps the shorter name `livesync-portal`. Its `setup-uri` CLI
subcommand runs the native codec in-process. Local directory names do not
affect the public repository identity.

```sh
./livesync-portal configure /secure/config.json
./livesync-portal
```

Default configuration and catalogs persist in `data/portal.db`. `configure`
imports validated configuration into SQLite; `PORTAL_DB` changes that import
destination. For startup use `PORTAL_CONFIG=sqlite:/absolute/path/portal.db`.
Alternatively `PORTAL_CONFIG=/secure/config.json` reads an externally managed
JSON secret file. JSON loading is independent of the catalog adapter. Changes
require a restart; configuration imports refuse while the process holds its lock.
Configuration, catalogs, WAL files and backups contain credentials: protect them.

Use one replica on persistent **local** storage. The process holds an exclusive
advisory lock beside SQLite for its lifetime and serializes database side
effects. SQLite uses WAL, FULL synchronous commits and versioned transactional
CAS. Do not put it on NFS, share it across hosts, delete the lock file while
running, or run separate state copies against the same CouchDB servers. The
lock cannot protect different paths or separate machines. Vault mode still
requires the same persistent local lock; it is not a multi-replica mode.

Put the listener behind a trusted HTTPS reverse proxy. Only port 8080 on the
private network should reach the process. External portal, OIDC and public
CouchDB URLs must be HTTPS. Internal CouchDB/Vault HTTP is allowed only as
explicit operator configuration on a private network. Trust private CAs with
`SSL_CERT_FILE`; never disable certificate verification. `/healthz` is unauthenticated.

## Configuration

Use the generated Compose example below or supply JSON:

```json
{
  "url": "https://portal.example.invalid",
  "issuer": "https://id.example.invalid/realms/example",
  "client_id": "obsidian-livesync-portal",
  "client_secret": "REPLACE-WITH-SECRET",
  "scopes": ["openid", "profile"],
  "sqlite": "/data/portal.db",
  "people": {
    "alice": {
      "issuer": "https://id.example.invalid/realms/example",
      "subject": "immutable-provider-subject",
      "credentials": {
        "host": "https://alice-couch.example.invalid",
        "management_url": "http://alice-couch:5984",
        "user": "server-owner",
        "password": "REPLACE-WITH-SECRET",
        "database": "notes-alice",
        "passphrase": "REPLACE-WITH-RANDOM-STABLE-KEY",
        "id_recovery_code": "sls-id-v1:REPLACE-WITH-64-LOWERCASE-HEX-DIGITS"
      }
    }
  }
}
```

| Setting | Meaning |
|---|---|
| `url` | Exact HTTPS origin; register `url + /callback` with the confidential OIDC client |
| `issuer` | Exact discovery issuer; Keycloak, Dex, Authentik, Entra ID, Google and standard OIDC providers need no routing-specific code |
| `scopes` | Must contain `openid`; defaults to `openid, profile`; request provider scopes needed for your claim policy |
| `groups`, `groups_claim` | Optional required groups; claim defaults to `groups`; accepts a string or string array, not nested paths |
| `group_match` | `any` (default) or `all` required groups; an empty list disables group admission |
| `require_verified_email` | Optional boolean claim check; defaults false; not an ownership/linking mechanism |
| `required_claims` | Exact top-level string claim/value checks, all must match |
| `people` | External eligible-user mapping; keys are generic lowercase user IDs, not emails or provider IDs |
| `listen` | Private HTTP listener, default `:8080` |
| `source_url` | Optional HTTPS source-offer link shown in the UI; modified deployments must point this to corresponding source |
| `sqlite` | Persistent local catalog/lock path; default `data/portal.db` |

The portal URL is an origin without a trailing slash. OIDC issuers and their
discovered HTTPS endpoints may contain paths and trailing slashes. The exact
issuer string, including its trailing slash, is preserved for verification and
ownership. Userinfo and fragments are rejected; issuers cannot contain queries.
Configured/discovered OAuth endpoints can contain query parameters.

Admission always requires an explicit mapped `(issuer, subject)` **and** every
configured claim policy. Email is never used to find, link or transfer an
account. Claim absence or wrong types deny required policies. A person without
email claims can log in when no email policy is enabled. Changing a subject or
issuer refuses existing catalog ownership; there is no automatic migration.
Removed users lose access after restart; sessions expire after one hour and are
memory-only. Provider/group revocation is evaluated on login, not continuously.
Restart to revoke existing portal sessions immediately. This does not revoke
already exported CouchDB credentials or devices.

OIDC verifies signatures, issuer, audience, expiry and nonce. Each short-lived
login uses single-use browser-bound state and S256 PKCE. Mutations require an
authenticated session, exact configured Origin and a CSRF token. Cookies use
Secure, HttpOnly, SameSite=Lax and the `__Host-` prefix. Every response is
`no-store`. Requests cannot select server endpoints; only configured endpoints
and trusted discovery are used. Redirects from backend HTTP calls are refused.
Do not configure an untrusted discovery issuer. A compromised issuer is part
of your authentication trust boundary and can advertise malicious endpoints.

## CouchDB and recovery

Each person must have a **dedicated CouchDB instance with server-administrator
credentials**. The account name is configurable, not necessarily `admin`.
Extra database names are `<configured-default-database>-<random-id>`.
The portal rejects shared management URLs, but aliases can hide a shared
server: the operator must enforce the dedicated-instance boundary.

Setup URIs include this person's full administrator credentials. All of that
person's vaults therefore share a credential/blast radius; they are not access
isolated. Never deploy multiple people on one server with a shared admin.
Restricted database credentials are not promised to support LiveSync remote
rebuild operations. The portal never provisions CouchDB users or servers and
never calls a Kubernetes API.

Default encryption/recovery settings are externally supplied and stable.
Extra-vault keys are generated once and stored before any database side effect.
Operations persist `creating`/`deleting` intent, use idempotent CouchDB changes,
then commit `active`/`deleted`. Retry pending operations after failures, including
process death between the CouchDB call and metadata commit. Do not manually
reuse a tombstoned database name. Deleted keys are removed from current
metadata, not SQLite free pages, WAL history, backups or devices. Protect and
retain encrypted backups according to your deletion policy. Back up SQLite
with its online backup mechanism or stop the service before copying all files.

The portal cannot discover encryption changes made on a device. If device
settings changed, export from that device instead of re-minting here.

## Optional Vault KV v2 catalogs

Add this block; other configuration and per-person server credentials stay in
the independently managed configuration file/SQLite configuration:

```json
{"vault":{"url":"https://vault.example.invalid","mount":"kv","path":"portal/catalogs","token_file":"/run/secrets/vault-token"}}
```

For Kubernetes authentication replace `token_file` with `jwt_file`, `auth_mount`
and `role`. For example `jwt_file: /run/secrets/workload.jwt`,
`auth_mount: kubernetes`, `role: obsidian-livesync-portal`. Exactly one authentication
method is required. Tokens must be renewed/rotated by your secret manager;
ordinary token files are read at startup. Kubernetes JWTs are reread and a
short-lived Vault token is cached for half its lease. This calls Vault's login
endpoint, not the Kubernetes API. No Kubernetes dependency is required.

Grant only `read, create, update` on `kv/data/portal/catalogs/*`; require KV v2
CAS. Do not grant config-management, people-management, delete or list rights.
Use a fresh path when extracting a deployment; no unsafe identity migration is
provided. Restarting a deleted/missing catalog cannot recover extra-vault keys.

## Compose with a synthetic identity

The example runs Caddy HTTPS, Dex with one synthetic local user, a dedicated
LiveSync-configured CouchDB, and the portal. Use three real DNS names pointing
to the Docker host, reachable on 80/443, including from the containers. Caddy
obtains public certificates. There are no HTTP/localhost authentication
defaults and no known demo password. Use a real identity provider for production.

```sh
export PORTAL_HOST=portal.your-domain.example
export OIDC_HOST=id.your-domain.example
export COUCH_HOST=couch.your-domain.example
# Set a bcrypt hash for a unique demo password, using caddy or htpasswd.
export DEMO_BCRYPT='YOUR-BCRYPT-HASH'
bash examples/configure.sh
docker compose --env-file .env -f examples/compose.yaml up --build -d
```

Login as `alice@example.invalid`. The generated mapping includes Dex's encoded
local-connector subject, not the email. The generator refuses to overwrite
existing files and writes secrets below a mode-0700 directory. Docker Compose
file-backed secrets are not encrypted at rest; restrict the host and encrypt
the disk. Mounts are readable inside their intended containers. Named volumes
retain CouchDB, Dex, portal and TLS state. Do not run `down -v` unless you intend
to destroy them. Browser CORS localhost origins in `couchdb.ini` are Obsidian's
local application origins, not insecure public service URLs.
The example configuration generator needs Bash, jq and OpenSSL, not Deno.

## Build, test and release

Go **1.27.4**, GoReleaser OSS **2.20.3**. Deno **2.9.8** is test-only.
Go's authoritative `https://go.dev/dl/?mode=json` reported 1.27.4 and the
downloaded toolchain verified it. Setup generation is native Go using standard
library PBKDF2, HKDF and AES-GCM. It supports the scoped persistent CouchDB
payload, without compression, S3/P2P or ephemeral modes. Mandatory bidirectional
tests compare complete payloads against the real frozen upstream generator and
decoder, including URL/Unicode encoding and saved-connection parsing. Only
these tests use integrity-locked npm dependencies. htmx 4.0.2 remains local.

```sh
bash scripts/test-interop.sh
SETUP_INTEROP_REQUIRED=1 go test -race ./...
go vet ./...
RELEASE_OWNER=ananthb RELEASE_REPO=obsidian-livesync-portal goreleaser check
RELEASE_OWNER=ananthb RELEASE_REPO=obsidian-livesync-portal goreleaser release --snapshot --clean --skip=publish,sign,sbom
docker build -t obsidian-livesync-portal:test .
bash scripts/image-smoke.sh obsidian-livesync-portal:test
```

Go race tests need a C compiler; production builds and the SQLite driver are
pure Go with `CGO_ENABLED=0`. CI inspects both ELF binaries for absent
interpreters and dynamic dependencies. Deno interoperability is required in
both test and tag-release workflows, not an optional release check.

The image is `scratch`, runs as UID/GID 65532, and contains the static binary,
notices, a writable `/data` directory, and CA roots. Go's pure-Go DNS resolver
uses Docker's injected `/etc/resolv.conf`; no libc/NSS is needed. The CA bundle
is the reviewed, byte-identical extraction from the digest-pinned official
Go 1.27.4 Bookworm build image, retained in `licenses/ca-bundle/certificates.pem`.
Both image builds copy those verified bytes, not a mutable host trust store.
No Debian executables or shared libraries are
copied into the runtime image. This CA data still has its own upstream/package
licensing obligations; scratch does not remove those obligations. Its package
copyright notice and complete CA source are retained under `/app/licenses/ca-bundle/`.

`scripts/package-source.sh` creates a separate checksummed source release
archive from the exact clean Git commit. It includes all tracked source and
upstream test fixtures, vendored Go dependencies, build/retrieval scripts,
and the exact build-image CA data plus its package copyright notice and a
hash/provenance manifest. The binary source and test-only npm graph are distinct:
`scripts/fetch-build-inputs.sh --interop` retrieves the pinned optional oracle
inputs, while a native build needs only Go dependencies. The source packager
uses the retained, digest-verified trust inputs and produces deterministic
archives; it does not require Docker or copy an unreviewed machine's CA roots.

Source assets are named `obsidian-livesync-portal_VERSION_source.tar.gz`, covered
by signed checksums, and independently signed. After tests and archive/source
legal checks pass, a source-only GitHub release at `source-COMMIT` publishes them
before the first image push. The workflow downloads both source and signature
without credentials and verifies the exact bytes and trusted signature before
proceeding. Point `source_url` at that exact deployed version's source
archive, including your modifications, not a moving default branch. Official
tagged binaries default this link to their matching source release asset;
development and modified deployments must configure their own source offer. Build from
the source archive with `CGO_ENABLED=0 go build -mod=vendor -tags=netgo,osusergo .`.

Tag pushes `v*` on GitHub run one serialized, fail-closed release job:
Release tags use `vMAJOR.MINOR.PATCH` with an optional `-prerelease` suffix.
Build-metadata `+` suffixes are not accepted as OCI version tags.

1. Verify legal source evidence normally, independently of the repository
   approval variable. Then verify that the tag still targets the checked-out commit and that no finalized
   release exists. Run Go race/vet, native/upstream interop and Compose-render checks.
2. Run GoReleaser OSS with `--skip=publish`. Build archives, SBOMs and checksums
   locally, then smoke the single installed amd64 binary. Inspect both static
   ELFs and package exact matching build source with vendored dependencies.
   Require the artifact-aware legal `--release` gate with both archives, exact
   source archive/revision, and the canonical source URL. Sign and verify the
   exact source archive, publish/reuse the immutable source-only release, and
   anonymously verify its bytes and signature before any candidate image push.
3. Build each architecture's image once from its exact archive. Push unique
   commit/run/attempt **candidate** tags with BuildKit SBOM and provenance.
4. Pull those exact build-produced digests. Smoke both images offline by digest,
   using matching-architecture test executables. The amd64 runner uses QEMU for
   arm64; an arm64 cross-build alone is not considered a successful image test.
5. Flatten-export those same image digests without rebuilding. Require the
   legal `--release --images` gate on both exports, archives and exact source.
   Create the candidate multiarch manifest from the tested digests and capture
   its digest directly from Buildx's metadata file. Keyless-sign both platform
   digests, the manifest digest, checksums and the image-digest release asset.
6. Verify signatures locally and recheck the immutable public source release,
   then create/update an application **draft** release, upload its
   artifacts, download them again, and verify signatures/checksums again.
7. Promote the same manifest by digest without rebuilding. Require the promoted
   descriptor's digest to match. Recheck tag/release state and unauthenticated
   source availability against the source checksum, then finalize the
   GitHub release as the last operation.

Candidates can be visible in GHCR before their image tests pass; they are not releases.
Their validated exact source is already public before they are pushed.
A failure never finalizes the application release. Failed attempts may leave candidates,
a public source-only release, or an application draft. Rerunning the same unmodified tag uses new candidate tags and replaces
only the existing draft's assets after the new build/tests/signatures succeed.
An already finalized release is refused, not overwritten. Finalization is not a
transaction across GitHub and GHCR: a failed final API call can leave a promoted
image tag and an unfinalized draft. Trust the signed digest, not a mutable tag.
Published source assets are reused only when byte-identical with valid signatures;
they are never clobbered. A mismatch aborts the release.

Configure a GitHub tag ruleset that blocks updates/deletion of `v*` and `source-*` tags and
restricts tag creation to release maintainers. Do not move a failed release tag;
rerun its original commit or create a new version. Keep other publishers from
writing these release assets/tags outside this serialized workflow.

Signed checksum and image-digest bundles are release assets. Checksums cover
the archives' binaries, source and notices. Use immutable digests; version tags can be overwritten by registry
administrators and must not be your trust anchor. No `latest` tag is published.

The confirmed destination is `ananthb/obsidian-livesync-portal` on GitHub and
GHCR. Release assets appear at
[GitHub Releases](https://github.com/ananthb/obsidian-livesync-portal/releases).
Archive URLs use
`https://github.com/ananthb/obsidian-livesync-portal/releases/download/vVERSION/obsidian-livesync-portal_VERSION_linux_ARCH.tar.gz`
with `ARCH` set to `amd64` or `arm64`.
Exact source URLs use
`https://github.com/ananthb/obsidian-livesync-portal/releases/download/source-COMMIT/obsidian-livesync-portal_VERSION_source.tar.gz`.
The workflows derive matching repository/registry metadata from
`github.repository`. They use **GitHub Actions OIDC + GHCR**.
Release publishing needs contents/packages write and id-token write; PR tests
have only contents read and no secrets. Protect tags and review workflow changes
before enabling publishing. Repository/registry
creation and public pushes are intentionally not part of extraction.
Publishing is disabled until the destination owner sets the repository variable
`PUBLIC_RELEASE_APPROVED=true` after independent security/license review and
destination approval. Subsequent tag releases are automatic.
The approval variable alone cannot authorize publication: the legal release
gate runs before any public artifact/image publication and again in release
script guards. Its source/notice/CA scope is owned by the independent licensing
review; removing the shipped Deno runtime does not automatically clear it.
Unpublished local snapshots can still be
built and checked for complete retained legal assets without lifting this gate.

The release-script tests use mocked CLIs to exercise ordering and fail-closed
behavior, including smoke/sign/upload/promotion failures and draft retries.
They do not prove Docker execution or GitHub OIDC integration. Real image and
GitHub signing/finalization checks remain pending until the workflow runs in
the approved repository; a Docker-less local environment is not a CI pass.

Verify with a trusted explicit repository, tag, and workflow identity:

```sh
identity="https://github.com/ananthb/obsidian-livesync-portal/.github/workflows/release.yml@refs/tags/v1.0.0"
cosign verify-blob --bundle checksums.txt.sigstore.json \
  --certificate-identity "$identity" --certificate-oidc-issuer https://token.actions.githubusercontent.com checksums.txt
sha256sum --check checksums.txt
cosign verify-blob --bundle image-digest.txt.sigstore.json \
  --certificate-identity "$identity" --certificate-oidc-issuer https://token.actions.githubusercontent.com image-digest.txt
cosign verify --certificate-identity "$identity" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com ghcr.io/ananthb/obsidian-livesync-portal@sha256:TRUSTED-DIGEST
```

Never derive a trusted owner, workflow or digest solely from an untrusted
download. Download all checksum-listed assets before checking them. Inspect
BuildKit attestations with `docker buildx imagetools inspect`.
The `verify release` manual workflow performs these checks with read-only
permissions. Supply the release tag and an independently trusted manifest
digest; it refuses a digest that differs from the signed release asset.

## License

First-party code is **AGPL-3.0-only**, not “or later”. Hosting a modified version
requires offering corresponding source to its network users. Provide the exact
release source and build instructions with your deployment. Third-party code
keeps its original license: see [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)
and `licenses/`. Obsidian and LiveSync names do not imply endorsement.
