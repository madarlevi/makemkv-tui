#!/usr/bin/env python3
"""Stage 2: finalize all six package formats from a real published GitHub release."""
import argparse
import datetime
import importlib.util
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tarfile
import io
import zipfile
import urllib.parse
import urllib.request
import urllib.error

ROOT = Path(__file__).resolve().parents[2]
PACKAGING = ROOT / "packaging"
spec = importlib.util.spec_from_file_location("release_integrity", ROOT / "tools/release/release.py")
release = importlib.util.module_from_spec(spec)
spec.loader.exec_module(release)
spec = importlib.util.spec_from_file_location("package_core", Path(__file__).with_name("package.py"))
core = importlib.util.module_from_spec(spec)
spec.loader.exec_module(core)
API = "https://api.github.com/repos/" + release.REPOSITORY + "/"


def fetch(url):
    if urllib.parse.urlparse(url).scheme != "https":
        raise ValueError("HTTPS is required")
    headers = {"User-Agent": "makemkv-tui-package-finalizer", "Accept": "application/vnd.github+json"}
    if os.getenv("GITHUB_TOKEN") and urllib.parse.urlparse(url).netloc == "api.github.com":
        headers["Authorization"] = "Bearer " + os.environ["GITHUB_TOKEN"]
    request = urllib.request.Request(url, headers=headers)
    opener = urllib.request.build_opener(release.SafeRedirect())
    with opener.open(request, timeout=120) as response:
        data = response.read(256 * 1024 * 1024 + 1)
    if len(data) > 256 * 1024 * 1024:
        raise ValueError("release input exceeds size limit")
    return data


def json_get(url):
    return json.loads(fetch(url))


def remote_commit(tag):
    obj = json_get(API + "git/ref/tags/" + urllib.parse.quote(tag, safe=""))["object"]
    for _ in range(8):
        if obj["type"] == "commit" and release.COMMIT.fullmatch(obj["sha"]):
            return obj["sha"]
        if obj["type"] != "tag":
            break
        obj = json_get(API + "git/tags/" + obj["sha"])["object"]
    raise ValueError("tag does not resolve to a commit")


def download(url, filename, cache, sha=None, size=None):
    if Path(filename).name != filename or "\\" in filename:
        raise ValueError("unsafe download filename")
    path = Path(cache) / filename
    if path.exists():
        data = path.read_bytes()
        # GitHub source tarballs and module endpoints have no asset digest.
        # Re-fetch them rather than silently blessing a modified local cache.
        if sha is None and data != fetch(url):
            raise ValueError(f"cached and remote source bytes differ: {filename}; not overwritten")
    else:
        data = fetch(url)
    actual = release.digest(data)
    if sha is not None and actual != sha:
        raise ValueError(f"changed cached or remote bytes: {filename}; not overwritten")
    if size is not None and len(data) != size:
        raise ValueError(f"size mismatch for {filename}")
    if not path.exists():
        path.parent.mkdir(parents=True, exist_ok=True)
        with path.open("xb") as f:
            f.write(data)
    return {"filename": filename, "url": url, "sha256": actual, "size": len(data)}, data


def check_module_zip(data, tag, files):
    prefix = "github.com/madarlevi/makemkv-tui@" + tag + "/"
    seen = set()
    with zipfile.ZipFile(io.BytesIO(data)) as archive:
        for entry in archive.infolist():
            if entry.is_dir():
                continue
            if not entry.filename.startswith(prefix):
                raise ValueError("unexpected Go module ZIP root")
            name = entry.filename[len(prefix):]
            if name in seen or name not in files or archive.read(entry) != files[name]:
                raise ValueError("Go module ZIP differs from the tagged source; possible stale proxy/retag")
            seen.add(name)
    required = {"go.mod", *release.DOCUMENTS}
    required.update(name for name in files if name.endswith(".go") and "vendor" not in Path(name).parts)
    if not required.issubset(seen):
        raise ValueError("Go module ZIP is missing tagged source files")


