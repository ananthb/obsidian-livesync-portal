// SPDX-License-Identifier: AGPL-3.0-only
import { join } from "node:path";

// Copy notices from downloaded dependency sources, not guessed SPDX labels.
const out = "licenses/dependencies";
await Deno.mkdir(out, { recursive: true });
const rows: {
  ecosystem: string;
  package: string;
  declared: string;
  notices: string[];
}[] = [];
const missing: string[] = [];
async function collect(
  ecosystem: string,
  name: string,
  dir: string,
  declared: string,
) {
  const notices: string[] = [];
  const stem = `${ecosystem}-${name.replaceAll(/[^a-zA-Z0-9._-]/g, "_")}`;
  for await (const entry of Deno.readDir(dir)) {
    if (
      entry.isFile &&
      /^(unlicense|licen[cs]e|copying|notice|copyright)([._-].*)?$/i.test(
        entry.name,
      )
    ) {
      const dest = `${out}/${stem}-${entry.name}`;
      try {
        await Deno.chmod(dest, 0o644);
      } catch { /* The first audit creates this path. */ }
      await Deno.copyFile(join(dir, entry.name), dest);
      await Deno.chmod(dest, 0o644);
      notices.push(dest);
    }
  }
  if (notices.length === 0) {
    try {
      const text = await Deno.readTextFile(join(dir, "README.md"));
      if (/Permission is hereby granted/i.test(text)) {
        const dest = `${out}/${stem}-README.md`;
        await Deno.writeTextFile(dest, text);
        notices.push(dest);
      }
    } catch { /* Some packages publish no README. */ }
  }
  if (notices.length === 0) {
    const overrides: Record<string, string[]> = {
      "@aws-sdk/credential-provider-http@3.972.86": ["licenses/Apache-2.0.txt"],
      "@aws-sdk/credential-provider-login@3.972.92": [
        "licenses/Apache-2.0.txt",
      ],
      "@aws-sdk/nested-clients@3.998.3": ["licenses/Apache-2.0.txt"],
      "pouchdb-wrappers@5.0.0": ["licenses/Apache-2.0.txt"],
      "qrcode-generator@1.5.2": ["licenses/qrcode-generator.txt"],
      "tr46@0.0.3": [
        "licenses/tr46.txt",
        "licenses/tr46-unicode.txt",
        "licenses/evidence/tr46-unicode-header.txt",
      ],
    };
    for (const file of overrides[name] || []) {
      await Deno.stat(file);
      notices.push(file);
    }
  }
  if (notices.length === 0) missing.push(`${ecosystem}:${name}`);
  rows.push({ ecosystem, package: name, declared, notices });
}
const go = await new Deno.Command("go", {
  args: ["list", "-m", "-f", "{{.Path}}|{{.Version}}|{{.Dir}}", "all"],
  stdout: "piped",
}).output();
if (!go.success) throw new Error("go module enumeration failed");
for (const line of new TextDecoder().decode(go.stdout).trim().split("\n")) {
  const [path, version, dir] = line.split("|");
  if (version) {
    if (!dir) {
      throw new Error(`Run go mod download all first: ${path}@${version}`);
    }
    await collect("go", `${path}@${version}`, dir, "See retained license text");
  }
}
const info = await new Deno.Command("deno", {
  args: ["info", "--json"],
  stdout: "piped",
}).output();
const cache = JSON.parse(new TextDecoder().decode(info.stdout)).npmCache;
const lock = JSON.parse(await Deno.readTextFile("setup/deno.lock"));
for (const key of Object.keys(lock.npm).sort()) {
  const split = key.lastIndexOf("@");
  const name = key.slice(0, split), version = key.slice(split + 1);
  const dir = join(cache, "registry.npmjs.org", name, version);
  const pkg = JSON.parse(await Deno.readTextFile(join(dir, "package.json")));
  await collect(
    "npm",
    key,
    dir,
    typeof pkg.license === "string"
      ? pkg.license
      : JSON.stringify(pkg.license || pkg.licenses || "UNDECLARED"),
  );
}
await Deno.writeTextFile(
  "licenses/inventory.json",
  JSON.stringify({ rows, missing }, null, 2) + "\n",
);
console.log(
  `${rows.length} dependencies audited; ${missing.length} missing top-level notices`,
);
if (missing.length) {
  console.log(missing.join("\n"));
  Deno.exit(1);
}
