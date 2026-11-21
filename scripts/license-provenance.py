# SPDX-License-Identifier: AGPL-3.0-only
"""Verify native-Go legal evidence and optional release/source/image artifacts.

This is a scoped mechanical gate, not legal advice or publication approval.
Deno provenance is test/source evidence, never proof of native runtime clearance.
Images must be flattened docker-export tar files, not OCI archives/layer tars.
"""
import argparse
import hashlib
import io
import json
from pathlib import Path, PurePosixPath
import re
import struct
import subprocess
import sys
import tarfile
import tempfile
from urllib.parse import urlsplit

ROOT = Path(__file__).resolve().parent.parent


def require(condition, message):
    if not condition:
        raise SystemExit(message)


def sha(data):
    return hashlib.sha256(data).hexdigest()


def verify_binary(data, scope, revision=None):
    require(data[:6] == b"\x7fELF\x02\x01", "Expected a little-endian ELF64 Go binary")
    machine = struct.unpack_from("<H", data, 18)[0]
    arch = {62: "amd64", 183: "arm64"}.get(machine)
    require(arch, "Unsupported release architecture")
    offset = struct.unpack_from("<Q", data, 32)[0]
    size, count = struct.unpack_from("<HH", data, 54)
    for index in range(count):
        kind = struct.unpack_from("<I", data, offset + size * index)[0]
        require(kind not in (2, 3), "Dynamic segment/interpreter: not the reviewed static profile")
    with tempfile.NamedTemporaryFile(dir="/tmp/opencode", suffix="-license-check") as binary:
        binary.write(data)
        binary.flush()
        info = json.loads(subprocess.check_output(
            ["go", "version", "-m", "-json", binary.name], cwd=ROOT))
    require(info["GoVersion"] == scope["toolchain"], "Unreviewed Go toolchain")
    require(info["Main"]["Path"] == "github.com/ananthb/obsidian-livesync-portal", "Unexpected main module")
    deps = {f"{d['Path']}@{d['Version']}" for d in info.get("Deps", [])}
    require(not any(d.get("Replace") for d in info.get("Deps", [])), "Unreviewed replacement module")
    require(deps == {d["package"] for d in scope["modules"]}, "Binary module closure differs from reviewed inventory")
    settings = {s["Key"]: s["Value"] for s in info["Settings"]}
    require(settings.get("CGO_ENABLED") == "0", "Expected CGO_ENABLED=0")
    require(settings.get("GOOS") == "linux" and settings.get("GOARCH") == arch, "Target mismatch")
    require(set(settings.get("-tags", "").split(",")) == set(scope["tags"]), "Unreviewed build tags")
    require(not settings.get("GOEXPERIMENT"), "Unreviewed experimental Go build")
    if revision:
        require(settings.get("vcs.revision") == revision, "Binary VCS revision does not match source")
        require(settings.get("vcs.modified") == "false", "Release binary built from dirty/unidentified source")
    return arch


def entries(archive):
    result = {}
    for member in archive.getmembers():
        name = str(PurePosixPath(member.name))
        require(not name.startswith("/") and ".." not in PurePosixPath(name).parts,
                "Unsafe archive path")
        require(name not in result, f"Duplicate archive path: {name}")
        result[name] = member
    return result


def same_file(archive, members, name, expected):
    member = members.get(name)
    require(member is not None and member.isfile(), f"Missing regular file: {name}")
    require(archive.extractfile(member).read() == expected, f"File differs: {name}")


