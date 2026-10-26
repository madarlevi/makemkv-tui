#!/usr/bin/env python3
"""Resolve the checked-out source once; no network writes or tag creation."""
import argparse
import os
from pathlib import Path
import subprocess
from release import VERSION, COMMIT

p=argparse.ArgumentParser(description=__doc__)
p.add_argument("--version",required=True)
a=p.parse_args()
if not VERSION.fullmatch(a.version):
    raise ValueError("tag must be a semantic vMAJOR.MINOR.PATCH version")
def git(*args):
    return subprocess.check_output(["git",*args],text=True).strip()
commit=git("rev-parse","HEAD")
if not COMMIT.fullmatch(commit):
    raise ValueError("checkout does not resolve to a commit")
preview=os.environ.get("GITHUB_EVENT_NAME")!="push"
if not preview:
    if os.environ.get("GITHUB_REF")!="refs/tags/"+a.version:
        raise ValueError("production release requires an exact version-tag push")
    if git("rev-parse","refs/tags/"+a.version+"^{commit}")!=commit:
        raise ValueError("checked-out commit does not match the pushed tag")
epoch=git("show","-s","--format=%ct","HEAD")
if not epoch.isdecimal():
    raise ValueError("invalid source commit timestamp")
values={"version":a.version,"commit":commit,"epoch":epoch,"preview":str(preview).lower()}
with Path(os.environ["GITHUB_OUTPUT"]).open("a",encoding="utf-8") as f:
    for k,v in values.items():f.write(f"{k}={v}\n")
print("Resolved source",values)
