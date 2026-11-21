# SPDX-License-Identifier: AGPL-3.0-only
"""Record already digest-verified crane exports; never run as an approval step."""
import hashlib
import json
from pathlib import Path, PurePosixPath
import sys
import tarfile

root = Path(__file__).resolve().parent.parent
dest = root / "licenses/distroless"
inventory = {"image": "gcr.io/distroless/static-debian13", "index":
             "sha256:aafe993665bf6200db38a02c11a9351e782a84b6a45e7f33b6695638c88e3931",
             "upstream": "https://github.com/GoogleContainerTools/distroless",
             "review_status": "pending independent legal review", "architectures": {}}
index = json.loads((dest / "evidence/index.json").read_text())
for arch in ("amd64", "arm64"):
    descriptor = next(d for d in index["manifests"] if d["platform"]["architecture"] == arch)
    files = {}
    with tarfile.open(Path(sys.argv[1]) / f"{arch}.tar") as archive:
        for member in archive:
            name = str(PurePosixPath(member.name))
            assert not name.startswith("/") and ".." not in PurePosixPath(name).parts
            row = {"type": member.type.decode(), "mode": member.mode,
                   "uid": member.uid, "gid": member.gid}
            if member.isfile():
                data = archive.extractfile(member).read()
                row["sha256"] = hashlib.sha256(data).hexdigest()
                if name.startswith(("usr/share/doc/", "usr/share/common-licenses/", "var/lib/dpkg/status.d/")):
                    notice = dest / "base-notices" / name
                    notice.parent.mkdir(parents=True, exist_ok=True)
                    notice.write_bytes(data)
            if member.issym() or member.islnk():
                row["target"] = member.linkname
            assert name not in files
            files[name] = row
    inventory["architectures"][arch] = {"descriptor": descriptor, "files": files}
(dest / "inventory.json").write_text(json.dumps(inventory, indent=2, sort_keys=True) + "\n")
