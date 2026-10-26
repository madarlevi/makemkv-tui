#!/usr/bin/env python3
"""Verified release inputs and deterministic binary .deb packaging; no runtime code."""
import argparse
import datetime
import gzip
import hashlib
import io
import json
import math
import os
from pathlib import Path, PurePosixPath
import re
import subprocess
import tarfile
import tempfile
import urllib.request
import zipfile

ROOT = Path(__file__).resolve().parents[2]
PACKAGING = ROOT / "packaging"


def load_lock(path):
    lock = json.loads(Path(path).read_text(encoding="utf-8"))
    if lock["schema_version"] != 1 or lock["repository"] != "madarlevi/makemkv-tui":
        raise ValueError("unsupported release inventory or repository")
    if not re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+", lock["tag"]):
        raise ValueError("invalid locked tag")
    for record in records(lock):
        name = record["filename"]
        if PurePosixPath(name).name != name or "\\" in name:
            raise ValueError("unsafe cache filename")
        if not re.fullmatch(r"[a-f0-9]{64}", record["sha256"]):
            raise ValueError("invalid SHA256")
        if not record["url"].startswith("https://"):
            raise ValueError("release input must use HTTPS")
    return lock


def records(lock):
    return [lock["source"], *lock["modules"].values(), *lock["assets"].values(),
            *lock["notices"].values()]


def verified(record, cache, offline=False):
    path = Path(cache) / record["filename"]
    if not path.exists() and (Path(cache) / "artifacts" / record["filename"]).exists():
        path = Path(cache) / "artifacts" / record["filename"]
    if path.exists():
        data = path.read_bytes()
    else:
        if offline:
            raise ValueError(f"offline cache missing {path}")
        req = urllib.request.Request(record["url"], headers={"User-Agent": "makemkv-tui-packaging"})
        with urllib.request.urlopen(req, timeout=90) as response:
            data = response.read(record["size"] + 1)
    if len(data) != record["size"] or hashlib.sha256(data).hexdigest() != record["sha256"]:
        raise ValueError(f"size/SHA256 mismatch: {path}; retained existing cache; refusing changed release")
    if not path.exists():
        path.parent.mkdir(parents=True, exist_ok=True)
        with path.open("xb") as f:
            f.write(data)
    return data


def source_files(data, prefix):
    """Read only verified regular files, rejecting links/traversal before extraction."""
    files = {}
    with tarfile.open(fileobj=io.BytesIO(data), mode="r:gz") as archive:
        for member in archive.getmembers():
            path = PurePosixPath(member.name)
            if path.is_absolute() or ".." in path.parts or "\\" in member.name or path.parts[0] != prefix:
                raise ValueError(f"unsafe source member {member.name}")
            if member.isdir():
                continue
            if not member.isfile():
                raise ValueError(f"non-regular source member {member.name}")
            relative = str(PurePosixPath(*path.parts[1:]))
            if relative == "." or relative in files:
                raise ValueError(f"duplicate/invalid source member {relative}")
            files[relative] = archive.extractfile(member).read()
    return files


def gzip_bytes(data, epoch):
    out = io.BytesIO()
    with gzip.GzipFile(fileobj=out, mode="wb", filename="", mtime=epoch, compresslevel=9) as gz:
        gz.write(data)
    return out.getvalue()


def tar_bytes(files, epoch):
    """files maps POSIX path to (content, mode); normalized root ownership."""
    out = io.BytesIO()
    with tarfile.open(fileobj=out, mode="w", format=tarfile.GNU_FORMAT) as archive:
        for name in sorted(files):
            data, mode = files[name]
            item = tarfile.TarInfo(name)
            item.size, item.mode, item.mtime = len(data), mode, epoch
            item.uid = item.gid = 0
            item.uname = item.gname = "root"
            archive.addfile(item, io.BytesIO(data))
    return gzip_bytes(out.getvalue(), epoch)


def ar_bytes(members, epoch):
    out = io.BytesIO(b"!<arch>\n")
    out.seek(0, io.SEEK_END)
    for name, data in members:
        header = f"{name + '/':<16}{epoch:<12}{0:<6}{0:<6}{'100644':<8}{len(data):<10}`\n"
        if len(header) != 60:
            raise ValueError("invalid ar header")
        out.write(header.encode("ascii"))
        out.write(data)
        if len(data) % 2:
            out.write(b"\n")
    return out.getvalue()


