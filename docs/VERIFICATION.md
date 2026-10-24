# Release preparation verification

Local verification: 2026-10-24. Host: Windows amd64. Toolchain:
`go version go1.26.5 windows/amd64`. Module declares Go 1.25.0; CI installs the
version from go.mod. GitHub Actions has been prepared, not executed in this workspace.

## Completed checks

| Check | Result |
| --- | --- |
| `gofmt -l cmd internal tools` | Empty output: formatted |
| `go vet -mod=readonly ./...` | Passed |
| `go test -count=1 -mod=readonly -json ./...` | 93 top-level tests passed across 10 tested packages; no failures |
| Default skipped test | One: opt-in TestCaptureScreenshots |
| Explicit TestCaptureScreenshots run | Passed; generated five actual View renderer captures with synthetic fixture data |
| Capture review | All five SVGs rasterized and visually inspected |
| Workflow YAML | Both files parsed with unique-key checking; Action references use 40-character commit pins |
| Windows packaged executable | `--version` returned makemkv-tui v0.1.0; `--help` and `batch --help` exited successfully |
| Release archive contents | All five contain executable, LICENSE, README.md, THIRD_PARTY.md and THIRD_PARTY_LICENSES.txt only |
| Archive integrity | All five SHA256SUMS entries verified; Unix executable modes checked |
| FreeBSD binary format | ELF64, amd64 verified |
| Reproducibility | Two independent FreeBSD archive builds were byte-identical with the same source/toolchain/epoch |

All five release-tool builds succeeded with CGO_ENABLED=0:

- freebsd/amd64
- linux/amd64
- windows/amd64
- darwin/amd64
- darwin/arm64

Reproducibility check used version v0.1.0 and SOURCE_DATE_EPOCH=1792800000.
These are **local candidate archives**, not evidence that a v0.1.0 tag or public
release exists. GitHub tag releases derive the epoch from their commit timestamp.

The tests cover mocked extraction, preservation after FUSE-like EIO, no-replace
collisions and races, write/sync/close failures, completion-save failures, staged
receipt recovery, prepared-copy reuse, interrupted extraction, completed-title
skipping, both stop modes, JSON/locking, legacy schema-1 plans and Unicode paths.
No test was removed to obtain a passing result. Optional recovery fields retain
compatibility when loading older schema-1 plans.

## Limits and remaining manual checks

No MakeMKV engine or real media was executed here. Native FreeBSD/FUSE/NTFS,
real optical-drive access, SSH disconnect/restart behavior with a real engine,
and real Windows engine process-tree termination remain unverified. Unix-specific
signal/process tests require a native Unix runner. Native Linux/macOS tests and
race detection are configured in CI; they have not been run locally. No local C
compiler is available for Go race instrumentation.

The original FreeBSD FUSE kernel/driver EIO cannot be diagnosed definitively from
this host. The fix removes reliance on the failing hard-link operation, tests
preservation under simulated failures, and documents the non-atomic safe-copy
fallback. Real mount validation with expendable inputs is a pre-release task.

The scans found no old project-generation naming in repository contents, obvious
private developer paths, common credential patterns, bundled engine components or
real media. Such pattern scans are not a guarantee that all possible secrets are
absent; the owner should review staged files before the first commit.

`git status --short` reports that this source directory is not a Git repository.
No Git initialization, commit, push, tag or release publication was performed.
See RELEASE.md for those explicit owner actions and the draft-release review.

Licensing review: MIT owner confirmed as madarlevi. Third-party notices are
included. go-localereader v0.0.1 declares MIT and its author in README but has no
standalone license file; the declaration and terms are included with this
provenance documented. MakeMKV and optional MKVToolNix remain separately licensed
external installations. No formal legal opinion or dependency vulnerability audit
is claimed; review current advisories before the first public release.
