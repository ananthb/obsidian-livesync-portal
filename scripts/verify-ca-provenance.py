# SPDX-License-Identifier: AGPL-3.0-only
"""Check retained Mozilla CA data against the exact Debian source package.

This checks data origin, not certificate trust policy or the upstream PGP signer.
"""
import base64
import hashlib
import io
import json
from pathlib import Path
import re
import tarfile


def main():
    root = Path(__file__).resolve().parent.parent / "licenses/ca-bundle"
    meta = json.loads((root / "provenance.json").read_text())
    source = (root / "upstream-source.tar.xz").read_bytes()
    digest = hashlib.sha256(source).hexdigest()
    dsc = (root / "upstream-source.dsc").read_text()
    if not re.search(rf"^ {digest} {len(source)} ca-certificates_{re.escape(meta['version'])}\.tar\.xz$", dsc, re.M):
        raise SystemExit("CA source archive differs from authoritative Debian .dsc checksum")
    with tarfile.open(fileobj=io.BytesIO(source)) as archive:
        certdata = archive.extractfile("ca-certificates/mozilla/certdata.txt").read().decode()
        copyright = archive.extractfile("ca-certificates/debian/copyright").read()
    if copyright != (root / "debian-copyright").read_bytes():
        raise SystemExit("Installed CA copyright differs from source package")
    originals = set()
    for block in re.findall(r"CKA_VALUE MULTILINE_OCTAL\n(.*?)\nEND", certdata, re.S):
        originals.add(bytes(int(v, 8) for v in re.findall(r"\\([0-7]{3})", block)))
    bundle = (root / "certificates.pem").read_bytes()
    if hashlib.sha256(bundle).hexdigest() != meta["bundle_sha256"]:
        raise SystemExit("CA bundle hash drift")
    certs = {base64.b64decode(body) for body in re.findall(
        rb"-----BEGIN CERTIFICATE-----\s+(.*?)\s+-----END CERTIFICATE-----", bundle, re.S)}
    if len(certs) != meta["certificate_count"] or not certs <= originals:
        raise SystemExit("CA bundle certificates not accounted for by original Mozilla certdata")
    with tarfile.open(root / "source-certificates.tar.gz") as archive:
        supplied = set()
        for member in archive:
            if member.isfile():
                data = archive.extractfile(member).read()
                bodies = re.findall(rb"-----BEGIN CERTIFICATE-----\s+(.*?)\s+-----END CERTIFICATE-----", data, re.S)
                supplied.update(base64.b64decode(body) for body in bodies)
    if certs != supplied:
        raise SystemExit("Retained individual source certificates differ from image bundle")
    print(f"Verified {len(certs)} CA certificates against exact Mozilla data in Debian {meta['version']}; source SHA-256 {digest}.")


if __name__ == "__main__":
    main()
