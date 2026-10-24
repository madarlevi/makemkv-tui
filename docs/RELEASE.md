# Repository and release preparation

## Repository identity

Repository/application: makemkv-tui. Owner confirmed as madarlevi. Module and
internal imports are github.com/madarlevi/makemkv-tui. MIT copyright: 2026 madarlevi.
No unresolved owner placeholder remains. A development build reports `dev`;
versioned archive builds embed their supplied tag in `--version`.

This workspace had no .git metadata. No repository was initialized, history
removed, commit created, remote contacted for pushing, or public release published.

Create an **empty** GitHub repository named makemkv-tui under madarlevi first.
From the source root, review files before committing:

```sh
git init -b main
git add .
git status --short
git diff --cached --check
git diff --cached --stat
git commit -m "Prepare makemkv-tui for initial public release"
git remote add origin https://github.com/madarlevi/makemkv-tui.git
git push -u origin main
```

Do not reinitialize an existing checkout or force-push over existing history. If
GitHub already contains commits, fetch and reconcile those deliberately instead.
Runtime plans saved under arbitrary custom JSON names must be kept outside the
checkout or explicitly ignored; no ignore rule can identify every custom name.
Check staged content for secrets, private paths, logs and engine keys.

## Before the first tag

- Run formatting, vet, tests and all target builds; review CI results on GitHub.
- Run real FreeBSD engine, FUSE/NTFS finalization, both stops and resume tests using
  expendable sources. Cross-builds and mocked faults are not native media tests.
- Review README claims, license/third-party notices and sample captures.
- Enable private vulnerability reporting and appropriate branch protection.
- Create an environment named `release` in GitHub settings, with required owner
  review if supported by your account. The workflow creates drafts even if an
  environment approval rule has not been configured.
- Review pinned Action commits and dependency advisories before publication.
  Pins are immutable, not an assurance that future vulnerabilities cannot arise.

## Reproducible packaging

Run from the repository root. The standard-library Go packaging tool refuses
existing archive/checksum paths, validates tags/targets, builds with CGO disabled,
`-trimpath -buildvcs=false`, stripped symbols and an embedded version. It includes
LICENSE, README, THIRD_PARTY.md and THIRD_PARTY_LICENSES.txt, never engine files.

```sh
export SOURCE_DATE_EPOCH="$(git show -s --format=%ct HEAD)"
go run ./tools/release --version v0.1.0 --target freebsd/amd64 --out dist
go run ./tools/release --version v0.1.0 --target linux/amd64 --out dist
go run ./tools/release --version v0.1.0 --target windows/amd64 --out dist
go run ./tools/release --version v0.1.0 --target darwin/amd64 --out dist
go run ./tools/release --version v0.1.0 --target darwin/arm64 --out dist
go run ./tools/release --checksums --out dist
```

PowerShell: set `$env:SOURCE_DATE_EPOCH` to the commit's integer Unix timestamp
before the same `go run` commands. Without this variable, local scratch builds
use epoch 0 (ZIP dates clamp to 1980). Reproducibility requires identical source,
dependencies, Go version, target and epoch. Build twice into **different empty
output directories** and compare archive SHA256 values. Never replace published
assets under the same version; issue a new version for changes.

Expected names: makemkv-tui-v0.1.0-freebsd-amd64.tar.gz,
makemkv-tui-v0.1.0-linux-amd64.tar.gz,
makemkv-tui-v0.1.0-windows-amd64.zip, macOS equivalents, and SHA256SUMS.
Checksums detect corruption; no cryptographic signing infrastructure is configured.

## First release

After the above review and explicit owner authorization:

```sh
git tag -a v0.1.0 -m "makemkv-tui v0.1.0"
git push origin v0.1.0
```

The tag workflow validates the tag, runs native mocked tests/vet/race checks and
cross-builds, packages all targets, computes checksums and creates a **draft**.
Manual workflow runs must select a version tag; branch runs are refused.
Only the final draft job has contents:write permission. No engine is downloaded.

Download and inspect every draft archive, verify checksums, test intended hosts,
edit candidate release notes to actual results and known limitations, then publish
manually in GitHub. Creating/pushing tags or publishing is not part of this local
preparation. Failed reruns do not overwrite an existing release automatically;
inspect any existing draft before deliberately updating/removing it.

## Structure

```text
makemkv-tui/
├── cmd/makemkv-tui/
├── internal/{backend,batch,batchstate,makemkv,media,preferences,proccontrol,publish,tui}/
├── tools/{release,rendercapture}/
├── docs/{BATCH.md,ENGINE_RESEARCH.md,RELEASE.md,VERIFICATION.md,screenshots/}
├── .github/{workflows/,ISSUE_TEMPLATE/,pull_request_template.md}
├── go.mod, go.sum
└── README.md, LICENSE, THIRD_PARTY*, CONTRIBUTING.md, CHANGELOG.md, SECURITY.md
```
