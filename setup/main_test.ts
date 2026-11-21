// SPDX-License-Identifier: AGPL-3.0-only
import { generateSetupURI } from "./upstream/generate_setup_uri.ts";
import { decodeSettingsFromSetupURI } from "./upstream/livesync-commonlib.ts";

Deno.test("persistent URI decrypts with the plugin commonlib, preserving keys", async () => {
  const fixture = {
    LIVESYNC_HOST: "https://couch.example.invalid",
    LIVESYNC_USER: "admin",
    LIVESYNC_PASS: "dummy-couchdb-password",
    LIVESYNC_DB: "obsidian-dummy-0123456789abcdef0123456789abcdef",
    LIVESYNC_PASSPHRASE: "dummy-stable-e2ee",
    id_recovery_code: `sls-id-v1:${"ab".repeat(32)}`,
    id_mode: "random",
    uri_mode: "persistent",
    uri_passphrase: "dummy-transport-password",
  };
  const generated = await generateSetupURI(fixture);
  if (!generated.setupURI.startsWith("obsidian://setuplivesync?")) throw new Error("not a setup URI");
  const settings = await decodeSettingsFromSetupURI(generated.setupURI, fixture.uri_passphrase);
  if (!settings) throw new Error("decode failed");
  const expected = {
    couchDB_URI: fixture.LIVESYNC_HOST,
    couchDB_USER: fixture.LIVESYNC_USER,
    couchDB_PASSWORD: fixture.LIVESYNC_PASS,
    couchDB_DBNAME: fixture.LIVESYNC_DB,
    passphrase: fixture.LIVESYNC_PASSPHRASE,
    encrypt: true,
    usePathObfuscation: true,
    idDerivationVersion: 1,
    idDerivationKey: "ab".repeat(32),
  };
  for (const [key, value] of Object.entries(expected)) {
    if (settings[key as keyof typeof settings] !== value) throw new Error(`mismatch: ${key}`);
  }
  const remint = await generateSetupURI(fixture);
  const decodedAgain = await decodeSettingsFromSetupURI(remint.setupURI, fixture.uri_passphrase);
  if (!decodedAgain) throw new Error("re-mint decode failed");
  if (decodedAgain.passphrase !== settings.passphrase || decodedAgain.idDerivationKey !== settings.idDerivationKey) throw new Error("unstable re-mint");
  let rejected = false;
  try { rejected = !await decodeSettingsFromSetupURI(generated.setupURI, "incorrect"); } catch { rejected = true; }
  if (!rejected) throw new Error("wrong passphrase accepted");
});
