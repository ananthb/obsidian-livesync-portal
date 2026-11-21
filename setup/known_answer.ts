// SPDX-License-Identifier: AGPL-3.0-only
// Independent WebCrypto known-answer calculation. Synthetic data only.
const header = Uint8Array.from({ length: 76 }, (_, i) => i);
const text = new TextEncoder();
const material = await crypto.subtle.importKey("raw", text.encode("pāss🔐!+%"), "PBKDF2", false, ["deriveBits"]);
const master = await crypto.subtle.deriveBits({ name: "PBKDF2", salt: header.slice(0, 32), iterations: 310000, hash: "SHA-256" }, material, 256);
const hkdf = await crypto.subtle.importKey("raw", master, "HKDF", false, ["deriveKey"]);
const key = await crypto.subtle.deriveKey({ name: "HKDF", salt: header.slice(44, 76), info: new Uint8Array(), hash: "SHA-256" }, hkdf, { name: "AES-GCM", length: 256 }, false, ["encrypt"]);
const ciphertext = new Uint8Array(await crypto.subtle.encrypt({ name: "AES-GCM", iv: header.slice(32, 44), tagLength: 128 }, key, text.encode('{"fixture":"雪! +%"}')));
console.log(btoa(String.fromCharCode(...header, ...ciphertext)));
