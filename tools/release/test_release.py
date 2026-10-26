import copy
import importlib.util
from pathlib import Path
import tempfile
import unittest
import urllib.parse
import urllib.request

spec = importlib.util.spec_from_file_location("release", Path(__file__).with_name("release.py"))
r = importlib.util.module_from_spec(spec)
spec.loader.exec_module(r)
SHA = "a" * 40


class FakeGitHub:
    repository = r.REPOSITORY
    def __init__(self):
        self.release = None
        self.content = {}
        self.created = self.uploads = self.patches = 0
        self.fail_once_after_upload = False
        self.tag_commit = SHA
    def commit_for_tag(self, tag):
        return self.tag_commit
    def request(self, method, path, data=None, raw=False):
        if raw:
            return self.content[path.rsplit("/", 1)[-1]]
        if method == "GET":
            if path.startswith("releases?"):
                return [copy.deepcopy(self.release)] if self.release else []
            return copy.deepcopy(self.release)
        if method == "POST" and path == "releases":
            self.created += 1
            self.release = dict(data, id=123, assets=[])
        elif method == "POST":
            self.uploads += 1
            name = urllib.parse.parse_qs(urllib.parse.urlparse(path).query)["name"][0]
            if name in self.content:
                raise AssertionError("asset overwrite attempted")
            self.content[name] = data
            self.release["assets"].append({"name":name,"url":"https://api.github.com/assets/"+name})
            if self.fail_once_after_upload:
                self.fail_once_after_upload = False
                raise RuntimeError("connection lost after server stored bytes")
        elif method == "PATCH":
            self.patches += 1
            self.release.update(data)
        else:
            raise AssertionError("unexpected mutation")
        return copy.deepcopy(self.release)


class ReleaseTests(unittest.TestCase):
    def files(self):
        return {n: n.encode() for n in [*r.names("v1.2.3"), r.CHECKSUMS]}

    def test_publish_exactly_once_and_retry_without_writes(self):
        api = FakeGitHub();files = self.files()
        r.publish(api,"v1.2.3",SHA,files)
        r.publish(api,"v1.2.3",SHA,files)
        self.assertEqual((api.created,api.uploads,api.patches),(1,6,1))
        self.assertFalse(api.release["draft"])

    def test_resume_draft_after_uncertain_upload_without_clobber(self):
        api = FakeGitHub();api.fail_once_after_upload=True
        with self.assertRaises(RuntimeError):
            r.publish(api,"v1.2.3",SHA,self.files())
        self.assertTrue(api.release["draft"])
        self.assertEqual(api.patches,0)
        r.publish(api,"v1.2.3",SHA,self.files())
        self.assertEqual((api.created,api.uploads,api.patches),(1,6,1))

    def test_existing_public_contents_never_replaced(self):
        api=FakeGitHub();files=self.files();r.publish(api,"v1.2.3",SHA,files)
        before=(api.created,api.uploads,api.patches)
        api.content[next(iter(files))]=b"different published bytes"
        with self.assertRaisesRegex(ValueError,"differs"):
            r.publish(api,"v1.2.3",SHA,files)
        self.assertEqual(before,(api.created,api.uploads,api.patches))

    def test_moved_tag_and_foreign_draft_fail_before_mutations(self):
        api=FakeGitHub();api.tag_commit="b"*40
        with self.assertRaisesRegex(ValueError,"moved"):
            r.publish(api,"v1.2.3",SHA,self.files())
        self.assertEqual(api.created,0)
        api.tag_commit=SHA;api.release={"id":123,"tag_name":"v1.2.3","draft":True,"body":"someone else's draft","assets":[]}
        with self.assertRaisesRegex(ValueError,"not owned"):
            r.publish(api,"v1.2.3",SHA,self.files())
        self.assertEqual(api.uploads,0)

    def test_partial_or_unrelated_artifact_set_refused(self):
        with tempfile.TemporaryDirectory() as d:
            Path(d,"secret.key").write_text("never upload")
            with self.assertRaisesRegex(ValueError,"unexpected"):
                r.validate_local(d,"v1.2.3",SHA,"1.26.10")

    def test_credentials_not_forwarded_to_signed_download_host(self):
        req=urllib.request.Request("https://api.github.com/assets/1",headers={"Authorization":"Bearer fake-unit-test-token"})
        new=r.SafeRedirect().redirect_request(req,None,302,"found",{},"https://release-assets.githubusercontent.com/file")
        self.assertIsNone(new.get_header("Authorization"))
        with self.assertRaises(ValueError):
            r.SafeRedirect().redirect_request(req,None,302,"found",{},"http://example.invalid/file")

if __name__=="__main__":
    unittest.main()
