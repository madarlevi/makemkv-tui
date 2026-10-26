# v0.2.0 release checklist

Before tagging:

- [ ] Review the complete staged diff and preserve existing remote history.
- [ ] Confirm .go-version, supported Action/runtime versions and go.sum.
- [ ] Run gofmt, go test ./..., go vet ./... and supported native race tests.
- [ ] Run Python release/packaging tests and actionlint.
- [ ] Build the five-target preview into a new directory; verify archives/checksums.
- [ ] Confirm version output, BUILDINFO identity, executable headers/modes and notices.
- [ ] Verify no engine binaries, keys, personal media/state or old dist files are staged.
- [ ] Push main and inspect native Linux/Windows/macOS CI and Debian checks.
- [ ] Optionally run the Release workflow manually; ensure publish is skipped.
- [ ] Review native FreeBSD/engine/FUSE/SSH stop/resume results separately; record limits.
- [ ] Ensure GitHub Actions can publish with its token and no old v0.2.0 tag exists.

Owner publication:

- [ ] Follow docs/RELEASE.md PowerShell commands to create and push v0.2.0.
- [ ] Verify the tag resolves to the reviewed main commit.
- [ ] Wait for all required validation jobs and exactly one public Release.
- [ ] Download all five archives plus SHA256SUMS.txt; independently verify them.
- [ ] Run the executable on intended native systems; external engine is separately licensed.

After the actual release exists:

- [ ] Run packaging/tools/finalize.py --tag v0.2.0 --out dist/packaging/v0.2.0.
- [ ] Inspect real URLs/hashes, source commit, generated .deb and all package definitions.
- [ ] Run WinGet/Scoop, Ports, Debian, AUR and Homebrew native validation where available.
- [ ] Commit only intentionally maintained manifests/metadata; dist/ remains ignored.
- [ ] Submit packages manually using packaging/README.md. Do not claim acceptance first.

Never force-retag, overwrite public assets or fill in guessed checksums to bypass
validation. Failed uploads may leave an owned draft; use the documented retry path.
