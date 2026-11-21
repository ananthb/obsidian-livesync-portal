<!-- SPDX-License-Identifier: AGPL-3.0-only -->
# Obsidian LiveSync Portal

Self-service CouchDB vaults and Setup URIs for [Obsidian Self-hosted LiveSync](https://github.com/vrtmrz/obsidian-livesync).

Users log in via OpenID Connect (OIDC) to generate LiveSync Setup URIs and QR codes for Obsidian. Each user is mapped to a dedicated CouchDB instance. The portal manages databases and encryption credentials, allowing users to initialize their primary vault and create extra vaults with independent encryption keys.

## How it works

- **Dedicated CouchDB per user**: LiveSync setup URIs include administrator credentials, so each user must have an isolated CouchDB instance.
- **Default vault**: Automatically created on first login using configured credentials.
- **Extra vaults**: Users can create additional databases with independent encryption passphrases.

## Quickstart

### Docker Compose

An end-to-end example running Caddy (TLS), Dex (OIDC), CouchDB, and the portal is available under `examples/`:

```sh
export PORTAL_HOST=portal.example.com
export OIDC_HOST=id.example.com
export COUCH_HOST=couch.example.com
export DEMO_BCRYPT='<bcrypt-hash>'
bash examples/configure.sh
docker compose --env-file .env -f examples/compose.yaml up -d
```

### Binary

Download or build the static binary, import configuration, and run:

```sh
./livesync-portal configure config.json
./livesync-portal
```

By default, state is kept in SQLite at `data/portal.db`. The process listens on `:8080` and should run behind an HTTPS reverse proxy.

## Configuration

Configuration can be provided as a JSON file or imported into SQLite via `./livesync-portal configure <file>`:

```json
{
  "url": "https://portal.example.com",
  "issuer": "https://id.example.com/realms/example",
  "client_id": "obsidian-livesync-portal",
  "client_secret": "REPLACE-WITH-SECRET",
  "scopes": ["openid", "profile"],
  "sqlite": "/data/portal.db",
  "people": {
    "alice": {
      "issuer": "https://id.example.com/realms/example",
      "subject": "alice-oidc-subject",
      "credentials": {
        "host": "https://alice-couch.example.com",
        "management_url": "http://alice-couch:5984",
        "user": "admin",
        "password": "REPLACE-WITH-SECRET",
        "database": "notes-alice",
        "passphrase": "REPLACE-WITH-RANDOM-STABLE-KEY",
        "id_recovery_code": "sls-id-v1:REPLACE-WITH-64-HEX-DIGITS"
      }
    }
  }
}
```

### Key options

| Setting | Description |
|---|---|
| `url` | Public HTTPS origin of the portal |
| `issuer` | OIDC discovery issuer URL |
| `client_id` / `client_secret` | OIDC client credentials |
| `scopes` | OIDC scopes to request (defaults to `["openid", "profile"]`) |
| `people` | Map of user IDs to OIDC identities (`issuer`, `subject`) and CouchDB credentials |
| `sqlite` | Path to the SQLite database (default `data/portal.db`) |
| `listen` | Address to listen on (default `:8080`) |
| `vault` | Optional HashiCorp Vault KV v2 backend for catalog storage |

## Development

```sh
# Run tests
go test ./...

# Build binary
CGO_ENABLED=0 go build -o livesync-portal .
```

## License

First-party code is [AGPL-3.0-only](LICENSE). Third-party notices and licenses are documented in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) and `licenses/`.