def make_deb(lock, binary, upstream):
    if binary[:5] != b"\x7fELF\x02" or binary[18:20] != b"\x3e\x00":
        raise ValueError("Debian package requires an ELF64 amd64 executable")
    epoch, version = lock["source_date_epoch"], lock["version"]
    doc = "usr/share/doc/makemkv-tui/"
    data = {"usr/bin/makemkv-tui": (binary, 0o755)}
    for name in ["README.md", "LICENSE", "THIRD_PARTY.md", "THIRD_PARTY_LICENSES.txt"]:
        data[doc + name] = (upstream[name], 0o644)
    for name in ["BATCH.md", "ENGINE_RESEARCH.md"]:
        data[doc + name] = (upstream["docs/" + name], 0o644)
    copyright_text = ("Upstream: https://github.com/madarlevi/makemkv-tui\n"
                      f"Source: {lock['source']['url']}\n"
                      "Upstream author/copyright holder: madarlevi\n\n").encode()
    copyright_text += upstream["LICENSE"] + b"\n\n" + upstream["THIRD_PARTY_LICENSES.txt"]
    data[doc + "copyright"] = (copyright_text, 0o644)
    data[doc + "README.Debian"] = ((PACKAGING / "debian/README.Debian").read_bytes(), 0o644)
    stamp = datetime.datetime.fromtimestamp(epoch, datetime.timezone.utc).strftime("%a, %d %b %Y %H:%M:%S +0000")
    changelog = (f"makemkv-tui ({version}-1) unstable; urgency=medium\n\n"
                 "  * Package verified tagged frontend source; engine installed separately.\n\n"
                 f" -- Madár Levente <madar.levi102@gmail.com>  {stamp}\n").encode()
    data[doc + "changelog.Debian.gz"] = (gzip_bytes(changelog, epoch), 0o644)
    man=(PACKAGING / "makemkv-tui.1").read_text(encoding="utf-8").replace("@VERSION@",version).replace("@DATE@",datetime.datetime.fromtimestamp(epoch,datetime.timezone.utc).date().isoformat()).encode()
    data["usr/share/man/man1/makemkv-tui.1.gz"] = (gzip_bytes(man, epoch), 0o644)
    installed_size = math.ceil(sum(len(content) for content, _ in data.values()) / 1024)
    control = (PACKAGING / "debian/control.in").read_text(encoding="utf-8")
    control = control.replace("@VERSION@", version).replace("@INSTALLED_SIZE@", str(installed_size))
    md5 = "".join(f"{hashlib.md5(data[name][0]).hexdigest()}  {name}\n" for name in sorted(data))
    members = [("debian-binary", b"2.0\n"),
               ("control.tar.gz", tar_bytes({"control": (control.encode(), 0o644), "md5sums": (md5.encode(), 0o644)}, epoch)),
               ("data.tar.gz", tar_bytes(data, epoch))]
    return ar_bytes(members, epoch)


def build_deb(lock, cache, out, offline):
    upstream = source_files(verified(lock["source"], cache, offline), lock["source_root"])
    with tempfile.TemporaryDirectory(prefix="makemkv-package-") as tmp:
        source = Path(tmp) / "source"
        for name, content in upstream.items():
            path = source / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(content)
        env = dict(os.environ, GOOS="linux", GOARCH="amd64", CGO_ENABLED="0", GOTOOLCHAIN=("go"+lock["go_version"]) if lock.get("go_version") else "local")
        if offline:
            env["GOPROXY"] = "off"
        binary = Path(tmp) / "makemkv-tui"
        subprocess.run(["go", "build", "-mod=readonly", "-trimpath", "-buildvcs=false", "-ldflags",
                        f"-s -w -buildid= -X main.version={lock['tag']} -X main.revision={lock['commit']}", "-o", str(binary),
                        "./cmd/makemkv-tui"], cwd=source, env=env, check=True)
        package = make_deb(lock, binary.read_bytes(), upstream)
    dest = Path(out) / f"makemkv-tui_{lock['version']}-1_amd64.deb"
    dest.parent.mkdir(parents=True, exist_ok=True)
    with dest.open("xb") as f:
        f.write(package)
    with Path(str(dest) + ".sha256").open("x", encoding="ascii", newline="\n") as f:
        f.write(f"{hashlib.sha256(package).hexdigest()}  {dest.name}\n")
    print(dest)


