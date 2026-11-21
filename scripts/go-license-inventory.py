# SPDX-License-Identifier: AGPL-3.0-only
"""Record the two CGO-disabled Linux Go source closures and inline notices.

Run after go mod download. This is evidence collection, not legal clearance.
Uses the repository's Go toolchain and release build tags; executes no dependency
programs. Only writes license evidence, never application or build configuration.
"""
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess


ROOT = Path(__file__).resolve().parent.parent
APPROVED_MODULES = {
    "github.com/coreos/go-oidc/v3@v3.21.0",
    "github.com/dustin/go-humanize@v1.1.0",
    "github.com/go-jose/go-jose/v4@v4.1.9",
    "github.com/google/uuid@v1.6.0",
    "github.com/remyoudompheng/bigfft@v0.0.0-20230129092748-24d4a6f8daec",
    "golang.org/x/oauth2@v0.38.0", "golang.org/x/sys@v0.49.0",
    "modernc.org/libc@v1.80.1", "modernc.org/mathutil@v1.7.1",
    "modernc.org/memory@v1.12.1", "modernc.org/sqlite@v1.62.0"
}


def run(args, env=None):
    return subprocess.check_output(args, cwd=ROOT, env=env).decode()


def stream(text):
    decoder = json.JSONDecoder()
    while text.strip():
        text = text.lstrip()
        value, end = decoder.raw_decode(text)
        yield value
        text = text[end:]


def digest(data):
    return hashlib.sha256(data).hexdigest()


def main():
    out = ROOT / "licenses/go-runtime"
    out.mkdir(parents=True, exist_ok=True)
    inventory = json.loads((ROOT / "licenses/inventory.json").read_text())
    known = {row["package"]: row["notices"] for row in inventory["rows"]
             if row["ecosystem"] == "go"}
    goroot = Path(run(["go", "env", "GOROOT"]).strip())
    version = run(["go", "env", "GOVERSION"]).strip()
    targets = {}
    comments = {}
    assets = []
    modules = {}
    for arch in ("amd64", "arm64"):
        env = dict(os.environ, CGO_ENABLED="0", GOOS="linux", GOARCH=arch,
                   GOFLAGS="", GOEXPERIMENT="")
        packages = list(stream(run([
            "go", "list", "-mod=readonly", "-tags=netgo,osusergo", "-deps", "-json", "."
        ], env)))
        targets[arch] = sorted(pkg["ImportPath"] for pkg in packages)
        for pkg in packages:
            if pkg.get("CgoFiles"):
                raise SystemExit(f"Unexpected cgo: {pkg['ImportPath']}")
            module = pkg.get("Module", {})
            if module.get("Replace"):
                raise SystemExit("Replacement module requires independent review")
            if module.get("Version"):
                key = f"{module['Path']}@{module['Version']}"
                if key not in known:
                    raise SystemExit(f"No retained license for linked module {key}")
                modules[key] = known[key]
            if not pkg.get("Standard") and not module.get("Version"):
                continue  # First-party source already carries its SPDX headers.
            for name in pkg.get("GoFiles", []) + pkg.get("SFiles", []) + pkg.get("HFiles", []):
                path = Path(pkg["Dir"]) / name
                data = path.read_bytes()
                origin = (str(path.relative_to(goroot)) if pkg.get("Standard") else
                          f"{module['Path']}@{module['Version']}/" +
                          str(path.relative_to(Path(module["Dir"]))))
                # Preserve complete contiguous comment blocks, including notices
                # after package/import declarations (Fiat and Cephes, for example).
                for match in re.finditer(r"(?m)(?:(?:^[ \t]*//[^\n]*(?:\n|$))+)|/\*[\s\S]*?\*/", data.decode()):
                    block = match.group()
                    if re.search(r"copyright|redistribution|permission is hereby|public domain", block, re.I):
                        comments[(origin, match.start())] = (block, digest(data))
    if set(modules) != APPROVED_MODULES or version != "go1.27.4":
        raise SystemExit("Changed linked module/toolchain closure requires human license review")
    text = "Go release source inline copyright/permission notices\n"
    text += f"Toolchain: {version}; Linux amd64/arm64; CGO_ENABLED=0; netgo,osusergo.\n"
    text += "Conservative source closure; compiler dead-code elimination may exclude portions.\n\n"
    for (origin, _), (block, source_hash) in sorted(comments.items()):
        text += f"===== {origin} (source SHA-256 {source_hash}) =====\n{block}\n\n"
    notice = out / "inline-notices.txt"
    notice.write_text(text)
    assets.append({"path": notice.relative_to(ROOT).as_posix(),
                   "sha256": digest(notice.read_bytes()),
                   "source": f"Go {version} standard library and checksum-locked linked Go module sources",
                   "note": "Exact source comment blocks; module source hashes recorded beside each block"})
    for path in sorted((goroot / "src/vendor").rglob("LICENSE")):
        dest = out / "stdlib-vendor" / path.relative_to(goroot / "src/vendor")
        dest.parent.mkdir(parents=True, exist_ok=True)
        dest.write_bytes(path.read_bytes())
        assets.append({"path": dest.relative_to(ROOT).as_posix(),
                       "sha256": digest(dest.read_bytes()),
                       "source": f"https://go.googlesource.com/go/+/refs/tags/{version}/{path.relative_to(goroot)}"})
    scope = {
        "profile": "native-go-static-linux", "toolchain": version,
        "cgo_enabled": False, "tags": ["netgo", "osusergo"],
        "modules": [{"package": key, "notices": value} for key, value in sorted(modules.items())],
        "targets": targets,
        "adapted_upstreams": [
            "licenses/setup-helper.txt",
            "licenses/dependencies/npm-_vrtmrz_livesync-commonlib_0.2.0-LICENSE",
            "licenses/dependencies/npm-octagonal-wheels_0.1.57-LICENSE"
        ],
        "embedded_asset_notices": ["licenses/htmx.txt"],
        "review": "Source closure and retained notices reviewed; artifact identity/staticness, CA provenance and source-delivery checks remain per release"
    }
    path = out / "inventory.json"
    path.write_text(json.dumps(scope, indent=2) + "\n")
    assets.append({"path": path.relative_to(ROOT).as_posix(), "sha256": digest(path.read_bytes()),
                   "source": "CGO_ENABLED=0 GOOS=linux GOARCH={amd64,arm64} go list -mod=readonly -tags=netgo,osusergo -deps -json ."})
    manifest = json.loads((ROOT / "licenses/provenance.json").read_text())
    manifest["assets"] = sorted([a for a in manifest["assets"] if not a["path"].startswith("licenses/go-runtime/")] + assets,
                                key=lambda a: a["path"])
    (ROOT / "licenses/provenance.json").write_text(json.dumps(manifest, indent=2) + "\n")
    print(f"Recorded {len(modules)} linked modules and {len(comments)} copyright/permission blocks for {version}.")


if __name__ == "__main__":
    main()
