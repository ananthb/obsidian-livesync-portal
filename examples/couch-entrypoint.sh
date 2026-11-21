#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
set -euo pipefail
# CouchDB's entrypoint chowns its config tree; keep read-only secret mounts outside it.
cp /run/livesync.ini /opt/couchdb/etc/local.d/10-livesync.ini
cp /run/secrets/couch_config /opt/couchdb/etc/local.d/20-credentials.ini
exec /docker-entrypoint.sh /opt/couchdb/bin/couchdb
