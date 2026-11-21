# SPDX-License-Identifier: AGPL-3.0-only
"""Regression checks for Docker metadata exceptions in the image legal gate."""
import importlib.util
import io
from pathlib import Path
import tarfile
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location(
    "provenance", Path(__file__).with_name("license-provenance.py"))
provenance = importlib.util.module_from_spec(spec)
spec.loader.exec_module(provenance)


class ImageMetadataTest(unittest.TestCase):
    def verify(self, name, target, kind=tarfile.SYMTYPE, image=True, duplicate=False):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "image.tar"
            with tarfile.open(path, "w") as archive:
                for filename, data in {
                    "app/livesync-portal" if image else "livesync-portal": b"binary",
                    **({"etc/ssl/certs/ca-certificates.crt":
                        (provenance.ROOT / "licenses/ca-bundle/certificates.pem").read_bytes()}
                       if image else {}),
                }.items():
                    member = tarfile.TarInfo(filename)
                    member.size = len(data)
                    archive.addfile(member, io.BytesIO(data))
                member = tarfile.TarInfo(name)
                member.type = kind
                member.linkname = target
                archive.addfile(member)
                if duplicate:
                    archive.addfile(member)
            with patch.object(provenance, "verify_binary", return_value="amd64"):
                return provenance.verify_distribution(path, {}, {}, image=image)

    def test_exact_docker_mtab_is_allowed(self):
        self.assertEqual(self.verify("etc/mtab", "/proc/mounts"), "amd64")

    def test_other_links_are_rejected(self):
        for name, target, kind, image in [
            ("etc/mtab", "/etc/passwd", tarfile.SYMTYPE, True),
            ("etc/mtab", "/proc/../proc/mounts", tarfile.SYMTYPE, True),
            ("etc/mtab", "../../proc/mounts", tarfile.SYMTYPE, True),
            ("etc/other", "/proc/mounts", tarfile.SYMTYPE, True),
            ("etc/mtab", "/proc/mounts", tarfile.LNKTYPE, True),
            ("etc/mtab", "/proc/mounts", tarfile.SYMTYPE, False),
        ]:
            with self.subTest(name=name, target=target, kind=kind, image=image):
                with self.assertRaisesRegex(SystemExit, "Unreviewed distribution"):
                    self.verify(name, target, kind, image)

    def test_traversal_and_absolute_paths_are_rejected(self):
        for name in ("../etc/mtab", "etc/../etc/mtab", "/etc/mtab"):
            with self.subTest(name=name):
                with self.assertRaisesRegex(SystemExit, "Unsafe archive path"):
                    self.verify(name, "/proc/mounts")

    def test_duplicate_metadata_is_rejected(self):
        with self.assertRaisesRegex(SystemExit, "Duplicate archive path"):
            self.verify("etc/mtab", "/proc/mounts", duplicate=True)


if __name__ == "__main__":
    unittest.main()