def collect(tag, cache):
    if not release.VERSION.fullmatch(tag) or "-" in tag:
        raise ValueError("package finalization requires a stable semantic version tag")
    info = json_get(API + "releases/tags/" + urllib.parse.quote(tag, safe=""))
    if info["tag_name"] != tag or info["draft"] or info["prerelease"]:
        raise ValueError("release must exist, be public, complete and non-prerelease")
    commit = remote_commit(tag)
    wanted = set(release.names(tag)) | {release.CHECKSUMS}
    assets = {}
    for asset in info["assets"]:
        if asset["name"] not in wanted:
            continue
        if asset["name"] in assets or asset["state"] != "uploaded":
            raise ValueError("duplicate or unfinished release assets")
        assets[asset["name"]] = asset
    if set(assets) != wanted:
        raise ValueError("release is missing required archives or SHA256SUMS.txt")
    cache = Path(cache)
    artifact_cache = cache / "artifacts"
    records = {}
    for name, asset in assets.items():
        url = asset["browser_download_url"]
        if not url.startswith(f"https://github.com/{release.REPOSITORY}/releases/download/{tag}/"):
            raise ValueError("unexpected GitHub asset URL")
        digest = asset.get("digest")
        sha = None
        if digest:
            if not re.fullmatch(r"sha256:[0-9a-f]{64}", digest):
                raise ValueError("unexpected asset digest format")
            sha = digest.split(":", 1)[1]
        records[name], _ = download(url, name, artifact_cache, sha, asset["size"])
    source, source_data = download(info["tarball_url"], f"makemkv-tui-{tag}-source.tar.gz", cache)
    with tarfile.open(fileobj=io.BytesIO(source_data), mode="r:gz") as archive:
        first = archive.getmembers()[0]
        source_root = first.name.split("/")[0]
    if not re.fullmatch("madarlevi-makemkv-tui-" + commit[:7] + "[0-9a-f]*", source_root):
        raise ValueError("source archive root does not identify the tagged commit")
    files = core.source_files(source_data, source_root)
    if not files["go.mod"].startswith(b"module github.com/madarlevi/makemkv-tui\n"):
        raise ValueError("tagged source uses a different Go module")
    toolchain = files[".go-version"].decode().strip()
    release.validate_local(artifact_cache, tag, commit, toolchain)
    # The Go validator independently checks actual ELF/PE/Mach-O headers and Go
    # linker metadata. Python alone does not claim to decode those build records.
    env = dict(os.environ, GOTOOLCHAIN="go" + toolchain)
    subprocess.run(["go", "run", "./tools/release", "--verify", "--version", tag,
                    "--commit", commit, "--go-version", toolchain, "--out", str(artifact_cache.resolve())],
                   cwd=ROOT, env=env, check=True)
    epochs = set()
    for name in release.names(tag):
        members = release.archive_files((artifact_cache / name).read_bytes(), name)
        for doc in release.DOCUMENTS:
            if members[doc] != files[doc]:
                raise ValueError("released documents differ from the actual tagged source")
        epochs.add(json.loads(members["BUILDINFO.json"])["source_date_epoch"])
    commit_info = json_get(API + "commits/" + commit)
    commit_epoch = int(datetime.datetime.fromisoformat(commit_info["commit"]["committer"]["date"].replace("Z", "+00:00")).timestamp())
    if epochs != {commit_epoch}:
        raise ValueError("release timestamp differs from source commit")
    modules = {}
    for ext in ("mod", "zip"):
        url = f"https://proxy.golang.org/github.com/madarlevi/makemkv-tui/@v/{tag}.{ext}"
        modules[ext], data = download(url, "source." + ext, cache)
        if ext == "mod" and data != files["go.mod"]:
            raise ValueError("Go proxy module differs from source; wait for proxy propagation")
        if ext == "zip":
            check_module_zip(data, tag, files)
    if remote_commit(tag) != commit:
        raise ValueError("tag moved during finalization")
    lock = {"schema_version": 1, "repository": release.REPOSITORY, "tag": tag, "version": tag[1:],
            "prerelease": False, "commit": commit, "source_date_epoch": commit_epoch,
            "go_version": toolchain, "release_url": info["html_url"], "source": source,
            "published_at": info["published_at"],
            "source_root": source_root, "modules": modules, "assets": records,
            "notices": {}, "observed_issues": []}
    return lock


