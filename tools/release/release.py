#!/usr/bin/env python3
"""Release integrity and draft-then-publish, using only the official GitHub API."""
import argparse
import hashlib
import io
import json
import os
from pathlib import Path
import re
import stat
import tarfile
import urllib.error
import urllib.parse
import urllib.request
import zipfile

REPOSITORY = "madarlevi/makemkv-tui"
TARGETS = ("darwin/amd64", "darwin/arm64", "freebsd/amd64", "linux/amd64", "windows/amd64")
DOCUMENTS = ("LICENSE", "README.md", "THIRD_PARTY.md", "THIRD_PARTY_LICENSES.txt")
CHECKSUMS = "SHA256SUMS.txt"
VERSION = re.compile(r"^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*))?$")
COMMIT = re.compile(r"^[0-9a-f]{40}$")


def digest(data):
    return hashlib.sha256(data).hexdigest()


def names(tag):
    if not VERSION.fullmatch(tag):
        raise ValueError("invalid semantic version tag")
    return {f"makemkv-tui-{tag}-{t.replace('/', '-')}{'.zip' if t.startswith('windows/') else '.tar.gz'}": t for t in TARGETS}


def archive_files(data, filename):
    result = {}
    def add(name, mode, content):
        if "/" in name or "\\" in name or name in result or not name or len(content) > 128 * 1024 * 1024:
            raise ValueError(f"unsafe/duplicate archive member {name}")
        expected = 0o755 if name in ("makemkv-tui", "makemkv-tui.exe") else 0o644
        if mode & 0o777 != expected:
            raise ValueError(f"wrong permissions for {name}")
        result[name] = content
    if filename.endswith(".zip"):
        with zipfile.ZipFile(io.BytesIO(data)) as archive:
            for f in archive.infolist():
                if f.is_dir() or stat.S_ISLNK(f.external_attr >> 16) or f.file_size > 128 * 1024 * 1024:
                    raise ValueError("unsafe ZIP entry")
                add(f.filename, f.external_attr >> 16, archive.read(f))
    else:
        with tarfile.open(fileobj=io.BytesIO(data), mode="r:gz") as archive:
            for f in archive.getmembers():
                if not f.isfile() or f.size > 128 * 1024 * 1024:
                    raise ValueError("unsafe tar entry")
                add(f.name, f.mode, archive.extractfile(f).read())
    return result


def validate_local(directory, tag, commit, toolchain, preview=False):
    expected = names(tag)
    if not COMMIT.fullmatch(commit) and not (preview and commit == "uncommitted"):
        raise ValueError("exact source commit required")
    directory = Path(directory)
    wanted = set(expected) | {CHECKSUMS}
    if {p.name for p in directory.iterdir()} != wanted:
        raise ValueError("missing or unexpected release artifacts; use a clean output directory")
    files, doc_hashes, epochs = {}, {}, set()
    for filename, target in expected.items():
        p = directory / filename
        if p.is_symlink() or not p.is_file():
            raise ValueError("release artifact is not a regular file")
        content = p.read_bytes()
        members = archive_files(content, filename)
        binary = "makemkv-tui.exe" if target.startswith("windows/") else "makemkv-tui"
        if set(members) != set(DOCUMENTS) | {binary, "BUILDINFO.json"} or not all(members.values()):
            raise ValueError("release archive has missing/extra/empty members")
        meta = json.loads(members["BUILDINFO.json"])
        identity = (meta["schema_version"], meta["version"], meta["commit"], meta["target"], meta["go_version"], meta["preview"])
        if identity != (1, tag, commit, target, "go" + toolchain.removeprefix("go"), preview):
            raise ValueError("release binary metadata identity mismatch")
        if digest(members[binary]) != meta["binary_sha256"]:
            raise ValueError("release binary digest mismatch")
        if not isinstance(meta["source_date_epoch"], int) or meta["source_date_epoch"] < 0:
            raise ValueError("invalid source date")
        epochs.add(meta["source_date_epoch"])
        for doc in DOCUMENTS:
            h = digest(members[doc])
            if doc in doc_hashes and doc_hashes[doc] != h:
                raise ValueError("archives came from different source documents")
            doc_hashes[doc] = h
        files[filename] = content
    if len(epochs) != 1:
        raise ValueError("archives came from different commits/timestamps")
    sums = "".join(f"{digest(files[n])}  {n}\n" for n in sorted(files)).encode()
    if (directory / CHECKSUMS).is_symlink() or (directory / CHECKSUMS).read_bytes() != sums:
        raise ValueError("release checksums do not match final archive bytes")
    files[CHECKSUMS] = sums
    return files


class SafeRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        if urllib.parse.urlparse(newurl).scheme != "https":
            raise ValueError("non-HTTPS redirect refused")
        new = super().redirect_request(req, fp, code, msg, headers, newurl)
        if new and urllib.parse.urlparse(newurl).netloc != urllib.parse.urlparse(req.full_url).netloc:
            new.remove_header("Authorization")
        return new


