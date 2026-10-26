import hashlib
import importlib.util
import io
import json
from pathlib import Path
import tarfile
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("package", Path(__file__).with_name("package.py"))
pkg = importlib.util.module_from_spec(spec)
spec.loader.exec_module(pkg)


def ar_members(blob):
    if blob[:8] != b"!<arch>\n":
        raise ValueError("invalid ar")
    pos, files = 8, {}
    while pos < len(blob):
        header = blob[pos:pos + 60]
        if len(header) != 60 or header[58:] != b"`\n":
            raise ValueError("bad ar header")
        name = header[:16].decode().strip().rstrip("/")
        size = int(header[48:58])
        files[name] = blob[pos + 60:pos + 60 + size]
        pos += 60 + size + size % 2
    return files


def untar(data):
    with tarfile.open(fileobj=io.BytesIO(data), mode="r:gz") as archive:
        return {f.name: (archive.extractfile(f).read(), f) for f in archive.getmembers()}


class PackagingTests(unittest.TestCase):
    def test_deb_is_reproducible_and_preserves_licenses(self):
        lock = pkg.load_lock(pkg.PACKAGING / "history/v0.1.0/release-lock.json")
        elf = b"\x7fELF\x02" + bytes(13) + b"\x3e\x00" + bytes(80)
        upstream = {n: n.encode() for n in ["README.md", "LICENSE", "THIRD_PARTY.md", "THIRD_PARTY_LICENSES.txt", "docs/BATCH.md", "docs/ENGINE_RESEARCH.md"]}
        blob = pkg.make_deb(lock, elf, upstream)
        self.assertEqual(blob, pkg.make_deb(lock, elf, upstream))
        members = ar_members(blob)
        self.assertEqual(list(members), ["debian-binary", "control.tar.gz", "data.tar.gz"])
        self.assertEqual(members["debian-binary"], b"2.0\n")
        control, data = untar(members["control.tar.gz"]), untar(members["data.tar.gz"])
        self.assertIn(b"Maintainer: Mad\xc3\xa1r Levente <madar.levi102@gmail.com>", control["control"][0])
        self.assertNotIn(b"@", control["control"][0].replace(b"madar.levi102@gmail.com", b"email"))
        binary = data["usr/bin/makemkv-tui"]
        self.assertEqual(binary[0], elf)
        self.assertEqual(binary[1].mode, 0o755)
        self.assertEqual(binary[1].uid, 0)
        copyright_data = data["usr/share/doc/makemkv-tui/copyright"][0]
        self.assertIn(upstream["LICENSE"], copyright_data)
        self.assertIn(upstream["THIRD_PARTY_LICENSES.txt"], copyright_data)
        for line in control["md5sums"][0].decode().splitlines():
            digest, path = line.split("  ")
            self.assertEqual(hashlib.md5(data[path][0]).hexdigest(), digest)
        self.assertFalse(any("makemkvcon" in n or "batch-state" in n for n in data))

    def test_existing_cache_is_never_trusted_without_hash(self):
        with tempfile.TemporaryDirectory() as d:
            p = Path(d) / "source.tar.gz"
            p.write_bytes(b"partial")
            record = {"filename": p.name, "size": 8, "sha256": "0" * 64}
            with self.assertRaisesRegex(ValueError, "mismatch"):
                pkg.verified(record, d, True)
            self.assertEqual(p.read_bytes(), b"partial")

    def test_source_traversal_and_links_rejected(self):
        for name, kind in [("root/../escape", tarfile.REGTYPE), ("root/link", tarfile.SYMTYPE)]:
            buf = io.BytesIO()
            with tarfile.open(fileobj=buf, mode="w:gz") as archive:
                member = tarfile.TarInfo(name)
                member.type = kind
                member.linkname = "../../escape"
                archive.addfile(member)
            with self.assertRaises(ValueError):
                pkg.source_files(buf.getvalue(), "root")

    def test_manifests_match_verified_release(self):
        lock = pkg.load_lock(pkg.PACKAGING / "history/v0.1.0/release-lock.json")
        scoop = json.loads((pkg.PACKAGING / "history/v0.1.0/scoop/bucket/makemkv-tui.json").read_text())
        self.assertEqual(scoop["version"], lock["version"])
        self.assertEqual(scoop["architecture"]["64bit"]["hash"][0], lock["assets"]["windows"]["sha256"])
        for n in lock["notices"].values():
            self.assertIn(n["sha256"], scoop["architecture"]["64bit"]["hash"])
        winget = (pkg.PACKAGING / "history/v0.1.0/winget/manifests/m/madarlevi/makemkv-tui/0.1.0/madarlevi.makemkv-tui.installer.yaml").read_text()
        self.assertIn(lock["assets"]["windows"]["sha256"].upper(), winget)
        self.assertIn(lock["assets"]["windows"]["url"], winget)
        for path in ["aur/PKGBUILD", "aur/.SRCINFO", "homebrew/Formula/makemkv-tui.rb"]:
            text = (pkg.PACKAGING / "history/v0.1.0" / path).read_text()
            self.assertIn(lock["source"]["sha256"], text)
        distinfo = (pkg.PACKAGING / "history/v0.1.0/freebsd/multimedia/makemkv-tui/distinfo").read_text()
        for n in lock["modules"].values():
            self.assertIn(n["sha256"], distinfo)
            self.assertIn(str(n["size"]), distinfo)


if __name__ == "__main__":
    unittest.main()
