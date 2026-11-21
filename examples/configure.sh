#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
set -euo pipefail
umask 077
: "${PORTAL_HOST:?}" "${OIDC_HOST:?}" "${COUCH_HOST:?}" "${DEMO_BCRYPT:?}"
for host in "$PORTAL_HOST" "$OIDC_HOST" "$COUCH_HOST"; do
  [[ "$host" =~ ^[a-z0-9][a-z0-9.-]+\.[a-z]{2,}$ && "$host" != *.invalid ]] || exit 1
done
[[ "$PORTAL_HOST" != "$OIDC_HOST" && "$PORTAL_HOST" != "$COUCH_HOST" && "$OIDC_HOST" != "$COUCH_HOST" ]]
[[ "$DEMO_BCRYPT" =~ ^\$2[aby]\$[0-9]{2}\$[./A-Za-z0-9]{53}$ ]]
for file in .env secrets/portal.json secrets/dex.json secrets/couch.ini; do
  [[ ! -e "$file" ]] || { echo 'Refusing to overwrite existing configuration' >&2; exit 1; }
done
mkdir -p -m 0700 secrets
issuer="https://$OIDC_HOST"
client_secret=$(openssl rand -hex 32)
couch_password=$(openssl rand -hex 32)
passphrase=$(openssl rand -hex 32)
recovery="sls-id-v1:$(openssl rand -hex 32)"
export issuer client_secret couch_password passphrase recovery
jq -n 'env as $e | {url:("https://"+$e.PORTAL_HOST),issuer:$e.issuer,client_id:"obsidian-livesync-portal",client_secret:$e.client_secret,scopes:["openid","profile","email"],sqlite:"/home/nonroot/portal.db",people:{alice:{issuer:$e.issuer,subject:"CgVhbGljZRIFbG9jYWw",credentials:{host:("https://"+$e.COUCH_HOST),management_url:"http://couch:5984",user:"server-owner",password:$e.couch_password,database:"notes-alice",passphrase:$e.passphrase,id_recovery_code:$e.recovery}}}}' > secrets/portal.json
jq -n 'env as $e | {issuer:$e.issuer,storage:{type:"sqlite3",config:{file:"/var/dex/dex.db"}},web:{http:"0.0.0.0:5556"},telemetry:{http:"0.0.0.0:5558"},oauth2:{responseTypes:["code"],skipApprovalScreen:true},staticClients:[{id:"obsidian-livesync-portal",secret:$e.client_secret,redirectURIs:[("https://"+$e.PORTAL_HOST+"/callback")],name:"Obsidian LiveSync Portal"}],enablePasswordDB:true,staticPasswords:[{email:"alice@example.invalid",hash:$e.DEMO_BCRYPT,username:"alice",userID:"alice"}]}' > secrets/dex.json
printf '[admins]\nserver-owner = %s\n' "$couch_password" > secrets/couch.ini
chmod 0644 secrets/portal.json secrets/dex.json secrets/couch.ini
printf 'PORTAL_HOST=%s\nOIDC_HOST=%s\nCOUCH_HOST=%s\n' "$PORTAL_HOST" "$OIDC_HOST" "$COUCH_HOST" > .env
echo 'Created private demo configuration. Login as alice@example.invalid with your chosen password.'
