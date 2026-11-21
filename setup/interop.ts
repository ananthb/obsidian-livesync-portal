// SPDX-License-Identifier: AGPL-3.0-only
// Test-only oracle: never compiled or included in release artifacts.
import { generateSetupURI } from "./upstream/generate_setup_uri.ts";
import { decodeSettingsFromSetupURI } from "./upstream/livesync-commonlib.ts";
import { decryptString } from "npm:@vrtmrz/livesync-commonlib@0.2.0/compat/encryption/stringEncryption";
import { ConnectionStringParser } from "npm:@vrtmrz/livesync-commonlib@0.2.0/compat/common/ConnectionString";

const input = JSON.parse(await new Response(Deno.stdin.readable).text());
const result = input.environment ? await generateSetupURI(input.environment) : input.result;
const raw = JSON.parse(await decryptString(
  decodeURIComponent(result.setupURI.slice("obsidian://setuplivesync?settings=".length)),
  result.setupPassphrase,
));
const decoded = await decodeSettingsFromSetupURI(result.setupURI, result.setupPassphrase);
const remote = Object.values(raw.remoteConfigurations)[0] as { uri: string };
console.log(JSON.stringify({ result, raw, decoded, connection: ConnectionStringParser.parse(remote.uri) }));
