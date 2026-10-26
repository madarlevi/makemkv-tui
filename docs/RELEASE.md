# Release procedure

Target: **v0.2.0**. Run from a real checkout of
`https://github.com/madarlevi/makemkv-tui`. This preparation does not push, tag,
publish or submit anything. The release workflow will publish automatically
**when the owner pushes a valid version tag and all required jobs succeed**.

## CI and publication ownership

- `build.yml`: push/PR to main; native Linux, Windows and macOS tests/vet,
  native Unix race tests, five separate cross-builds, formatting, Python tooling
  tests and actionlint. It is reusable by the release workflow.
- `packaging.yml`: a source-only Debian preview build, reproducibility and native
  dpkg checks. It never downloads a not-yet-published application release.
- `release.yml`: the **only** workflow that writes GitHub Releases. Trigger:
  `push.tags: ['v*.*.*']`, with strict semantic-version validation after checkout.
- Manual `workflow_dispatch` always builds preview artifacts and **cannot publish**,
  even when its version label matches a production tag. It can test a selected
  branch before that tag exists.

The resolver checks out the event's exact source, verifies a tag push against its
peeled commit, and records commit/time once. Reusable CI, package checks, all five
builds, archive collection and checksum validation must succeed before publishing.
The final job revalidates downloaded bytes and rechecks the remote tag.

Checkout/setup-go/upload/download use verified immutable Node.js 24 Action pins.
Jobs have contents:read except the single publishing job's contents:write.
Checkout never persists credentials. No pull_request_target, secret-bearing PR
execution, MakeMKV download or third-party release action is used. Hosted runners
provide Node 24 support; old self-hosted runners require an upgrade before use.

Temporary Actions artifacts may be replaced on rerun. This affects **workflow
artifacts only**. Public Release assets are never deleted or overwritten.

## Toolchain, identity and archive contract

`go.mod` declares the application's minimum Go API level. **`.go-version` pins the
release compiler to Go 1.26.10**, the same compiler used by CI and release builds.
Builds use `CGO_ENABLED=0`, `-mod=readonly -trimpath -buildvcs=false`, no build ID,
and a linked version/commit identity. Dependency versions and go.sum are retained;
application libraries were not upgraded as part of a release infrastructure repair.

`--version` and `version` print `makemkv-tui v0.2.0` for that release.
`version --json` also shows commit, compiler, OS and architecture. `go install`
uses its module build version when release linker values are absent. An untagged
working-tree build can still say dev; a release artifact cannot pass validation
with dev. There is no hardcoded future tag in application code.

Go omits the `-ldflags` setting from BuildInfo with `-trimpath` enabled. Therefore
a linked, delimited identity string drives both the CLI and cross-platform
inspection. The verifier also reads real Go BuildInfo and ELF/PE/Mach-O headers:
correct package, compiler, OS, architecture, CGO=0, trimpath and executable type.
It validates the binary hash and checks the identity against the expected tag/SHA.
A native-host release build additionally executes `--version` without an engine.

For v0.2.0 exactly these assets are published:

```text
makemkv-tui-v0.2.0-freebsd-amd64.tar.gz
makemkv-tui-v0.2.0-linux-amd64.tar.gz
makemkv-tui-v0.2.0-windows-amd64.zip
makemkv-tui-v0.2.0-darwin-amd64.tar.gz
makemkv-tui-v0.2.0-darwin-arm64.tar.gz
SHA256SUMS.txt
```

Each archive has the correct executable, README.md, LICENSE, THIRD_PARTY.md,
THIRD_PARTY_LICENSES.txt and BUILDINFO.json. Unix binaries have 0755 mode; documents
0644. Windows also has executable permission metadata in its ZIP. Paths, member
counts, archive CRCs, file modes, internal hashes, source timestamps and documents
are checked. All five must have the same version/commit/compiler/source documents.
Unexpected files, stale artifacts, missing targets, unsafe paths and wrong hashes
fail. Release jobs use fresh runner-temp directories, never the checkout's dist/.

No proprietary engine, source component, registration/beta key or user data is
allowed in the strict archive allowlist. The MIT frontend and third-party notices
are included; MakeMKV remains a separately installed/licensed dependency.

## Local preview without a tag or publication

PowerShell, from the prepared source directory:

```powershell
$env:GOTOOLCHAIN = 'go' + (Get-Content .go-version -Raw).Trim()
$env:SOURCE_DATE_EPOCH = '1792844226' # Fixed sample timestamp for preview reproducibility
$Out = Join-Path $env:TEMP ('makemkv-release-preview-' + [guid]::NewGuid().ToString('N'))
go run ./tools/release --version v0.2.0 --commit uncommitted --preview --target all --out $Out
if ($LASTEXITCODE) { throw 'Preview build failed' }
go run ./tools/release --checksums --version v0.2.0 --commit uncommitted --preview --out $Out
if ($LASTEXITCODE) { throw 'Checksum generation failed' }
go run ./tools/release --verify --version v0.2.0 --commit uncommitted --preview --out $Out
if ($LASTEXITCODE) { throw 'Archive verification failed' }
python tools/release/release.py --preview --tag v0.2.0 --commit uncommitted --dir $Out
if ($LASTEXITCODE) { throw 'Dry-run validation failed' }
```

These are explicitly uncommitted preview artifacts. They cannot be published by
the production workflow or used to finalize package manifests. Repeat into another
empty directory with the same source/compiler/epoch and compare archive hashes.
Do not use real-looking invented commit hashes. Existing files are never silently
replaced; choose a fresh output directory instead of deleting user files.

