# CI and release preparation validation

Checked on **2026-10-26**, Windows amd64, pinned Go **1.26.10**. This is a local
preparation report, not a claim that GitHub Actions or a public release has run.
No commits, tags, pushes, Release publication or package submissions were made.

## Audit findings

The upstream source audited was commit
`5a0c6950910ced230557ed88810c07629bed9e41`. The supplied source directory has no
usable Git history. The companion preparation patch is intended for a clean
checkout of that existing history; do not replace it with a new initial commit.

- Historical run [40093371918](https://github.com/madarlevi/makemkv-tui/actions/runs/40093371918)
  failed native tests on Linux, Windows and macOS while cross-builds passed.
  Public metadata and annotations were readable; detailed log downloads returned
  HTTP 403. Annotations exposed exit 1 and Node 20 deprecation, without the test
  failure. The exact original failure remains unconfirmed. The matching old code
  passed locally on Windows with Go 1.25.0 and 1.26.5, including CI-like terminal
  environment variables.
- Test hardening addresses observed brittleness: color conversion can round the
  green channel to 73 or 74, and subprocess startup time is not cancellation time.
  Readiness-based timing, bounded contexts and isolated helper environment values
  keep the actual cancellation assertions. No important test was removed.
- Workflows used obsolete Node 20 actions, overlapping tag validation and a
  draft/manual gate inconsistent with automatic publication. Only release.yml now
  publishes, after reusable native, cross-build and package checks.
- Packaging checks depended on historical release downloads. Active recipes now
  use templates before publication and actual verified assets afterward. Historical
  files remain offline fixtures, not current submission manifests.
- Loose archive/checksum collection did not establish a complete same-tag,
  same-commit set. The replacement verifies five targets, real executable headers,
  embedded identity, required notices, modes, hashes and the exact asset allowlist.
- Public asset retries previously lacked a complete no-overwrite contract. The
  publisher verifies an owned draft, uploads only missing files, verifies remote
  bytes and publishes once. An identical public release is a no-op; conflicts fail.

## Local results

| Check | Result |
| --- | --- |
| `gofmt -l cmd internal tools` | Passed; no unformatted files |
| `go vet ./...` | Passed |
| `go mod verify` | Passed; all modules verified |
| `go test -count=1 -timeout=5m ./...` | Passed on Windows; 97 top-level tests passed, one opt-in screenshot capture skipped |
| Python release tests | 6 passed, using a fake GitHub API; no publication |
| Python packaging tests | 9 passed, including synthetic post-release rendering and source integrity |
| actionlint 1.7.12 | All three workflows passed; shellcheck unavailable and explicitly disabled |
| Workflow YAML | Parsed with duplicate-key checking |
| `bash -n` | AUR template and rendered PKGBUILD passed |
| WinGet 1.12.0 and Scoop schemas | Synthetic rendered fixtures passed official JSON schema validation |
| Five-platform preview | FreeBSD amd64, Linux amd64, Windows amd64, macOS amd64/arm64 built with CGO disabled |
| Executable metadata | Real ELF, PE and Mach-O headers, Go BuildInfo, linked version/commit and binary hashes passed |
| Archive integrity | All five archives extracted/parsed and passed Go and Python validation; SHA256SUMS.txt matched actual archives |
| Native version smoke | Windows executable reported v0.2.0; JSON reported uncommitted preview identity, go1.26.10 and windows/amd64 |
| Repeat FreeBSD build | Byte-identical archive with fixed source, compiler and SOURCE_DATE_EPOCH |
| Debian preview | Two independent `0.2.0~ci-1` packages were byte-identical; ar/tar metadata, documentation, modes and checksums inspected |
| Stage-two live gate | Actual v0.2.0 Release API returned HTTP 404; generator refused to create submission manifests |

The preview Debian SHA256 was
`3759ee7d715d3a63c6776bce5a34e6326642149a3e75f7a54879ad50d991932d`.
This identifies a local preview, **not** a public package or a submission checksum.
Preview archives use `preview=true` and `commit=uncommitted`; production validation
rejects that identity. Release jobs build fresh artifacts from the real tag SHA.

## Limits requiring native or post-release checks

- `go test -race ./...` was attempted locally with CGO enabled but could not build:
  this Windows environment has no GCC/C toolchain. No race test success is claimed.
  Required native Linux/macOS CI race jobs retain CGO enabled and fail normally.
- Linux and macOS native tests, native FreeBSD execution, real MakeMKV, SSH
  cancellation and FUSE/NTFS behavior were not run here. Cross-compilation only
  proves compilation. Mocked scheduler, publication and recovery tests remain.
- FreeBSD Ports/portlint/poudriere, WinGet install validation, Homebrew/Ruby, AUR
  makepkg and native dpkg/lintian installation checks were unavailable locally.
  CI defines native Linux dpkg inspection and frontend smoke checks; its results
  must be reviewed after pushing. Schema checks are not native package installs.
- v0.2.0 did not exist at the live finalizer check. Its real release URLs, hashes,
  Go-proxy source and completed package-manager manifests must be generated after
  publication. No fabricated production checksums were added.
- `go install ...@latest` depends on the actual public module/tag. Local version
  fallback tests passed; this preparation does not certify a future public install.
- The new hosted Actions jobs have not been executed from this source copy.
  The owner must inspect the first complete main CI run and optional manual Release
  preview before pushing the production tag.

## Scope and handoff

The batch JSON schema, extraction scheduler, publication/recovery implementation
and stop behavior are unchanged. The only application behavior added is version
reporting and module-version fallback. Existing module/dependency and license files
are retained; no MakeMKV components or keys are bundled.

See [CHANGED_FILES.md](CHANGED_FILES.md) for every source change,
[RELEASE.md](RELEASE.md) for exact PowerShell apply/commit/push/tag commands and
[the packaging guide](../packaging/README.md) for manual submission procedures.
