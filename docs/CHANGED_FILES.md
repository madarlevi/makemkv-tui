# Initial release preparation changes

The list below records the initial release preparation. The subsequent native
CI test fixes are documented in [CI_TEST_FIX.md](CI_TEST_FIX.md).

Compared with upstream commit `5a0c6950910ced230557ed88810c07629bed9e41`.
This list includes itself. Historical package snapshots were moved under
`packaging/history/v0.1.0/`; removals below are old locations, not lost history.
Generated binaries, dist/, Python caches and local runtime state are excluded.

The module, dependencies, MIT license, third-party notices, batch JSON schema,
extraction, publication, recovery and stop implementations remain unchanged.
Application changes are limited to version reporting; three test files were hardened.

## Added (47)

- `.github/workflows/packaging.yml`
- `.go-version`
- `cmd/makemkv-tui/version.go`
- `cmd/makemkv-tui/version_test.go`
- `docs/CHANGED_FILES.md`
- `docs/CI_RELEASE_VALIDATION.md`
- `docs/RELEASE_CHECKLIST.md`
- `packaging/README.md`
- `packaging/aur/.SRCINFO.in`
- `packaging/aur/PKGBUILD.in`
- `packaging/debian/README.Debian`
- `packaging/debian/control.in`
- `packaging/freebsd/multimedia/makemkv-tui/Makefile.in`
- `packaging/freebsd/multimedia/makemkv-tui/pkg-descr`
- `packaging/freebsd/multimedia/makemkv-tui/pkg-message`
- `packaging/freebsd/multimedia/makemkv-tui/pkg-plist`
- `packaging/history/v0.1.0/README.md`
- `packaging/history/v0.1.0/SHA256SUMS`
- `packaging/history/v0.1.0/VALIDATION.md`
- `packaging/history/v0.1.0/aur/.SRCINFO`
- `packaging/history/v0.1.0/aur/PKGBUILD`
- `packaging/history/v0.1.0/freebsd/multimedia/makemkv-tui/Makefile`
- `packaging/history/v0.1.0/freebsd/multimedia/makemkv-tui/distinfo`
- `packaging/history/v0.1.0/freebsd/multimedia/makemkv-tui/pkg-descr`
- `packaging/history/v0.1.0/freebsd/multimedia/makemkv-tui/pkg-message`
- `packaging/history/v0.1.0/freebsd/multimedia/makemkv-tui/pkg-plist`
- `packaging/history/v0.1.0/homebrew/Formula/makemkv-tui.rb`
- `packaging/history/v0.1.0/release-lock.json`
- `packaging/history/v0.1.0/scoop/bucket/makemkv-tui.json`
- `packaging/history/v0.1.0/winget/manifests/m/madarlevi/makemkv-tui/0.1.0/madarlevi.makemkv-tui.installer.yaml`
- `packaging/history/v0.1.0/winget/manifests/m/madarlevi/makemkv-tui/0.1.0/madarlevi.makemkv-tui.locale.en-US.yaml`
- `packaging/history/v0.1.0/winget/manifests/m/madarlevi/makemkv-tui/0.1.0/madarlevi.makemkv-tui.yaml`
- `packaging/homebrew/Formula/makemkv-tui.rb.in`
- `packaging/makemkv-tui.1`
- `packaging/scoop/makemkv-tui.json.in`
- `packaging/tools/finalize.py`
- `packaging/tools/package.py`
- `packaging/tools/test_finalize.py`
- `packaging/tools/test_package.py`
- `packaging/winget/madarlevi.makemkv-tui.installer.yaml.in`
- `packaging/winget/madarlevi.makemkv-tui.locale.en-US.yaml.in`
- `packaging/winget/madarlevi.makemkv-tui.yaml.in`
- `tools/release/ci.py`
- `tools/release/release.py`
- `tools/release/test_release.py`
- `tools/release/validate.go`
- `tools/release/validate_test.go`

## Modified (15)

- `.gitattributes`
- `.github/workflows/build.yml`
- `.github/workflows/release.yml`
- `.gitignore`
- `CHANGELOG.md`
- `CONTRIBUTING.md`
- `README.md`
- `cmd/makemkv-tui/main.go`
- `docs/RELEASE.md`
- `docs/RELEASE_NOTES.md`
- `docs/VERIFICATION.md`
- `internal/makemkv/process_test.go`
- `internal/makemkv/process_unix_test.go`
- `internal/tui/explorer_test.go`
- `tools/release/main.go`

## Removed from old location (0)