For a GitHub preview before tagging, push the changes to main, then choose
Actions → Release → Run workflow, select main and version label v0.2.0. It runs
the full gates and uploads verified preview artifacts. The publish job is skipped.

## Apply the finished changes without replacing Git history

The supplied prepared source copy has no usable Git history. Do not initialize a
new unrelated history and force-push it over the existing repository. A patch is
provided against upstream commit `5a0c6950910ced230557ed88810c07629bed9e41`.
From the prepared source folder, use a **new** checkout, or apply the patch in your
existing clean clone. Git apply --check refuses conflicting newer changes.

```powershell
$ErrorActionPreference = 'Stop'
$Patch = (Resolve-Path '..\makemkv-tui-v0.2.0-preparation.patch').Path
$Checkout = Join-Path (Split-Path (Get-Location) -Parent) 'makemkv-tui-github'
if (Test-Path -LiteralPath $Checkout) { throw 'Choose a new checkout path or use your existing clean clone' }
git clone https://github.com/madarlevi/makemkv-tui.git $Checkout
if ($LASTEXITCODE) { throw 'Clone failed' }
Set-Location $Checkout
git switch main
if ($LASTEXITCODE) { throw 'Switch failed' }
git apply --check $Patch
if ($LASTEXITCODE) { throw 'Patch conflicts: inspect and resolve; do not overwrite newer work' }
git apply $Patch
if ($LASTEXITCODE) { throw 'Patch application failed' }
git status --short
git add --all
git diff --cached --check
if ($LASTEXITCODE) { throw 'Whitespace check failed' }
git diff --cached --stat
# Review the staged diff, then:
git commit -m 'Repair CI, release integrity and two-stage package distribution'
if ($LASTEXITCODE) { throw 'Commit failed' }
git push origin main
if ($LASTEXITCODE) { throw 'Push failed; follow repository branch protection rather than force pushing' }
```

If main requires a PR, create a branch before committing, push that branch, and
merge the reviewed PR through GitHub. The tag must point at the resulting main
commit after required checks pass. No force-push is needed.

## Tag and automatically publish v0.2.0

After the main CI and optional GitHub preview have succeeded, from your normal
clone (these commands are for the owner; none were executed by the agent):

```powershell
git switch main
if ($LASTEXITCODE) { throw 'Switch failed' }
git pull --ff-only origin main
if ($LASTEXITCODE) { throw 'Update failed' }
if (git status --porcelain) { throw 'Working tree must be clean' }
$ExistingLocal = git tag --list v0.2.0
$ExistingRemote = git ls-remote --tags origin refs/tags/v0.2.0
if ($LASTEXITCODE) { throw 'Remote tag lookup failed' }
if ($ExistingLocal -or $ExistingRemote) { throw 'v0.2.0 already exists; inspect it, never recreate or force-replace it' }
git tag -a v0.2.0 -m 'makemkv-tui v0.2.0'
if ($LASTEXITCODE) { throw 'Tag creation failed' }
git push origin refs/tags/v0.2.0
if ($LASTEXITCODE) { throw 'Tag push failed' }
```

That final push triggers automatic publication after validation. No manual
production publish input, environment approval or later button is required by
the workflow. Repository/organization restrictions may still prohibit token
writes; configure those before tagging. Use tag protection/immutable release
settings where available; never retag a published version.

## Retry and failure policy

Publication uses the official API to create **one private draft**, upload only the
six verified files, download/check the uploaded bytes, then switch draft=false.
The draft is a transaction boundary, not a manual approval gate.

- A failure before publication leaves no public success release. An owned partial
  draft may remain for diagnosis and retry.
- Rerunning the original tag workflow can resume only a draft bearing this exact
  workflow/tag/commit ownership marker. It uploads missing assets only, verifying
  every existing byte first.
- An identical public release is a no-op. An incomplete/different public release,
  foreign draft, unexpected asset or moved tag causes a hard failure. No assets
  are deleted, replaced or clobbered. Re-running all jobs rebuilds deterministic
  artifacts; rerunning just failed jobs can reuse validated same-run artifacts.
- If publication succeeded but a response was lost, retry verifies the public
  bytes and exits successfully without mutation.
- Do not attach additional assets to an existing release and expect a later
  publishing retry to ignore them; that is a deliberate hard-stop condition.

## Troubleshooting Actions

1. Open the **first failing step**, not just the final skipped publish job.
2. For native tests download tests-OS artifacts: native-tests.jsonl and, on Unix,
   race-tests.jsonl. They retain failing test names/output even when the job fails.
3. For the historical run 40093371918, public job metadata was accessible but log
   downloads returned HTTP 403. Annotations showed Node 20 deprecation and test
   exit 1; the exact original test failure was unavailable and is not claimed fixed
   from those annotations. The matching code passed locally with Go 1.25.0 and
   1.26.5; the new pinned compiler and hardened tests were also tested locally.
4. A formatting failure lists files: run gofmt -w cmd internal tools and commit.
5. Missing/extra artifact or identity error: inspect BUILDINFO.json, source SHA,
   compiler and artifact names. Never substitute an older dist/ archive.
6. Permission 403 publishing: check Actions write-token policy, tag protection and
   the job's contents:write permission. Do not add a personal token to source.
7. Upload response/network failure: retain the draft and rerun the original tag
   workflow. Do not use --clobber, delete public assets or move the tag.
8. Go proxy 404 during package finalization: the public release/tag must exist;
   allow proxy propagation and retry with a fresh output directory.

FreeBSD compilation is not native FreeBSD/engine/FUSE testing. Native Linux/macOS
CI and race checks cannot be claimed successful until your new Actions run passes.
See the release checklist and local verification report for exact limits.