def verify_release(lock, cache, offline, submission):
    for record in records(lock):
        verified(record, cache, offline)
        print(f"VERIFIED {record['filename']} {record['sha256']}")
    upstream = source_files(verified(lock["source"], cache, True), lock["source_root"])
    for name, notice in lock["notices"].items():
        if upstream[name] != verified(notice, cache, True):
            raise ValueError(f"tagged archive and pinned commit notice differ: {name}")
    if upstream["go.mod"] != verified(lock["modules"]["mod"], cache, True):
        raise ValueError("tagged source go.mod differs from Go proxy module")
    for asset in lock["assets"].values():
        if not asset["filename"].endswith(".zip"):
            continue
        with zipfile.ZipFile(io.BytesIO(verified(asset, cache, True))) as archive:
            for name in archive.namelist():
                if PurePosixPath(name).name != name or "\\" in name:
                    raise ValueError(f"unsafe release ZIP member {name}")
    if submission:
        windows = lock["assets"].get("windows") or lock["assets"][f"makemkv-tui-{lock['tag']}-windows-amd64.zip"]
        with zipfile.ZipFile(io.BytesIO(verified(windows, cache, True))) as archive:
            required = {"LICENSE", "THIRD_PARTY.md", "THIRD_PARTY_LICENSES.txt"}
            missing = sorted(required - set(archive.namelist()))
        if missing or not windows["filename"].startswith(f"makemkv-tui-{lock['tag']}-"):
            raise ValueError(f"WinGet submission blocked: missing {missing} and/or release/asset version mismatch; see packaging/README.md")


def build_local_deb(version, commit, out):
    if not re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+", version):
        raise ValueError("preview version must be a semantic tag")
    epoch = int(os.environ.get("SOURCE_DATE_EPOCH", "0"))
    lock = {"version": version[1:]+"~ci", "tag": version, "commit": commit,
            "source_date_epoch": epoch, "source": {"url": "Local CI preview; not a published source archive"}}
    upstream = {n:(ROOT/n).read_bytes() for n in ["LICENSE","README.md","THIRD_PARTY.md","THIRD_PARTY_LICENSES.txt","docs/BATCH.md","docs/ENGINE_RESEARCH.md"]}
    with tempfile.TemporaryDirectory(prefix="makemkv-deb-preview-") as tmp:
        binary=Path(tmp)/"makemkv-tui"
        compiler=(ROOT/".go-version").read_text().strip()
        env=dict(os.environ,GOOS="linux",GOARCH="amd64",CGO_ENABLED="0",GOTOOLCHAIN="go"+compiler)
        subprocess.run(["go","build","-mod=readonly","-trimpath","-buildvcs=false","-ldflags",
                        f"-s -w -buildid= -X main.version={version} -X main.revision={commit}",
                        "-o",str(binary),"./cmd/makemkv-tui"],cwd=ROOT,env=env,check=True)
        package=make_deb(lock,binary.read_bytes(),upstream)
    dest=Path(out)/f"makemkv-tui_{lock['version']}-1_amd64.deb"
    dest.parent.mkdir(parents=True,exist_ok=True)
    with dest.open("xb") as f:f.write(package)
    print("PREVIEW ONLY:",dest)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=["verify", "deb", "deb-local"])
    parser.add_argument("--lock", help="verified lock generated by finalize.py after publication")
    parser.add_argument("--version", default="v0.0.0", help="local CI preview version only")
    parser.add_argument("--commit", default="uncommitted", help="local preview identity")
    parser.add_argument("--cache", default=str(ROOT / ".cache/packaging"))
    parser.add_argument("--out", default=str(ROOT / "dist/packages"))
    parser.add_argument("--offline", action="store_true")
    parser.add_argument("--submission", action="store_true")
    args = parser.parse_args()
    if args.command == "deb-local":
        build_local_deb(args.version,args.commit,args.out)
        return
    if not args.lock:
        parser.error("--lock is required: run packaging/tools/finalize.py after the real release exists")
    lock = load_lock(args.lock)
    if args.command == "verify":
        verify_release(lock, args.cache, args.offline, args.submission)
    else:
        build_deb(lock, args.cache, args.out, args.offline)


if __name__ == "__main__":
    main()