def verify_distribution(path, scope, legal_files, image=False, revision=None):
    with tarfile.open(path) as archive:
        members = entries(archive)
        prefix = "app/" if image else ""
        for name, data in legal_files.items():
            same_file(archive, members, prefix + name, data)
        binary_name = prefix + "livesync-portal"
        require(binary_name in members, f"{path}: missing application")
        arch = None
        for name, member in members.items():
            base = PurePosixPath(name).name
            require(base not in ("setup-uri", "deno", "denort", "verify-uri") and
                    not base.startswith(("denort-", "libdenort")),
                    f"Unexpected Deno/helper artifact: {name}")
            if member.issym() or member.islnk():
                raise SystemExit(f"Unreviewed distribution symlink/hardlink: {name}")
            if member.isfile():
                data = archive.extractfile(member).read()
                executable = data.startswith((b"\x7fELF", b"MZ", b"#!", b"\xcf\xfa\xed\xfe"))
                if name == binary_name:
                    arch = verify_binary(data, scope, revision)
                elif executable and not (not image and name.startswith("examples/") and data.startswith(b"#!")):
                    raise SystemExit(f"Unexpected executable in distribution: {name}")
        if image:
            ca = json.loads((ROOT / "licenses/ca-bundle/provenance.json").read_text())
            cert = members.get("etc/ssl/certs/ca-certificates.crt")
            require(cert is not None and cert.isfile(), "Missing image CA bundle")
            require(sha(archive.extractfile(cert).read()) == ca["bundle_sha256"], "Unreviewed CA bundle")
            # Scratch's approved payload: application/legal files, CA data and empty directories.
            approved = {prefix + name for name in legal_files} | {binary_name, "etc/ssl/certs/ca-certificates.crt"}
            # Docker may add these runtime metadata files to a container export;
            # they are not licenses/OS payload and must never be executables.
            approved |= {".dockerenv", "etc/hosts", "etc/hostname", "etc/resolv.conf"}
            require(all(not m.isfile() or n in approved for n, m in members.items()),
                    "Image contains unreviewed files outside scratch payload")
        else:
            approved = set(legal_files) | {binary_name, "README.md"}
            approved |= {p.relative_to(ROOT).as_posix() for p in (ROOT / "examples").rglob("*") if p.is_file()}
            require(all(not m.isfile() or n in approved for n, m in members.items()),
                    "Release archive contains unreviewed payload (including nested archives)")
    print(f"Verified {path}: static Linux {arch}, legal files and {'scratch payload' if image else 'no runtime helper'}.")
    return arch