def render(lock, out):
    """Only verified inputs enter this function in the CLI. Never overwrite recipes."""
    out = Path(out)
    if out.exists():
        raise ValueError("output already exists; review it or choose a new empty output path")
    values = {"VERSION": lock["version"], "COMMIT": lock["commit"], "SOURCE_URL": lock["source"]["url"],
              "SOURCE_SHA256": lock["source"]["sha256"], "SOURCE_ROOT": lock["source_root"],
              "WINDOWS_URL": lock["assets"][f"makemkv-tui-{lock['tag']}-windows-amd64.zip"]["url"],
              "WINDOWS_SHA256": lock["assets"][f"makemkv-tui-{lock['tag']}-windows-amd64.zip"]["sha256"],
              "RELEASE_DATE": datetime.datetime.fromtimestamp(lock["source_date_epoch"], datetime.timezone.utc).date().isoformat()}
    if lock.get("published_at"):
        values["RELEASE_DATE"] = datetime.datetime.fromisoformat(lock["published_at"].replace("Z", "+00:00")).date().isoformat()
    values["WINDOWS_SHA256_UPPER"] = values["WINDOWS_SHA256"].upper()
    def substitute(text):
        for key, value in values.items():
            text = text.replace("@" + key + "@", value)
        if re.search(r"@[A-Z][A-Z0-9_]+@", text):
            raise ValueError("unresolved package template token")
        return text
    rendered = {}
    for kind in ("freebsd", "aur", "homebrew", "scoop", "winget"):
        for path in (PACKAGING / kind).rglob("*"):
            if not path.is_file():
                continue
            rel = path.relative_to(PACKAGING)
            if path.suffix == ".in":
                rel = rel.with_suffix("")
                if kind == "winget":
                    rel = Path("winget/manifests/m/madarlevi/makemkv-tui") / lock["version"] / path.name.removesuffix(".in")
                if kind == "scoop":
                    rel = Path("scoop/bucket") / path.name.removesuffix(".in")
                rendered[rel] = substitute(path.read_text(encoding="utf-8"))
            elif path.name in ("pkg-plist", "pkg-descr", "pkg-message"):
                rendered[rel] = path.read_text(encoding="utf-8")
    # AUR source URL is recorded from the real API, not inferred from tag names.
    for rel in (Path("aur/PKGBUILD"), Path("aur/.SRCINFO")):
        rendered[rel] = rendered[rel].replace("https://api.github.com/repos/madarlevi/makemkv-tui/tarball/v${pkgver}", values["SOURCE_URL"])
        rendered[rel] = rendered[rel].replace(f"https://api.github.com/repos/madarlevi/makemkv-tui/tarball/v{lock['version']}", values["SOURCE_URL"])
    prefix = f"go/multimedia_makemkv-tui/makemkv-tui-{lock['tag']}/"
    distinfo = f"TIMESTAMP = {lock['source_date_epoch']}\n"
    for ext in ("mod", "zip"):
        record = lock["modules"][ext]
        name = prefix + lock["tag"] + "." + ext
        distinfo += f"SHA256 ({name}) = {record['sha256']}\nSIZE ({name}) = {record['size']}\n"
    rendered[Path("freebsd/multimedia/makemkv-tui/distinfo")] = distinfo
    rendered[Path("release-lock.json")] = json.dumps(lock, ensure_ascii=False, indent=2) + "\n"
    rendered[Path("debian/README.md")] = (
        "The finalizer writes a real amd64 .deb here using the already verified Linux release executable.\n"
        "It preserves source licenses and documentation. Validate with dpkg-deb/lintian and install in a disposable VM.\n")
    rendered[Path("README.md")] = (f"# Finalized packages for {lock['tag']}\n\n"
        f"Verified source commit: {lock['commit']}\n\n"
        "Generated from actual published release bytes. No upstream submission was performed.\n"
        "Run the native validators and submission steps in the repository packaging/README.md.\n"
        "Build Debian using packaging/tools/package.py deb --lock PATH/TO/release-lock.json.\n")
    out.mkdir(parents=True, exist_ok=False)
    for rel, text in rendered.items():
        p = out / rel
        p.parent.mkdir(parents=True, exist_ok=True)
        with p.open("x", encoding="utf-8", newline="\n") as f:
            f.write(text)
    print("Prepared verified package recipes at", out)


def materialize_deb(lock, cache, out):
    upstream = core.source_files(core.verified(lock["source"], cache, True), lock["source_root"])
    name = f"makemkv-tui-{lock['tag']}-linux-amd64.tar.gz"
    members = release.archive_files(core.verified(lock["assets"][name], cache, True), name)
    content = core.make_deb(lock, members["makemkv-tui"], upstream)
    path = Path(out) / "debian" / f"makemkv-tui_{lock['version']}-1_amd64.deb"
    with path.open("xb") as f:
        f.write(content)
    with Path(str(path) + ".sha256").open("x", encoding="ascii", newline="\n") as f:
        f.write(f"{release.digest(content)}  {path.name}\n")
    print("Finalized all six formats, including", path)


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--tag", required=True)
    p.add_argument("--out", required=True)
    p.add_argument("--cache")
    a = p.parse_args()
    cache = a.cache or str(ROOT / ".cache/packaging" / a.tag)
    try:
        lock = collect(a.tag, cache)
        render(lock, a.out)
        materialize_deb(lock, cache, a.out)
    except urllib.error.HTTPError as e:
        raise SystemExit(f"Finalization stopped: HTTP {e.code} from {e.url}. The release must be published first; Go proxy propagation may require a retry. No guessed hashes are generated.") from e


if __name__ == "__main__":
    main()
