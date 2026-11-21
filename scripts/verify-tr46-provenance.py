# SPDX-License-Identifier: AGPL-3.0-only
"""Verify externally retrieved tr46 0.0.3 tarball and Unicode 8.0.0 table.

Does not download or execute upstream code. Pass paths to the two data files.
The upstream author/license-only commit evidence is retained in licenses/.
"""
import argparse
import hashlib
import json
from pathlib import Path
import tarfile


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("tarball", type=Path)
    parser.add_argument("unicode_table", type=Path)
    args = parser.parse_args()
    root = Path(__file__).resolve().parent.parent
    if hashlib.sha256(args.tarball.read_bytes()).hexdigest() != (
        "164ae1eb32cea353551bbc7f9358dcaae4ffabbe65ec37a92ca464a9570a2a0a"
    ):
        raise SystemExit("Unexpected tr46 tarball hash")
    with tarfile.open(args.tarball) as archive:
        package = json.load(archive.extractfile("package/package.json"))
        expected = json.load(archive.extractfile("package/lib/mappingTable.json"))
    if package["version"] != "0.0.3" or package["license"] != "MIT":
        raise SystemExit("Unexpected package identity/license")
    commit = json.loads(
        (root / "licenses/evidence/tr46-license-commit.json").read_text()
    )
    if (
        commit["sha"] != "3a6f29721e7063b9ffd421e461a54beae6170001"
        or [p["sha"] for p in commit["parents"]]
        != ["a8009f9ce80ff5dbe71dd71e203afe4e4c878d28"]
        or [(f["filename"], f["status"]) for f in commit["files"]]
        != [("LICENSE.md", "added")]
    ):
        raise SystemExit("Unexpected author license-only commit evidence")
    # Reproduce upstream generateMappingTable.js without running its code.
    actual = []
    text = args.unicode_table.read_text()
    if not text.startswith("# IdnaMappingTable-8.0.0.txt"):
        raise SystemExit("Expected Unicode 8.0.0 IDNA data")
    for line in text.splitlines():
        cells = [cell.strip() for cell in line.split("#")[0].split(";")]
        if len(cells) == 1:
            continue
        bounds = cells[0].split("..")
        cells[0] = [int(bounds[0], 16), int(bounds[-1], 16)]
        if len(cells) > 2:
            cells[2] = [int(value, 16) for value in cells[2].split()]
        actual.append(cells)
    if actual != expected:
        raise SystemExit("Unicode table transformation does not match npm data")
    print(f"Verified author license-only parent relationship and {len(actual)} "
          "Unicode-derived mapping rows.")
    print("Unicode source SHA-256:", hashlib.sha256(text.encode()).hexdigest())


if __name__ == "__main__":
    main()
