import copy
import importlib.util
import json
import io
import zipfile
from pathlib import Path
import tempfile
import unittest

spec=importlib.util.spec_from_file_location("finalize",Path(__file__).with_name("finalize.py"))
f=importlib.util.module_from_spec(spec);spec.loader.exec_module(f)


class FinalizeTests(unittest.TestCase):
    def fixture(self):
        # Synthetic transport fixtures, never committed as submission manifests.
        record=lambda name:{"filename":name,"url":"https://example.invalid/unit-test/"+name,"sha256":f.release.digest(name.encode()),"size":len(name)}
        return {"schema_version":1,"repository":f.release.REPOSITORY,"tag":"v1.2.3","version":"1.2.3",
                "commit":"a"*40,"source_date_epoch":1700000000,"go_version":"1.26.10",
                "source":record("source.tar.gz"),"source_root":"unit-test-source",
                "assets":{name:record(name) for name in f.release.names("v1.2.3")},
                "modules":{ext:record("source."+ext) for ext in ("mod","zip")},"notices":{}}

    def test_six_formats_render_only_verified_input_values(self):
        lock=self.fixture()
        with tempfile.TemporaryDirectory() as d:
            out=Path(d)/"generated";f.render(lock,out)
            self.assertIn("DISTVERSION=\t1.2.3",(out/"freebsd/multimedia/makemkv-tui/Makefile").read_text())
            self.assertIn(lock["modules"]["zip"]["sha256"],(out/"freebsd/multimedia/makemkv-tui/distinfo").read_text())
            self.assertIn(lock["source"]["sha256"],(out/"aur/PKGBUILD").read_text())
            self.assertIn(lock["source"]["sha256"],(out/"aur/.SRCINFO").read_text())
            self.assertIn(lock["source"]["url"],(out/"homebrew/Formula/makemkv-tui.rb").read_text())
            scoop=json.loads((out/"scoop/bucket/makemkv-tui.json").read_text())
            windows=lock["assets"]["makemkv-tui-v1.2.3-windows-amd64.zip"]
            self.assertEqual(scoop["architecture"]["64bit"]["hash"],windows["sha256"])
            self.assertEqual(scoop["bin"],"makemkv-tui.exe")
            installer=(out/"winget/manifests/m/madarlevi/makemkv-tui/1.2.3/madarlevi.makemkv-tui.installer.yaml").read_text()
            self.assertIn(windows["sha256"].upper(),installer)
            self.assertIn("RelativeFilePath: makemkv-tui.exe",installer)
            self.assertEqual(json.loads((out/"release-lock.json").read_text())["commit"],lock["commit"])
            with self.assertRaisesRegex(ValueError,"already exists"):
                f.render(lock,out)

    def test_missing_public_release_does_not_create_output_or_hashes(self):
        original=f.json_get
        f.json_get=lambda url: {"tag_name":"v1.2.3","draft":True,"prerelease":False}
        try:
            with tempfile.TemporaryDirectory() as d:
                with self.assertRaisesRegex(ValueError,"public"):
                    f.collect("v1.2.3",Path(d)/"cache")
                self.assertFalse((Path(d)/"cache").exists())
        finally:f.json_get=original

    def test_bad_tag_cannot_be_used_as_cache_traversal(self):
        with self.assertRaisesRegex(ValueError,"stable"):
            f.collect("../../outside",Path("unused"))

    def test_source_without_authoritative_hash_rechecks_cached_bytes(self):
        original=f.fetch
        f.fetch=lambda url:b"real remote source bytes"
        try:
            with tempfile.TemporaryDirectory() as d:
                path=Path(d)/"source.tar.gz";path.write_bytes(b"changed local cache")
                with self.assertRaisesRegex(ValueError,"differ"):
                    f.download("https://example.invalid/unit-test/source","source.tar.gz",d)
                self.assertEqual(path.read_bytes(),b"changed local cache")
        finally:f.fetch=original

    def test_module_zip_cannot_hide_stale_source_behind_matching_documents(self):
        files={name:name.encode() for name in ("go.mod",*f.release.DOCUMENTS)}
        files["cmd/makemkv-tui/main.go"]=b"package main"
        buf=io.BytesIO()
        with zipfile.ZipFile(buf,"w") as z:
            for name,data in files.items():z.writestr("github.com/madarlevi/makemkv-tui@v1.2.3/"+name,data)
        f.check_module_zip(buf.getvalue(),"v1.2.3",files)
        files["cmd/makemkv-tui/main.go"]=b"different tagged source"
        with self.assertRaisesRegex(ValueError,"differs"):
            f.check_module_zip(buf.getvalue(),"v1.2.3",files)

if __name__=="__main__":unittest.main()