def verify_source(path, revision):
    expected = subprocess.check_output(["git", "archive", "--format=tar", revision], cwd=ROOT)
    with tarfile.open(fileobj=io.BytesIO(expected)) as reference, tarfile.open(path) as archive:
        refs = entries(reference)
        actual = entries(archive)
        # Accommodate one top-level directory used by GitHub/GoReleaser source archives.
        candidates = [""] + sorted({n.split("/")[0] + "/" for n in actual if "/" in n})
        prefixes = [p for p in candidates if p + "go.mod" in actual]
        require(len(prefixes) == 1, "Ambiguous or missing source archive root")
        prefix = prefixes[0]
        expected_files = {n for n, m in refs.items() if m.isfile()}
        actual_files = {n[len(prefix):] for n, m in actual.items() if m.isfile() and n.startswith(prefix)}
        additions = actual_files - expected_files
        require(all(n.startswith(("vendor/", "build-inputs/")) for n in additions),
                "Unreviewed additions to exact source archive")
        require(expected_files <= actual_files, "Source archive omits tracked source/build/legal assets")
        for name in expected_files:
            same_file(archive, actual, prefix + name, reference.extractfile(refs[name]).read())
        if additions:
            for name in ("go.mod", "go.sum"):
                require((ROOT / name).read_bytes() == reference.extractfile(refs[name]).read(),
                        "Current module locks differ from source revision; cannot validate vendor tree")
            with tempfile.TemporaryDirectory(dir="/tmp/opencode", prefix="legal-source-") as temp:
                vendor = Path(temp) / "vendor"
                subprocess.check_call(["go", "mod", "vendor", "-o", str(vendor)], cwd=ROOT)
                vendor_files = {"vendor/" + p.relative_to(vendor).as_posix(): p.read_bytes()
                                for p in vendor.rglob("*") if p.is_file()}
            ca_dir = ROOT / "licenses/ca-bundle"
            required = set(vendor_files) | {
                "build-inputs/manifest.json",
                "build-inputs/etc/ssl/certs/ca-certificates.crt",
                "build-inputs/usr/share/doc/ca-certificates/copyright"
            }
            require(additions == required, "Source vendor/build-input file set is incomplete or unexpected")
            for name, data in vendor_files.items():
                same_file(archive, actual, prefix + name, data)
            same_file(archive, actual, prefix + "build-inputs/etc/ssl/certs/ca-certificates.crt",
                      (ca_dir / "certificates.pem").read_bytes())
            same_file(archive, actual, prefix + "build-inputs/usr/share/doc/ca-certificates/copyright",
                      (ca_dir / "debian-copyright").read_bytes())
            inputs = json.load(archive.extractfile(actual[prefix + "build-inputs/manifest.json"]))
            ca = json.loads((ca_dir / "provenance.json").read_text())
            require(inputs["source_commit"] == revision and inputs["go"] == "1.27.4" and
                    inputs["ca_bundle_sha256"] == ca["bundle_sha256"] and
                    inputs["builder_image"] == "golang:1.27.4-bookworm@" + ca["builder_index"],
                    "Source build-input manifest does not match reviewed inputs")
    print(f"Verified exact Corresponding Source archive for {revision} (all tracked files/build/legal assets).")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--release", action="store_true")
    parser.add_argument("--images", action="store_true", help="Require both flattened image exports for release")
    parser.add_argument("--archive", action="append", type=Path, default=[])
    parser.add_argument("--image-tar", action="append", type=Path, default=[])
    parser.add_argument("--source-archive", type=Path)
    parser.add_argument("--revision", help="Exact 40-character release source commit")
    parser.add_argument("--source-url", help="Public version-specific source URL offered with release/deployment")
    args = parser.parse_args()
    manifest = json.loads((ROOT / "licenses/provenance.json").read_text())
    for asset in manifest["assets"]:
        require(sha((ROOT / asset["path"]).read_bytes()) == asset["sha256"], asset["path"])
    subprocess.check_call([sys.executable, str(ROOT / "scripts/verify-ca-provenance.py")], cwd=ROOT)
    inventory = json.loads((ROOT / "licenses/inventory.json").read_text())
    require(not inventory["missing"], "Missing source/test dependency notices")
    for row in inventory["rows"]:
        require(row["notices"] and all((ROOT / n).is_file() for n in row["notices"]), row["package"])
    scope = json.loads((ROOT / "licenses/go-runtime/inventory.json").read_text())
    for row in scope["modules"]:
        require(all((ROOT / n).is_file() for n in row["notices"]), row["package"])
    require((ROOT / "licenses/setup-helper.txt").read_bytes() == (ROOT / "setup/upstream/LICENSE").read_bytes(), "Helper notice drift")
    require((ROOT / "licenses/htmx.txt").read_bytes() == (ROOT / "static/htmx.LICENSE").read_bytes(), "htmx notice drift")
    for notice in scope["adapted_upstreams"]:
        require((ROOT / notice).is_file(), notice)
    legal_paths = [ROOT / "LICENSE", ROOT / "THIRD_PARTY_NOTICES.md"]
    legal_paths += [p for p in (ROOT / "licenses").rglob("*") if p.is_file()]
    legal = {p.relative_to(ROOT).as_posix(): p.read_bytes() for p in legal_paths}
    if args.revision:
        require(re.fullmatch(r"[0-9a-f]{40}", args.revision), "Expected exact source commit")
    arches = {verify_distribution(p, scope, legal, revision=args.revision) for p in args.archive}
    images = {verify_distribution(p, scope, legal, image=True, revision=args.revision) for p in args.image_tar}
    if args.source_archive:
        require(args.revision, "Source verification requires --revision")
        verify_source(args.source_archive, args.revision)
    if args.release:
        require(manifest["profile"] == "native-go-static-linux", "Unreviewed distribution profile")
        require(not manifest["publication_blockers"], "Open profile blockers: " + "; ".join(manifest["publication_blockers"]))
        require(arches == {"amd64", "arm64"}, "Release requires both architecture archives")
        require(args.source_archive and args.revision, "Release requires exact matching source archive/revision")
        url = urlsplit(args.source_url or "")
        release_asset = re.fullmatch(
            r"/ananthb/obsidian-livesync-portal/releases/download/v([^/]+)/obsidian-livesync-portal_([^/]+)_source\.tar\.gz", url.path)
        exact_source = args.revision in url.path
        if release_asset:
            exact_source = (release_asset[1] == release_asset[2] and
                            PurePosixPath(url.path).name == args.source_archive.name)
        require(url.scheme == "https" and url.hostname == "github.com" and not url.username and
                url.path.startswith("/ananthb/obsidian-livesync-portal/") and exact_source and not url.query and not url.fragment,
                "Release requires the canonical exact-commit or matching versioned source-asset URL")
        if args.images:
            require(images == {"amd64", "arm64"}, "Image release requires both flattened image exports")
    print(f"Verified {len(manifest['assets'])} evidence hashes; {len(scope['modules'])} reviewed linked Go modules.")
    print("Deno/Rust/V8 inventory is TEST/SOURCE-ONLY; native closure was NOT cleared for redistribution.")
    print("Artifact checks are local evidence, not proof of public source availability or deployment section-13 compliance.")


if __name__ == "__main__":
    main()
