# SPDX-License-Identifier: AGPL-3.0-only
"""Copy only tracked legal evidence, never the working tree or configuration."""
from pathlib import Path
import shutil
import subprocess

root = Path(__file__).resolve().parent.parent
paths = subprocess.check_output(
    ["git", "ls-files", "-z", "LICENSE", "THIRD_PARTY_NOTICES.md", "licenses/"], cwd=root
).decode().strip("\0").split("\0")
dest = root / "kodata"
expected = set(paths)
if dest.exists():
    if dest.is_symlink() or any(p.is_symlink() for p in dest.rglob("*")):
        raise SystemExit("Unexpected kodata link; refusing to follow it")
    actual = {p.relative_to(dest).as_posix() for p in dest.rglob("*") if not p.is_dir()}
    if not actual <= expected:
        raise SystemExit("Unexpected kodata assets; refusing to package them")
for name in paths:
    source = root / name
    if not source.is_file() or source.is_symlink():
        raise SystemExit(f"Expected tracked regular legal file: {name}")
    target = dest / name
    if target.is_symlink():
        raise SystemExit(f"Unexpected kodata link: {name}")
    target.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(source, target)