class GitHub:
    def __init__(self, repository, token):
        if repository != REPOSITORY:
            raise ValueError("publishing is restricted to the intended application repository")
        self.repository, self.token = repository, token
        self.opener = urllib.request.build_opener(SafeRedirect())

    def request(self, method, path, data=None, raw=False):
        url = path if path.startswith("https://") else f"https://api.github.com/repos/{self.repository}/{path}"
        if urllib.parse.urlparse(url).netloc not in ("api.github.com", "uploads.github.com"):
            raise ValueError("refusing to send credentials outside official GitHub API hosts")
        headers = {"Authorization": "Bearer " + self.token, "User-Agent": "makemkv-tui-release",
                   "Accept": "application/octet-stream" if raw else "application/vnd.github+json",
                   "X-GitHub-Api-Version": "2022-11-28"}
        if isinstance(data, dict):
            data = json.dumps(data).encode()
            headers["Content-Type"] = "application/json"
        elif data is not None:
            headers["Content-Type"] = "application/octet-stream"
        req = urllib.request.Request(url, data=data, method=method, headers=headers)
        try:
            with self.opener.open(req, timeout=120) as response:
                body = response.read()
        except urllib.error.HTTPError as e:
            if method == "GET" and e.code == 404:
                return None
            raise RuntimeError(f"GitHub API {method} failed: HTTP {e.code}; rerun after inspection; no assets overwritten") from e
        return body if raw else (json.loads(body) if body else None)

    def commit_for_tag(self, tag):
        obj = self.request("GET", "git/ref/tags/" + urllib.parse.quote(tag, safe=""))
        if not obj:
            raise ValueError("tag does not exist remotely")
        obj = obj["object"]
        for _ in range(8):
            if obj["type"] == "commit" and COMMIT.fullmatch(obj["sha"]):
                return obj["sha"]
            if obj["type"] != "tag":
                break
            obj = self.request("GET", "git/tags/" + obj["sha"])["object"]
        raise ValueError("cannot resolve remote tag to a commit")


def publish(api, tag, commit, files, notes=None):
    """Retry-safe: no deletes, no clobber, public releases become read-only."""
    if api.commit_for_tag(tag) != commit:
        raise ValueError("remote tag moved; refusing publication")
    marker = f"<!-- makemkv-tui-release:{tag}:{commit} -->"
    if notes is None:
        notes = ("Five verified frontend archives and SHA256SUMS.txt. MakeMKV is not bundled. "
                 "FreeBSD is cross-compiled; native engine/FUSE validation remains separate. See README and CHANGELOG.")
    release = api.request("GET", "releases/tags/" + urllib.parse.quote(tag, safe=""))
    if release is None:
        # Authenticated listing also finds an unfinished owned draft if tag lookup
        # does not return drafts. Refuse ambiguity rather than creating another.
        candidates = api.request("GET", "releases?per_page=100") or []
        candidates = [r for r in candidates if r["tag_name"] == tag]
        if len(candidates) > 1:
            raise ValueError("multiple releases/drafts for tag; inspect manually")
        release = candidates[0] if candidates else None
    if release is None:
        release = api.request("POST", "releases", {"tag_name": tag, "target_commitish": commit,
            "name": "makemkv-tui " + tag, "draft": True, "prerelease": "-" in tag,
            "body": marker + "\n\n" + notes})
    if release["draft"] and marker not in release.get("body", ""):
        raise ValueError("existing draft is not owned by this commit/workflow; no modifications made")
    assets = release.get("assets", [])
    existing = {}
    for asset in assets:
        name = asset["name"]
        if name not in files or name in existing:
            raise ValueError("unexpected/duplicate existing release assets; nothing overwritten")
        content = api.request("GET", asset["url"], raw=True)
        if content is None or content != files[name]:
            raise ValueError(f"existing release asset differs: {name}; nothing overwritten")
        existing[name] = True
    if not release["draft"]:
        if set(existing) != set(files):
            raise ValueError("published release is incomplete; repair requires explicit owner action")
        print("Already published with identical assets; no changes made")
        return
    rid = release["id"]
    if not isinstance(rid, int) or rid <= 0:
        raise ValueError("invalid release id")
    for name in sorted(set(files) - set(existing)):
        path = f"https://uploads.github.com/repos/{api.repository}/releases/{rid}/assets?name={urllib.parse.quote(name, safe='')}"
        api.request("POST", path, files[name])
    checked = api.request("GET", f"releases/{rid}")
    if checked.get("tag_name") != tag or marker not in checked.get("body", ""):
        raise ValueError("release changed during upload")
    if len(checked["assets"]) != len(files) or {a["name"] for a in checked["assets"]} != set(files):
        raise ValueError("uploaded asset set incomplete")
    for asset in checked["assets"]:
        if api.request("GET", asset["url"], raw=True) != files[asset["name"]]:
            raise ValueError("uploaded asset validation failed; draft retained")
    if api.commit_for_tag(tag) != commit:
        raise ValueError("tag moved during upload; draft retained")
    api.request("PATCH", f"releases/{rid}", {"draft": False})
    final = api.request("GET", f"releases/{rid}")
    if final["draft"]:
        raise ValueError("GitHub did not publish the verified release")
    print("Published one complete verified release: " + tag)


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--tag", required=True)
    p.add_argument("--commit", required=True)
    p.add_argument("--dir", required=True)
    p.add_argument("--go-version", default=Path(".go-version").read_text().strip())
    p.add_argument("--publish", action="store_true")
    p.add_argument("--preview", action="store_true")
    args = p.parse_args()
    if args.publish and args.preview:
        raise ValueError("preview artifacts can never be published")
    files = validate_local(args.dir, args.tag, args.commit, args.go_version, args.preview)
    if not args.publish:
        print("Dry run: all six final artifacts validated; no GitHub API writes")
        return
    if os.getenv("GITHUB_EVENT_NAME") != "push" or os.getenv("GITHUB_REF") != "refs/tags/" + args.tag:
        raise ValueError("publishing is allowed only for an actual tag push, never a manual branch run")
    token = os.environ["GITHUB_TOKEN"]
    publish(GitHub(os.environ["GITHUB_REPOSITORY"], token), args.tag, args.commit, files,
            Path("docs/RELEASE_NOTES.md").read_text(encoding="utf-8"))


if __name__ == "__main__":
    main()
