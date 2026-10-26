# Two-stage package distribution

The six formats remain in this repository. Maintainer: **Madár Levente
<madar.levi102@gmail.com>**. The frontend is MIT licensed and requires the separately
installed/licensed makemkvcon engine. No package bundles MakeMKV components or keys.
No Ports entry, WinGet/Scoop acceptance, AUR submission, tap publication or package
repository submission is performed by the tooling.

## Stage 1 — before v0.2.0 exists

The active recipes are **templates**:

```text
packaging/freebsd/multimedia/makemkv-tui/Makefile.in
packaging/freebsd/multimedia/makemkv-tui/{pkg-plist,pkg-descr,pkg-message}
packaging/winget/*.yaml.in
packaging/scoop/makemkv-tui.json.in
packaging/debian/{control.in,README.Debian}
packaging/aur/{PKGBUILD.in,.SRCINFO.in}
packaging/homebrew/Formula/makemkv-tui.rb.in
```

Tokens such as @VERSION@ and @SOURCE_SHA256@ are deliberately not installable
manifests. No guessed v0.2.0 hash/download is committed as submission-ready data.
`history/v0.1.0/` preserves the previous audited snapshots and inconsistent release
recipes as **offline fixtures only**. The public tags/releases were absent at the
latest repository audit; CI never downloads those historical URLs.

Before release:

```powershell
python -m unittest discover -s packaging/tools -p 'test_*.py' -v
python -m unittest discover -s tools/release -p 'test_*.py' -v
python packaging/tools/package.py deb-local --version v0.2.0 --out dist/deb-preview
```

`deb-local` packages the working tree as `0.2.0~ci-1` for format/reproducibility
validation only. It does not claim a published source URL. CI builds two such
previews, compares them, inspects them with dpkg and runs the extracted Linux
frontend's --version. This removes the old circular dependency on unavailable
release assets. Local preview generation does not publish anything.

## Stage 2 — after Actions publishes the actual release

Use Go matching `.go-version` and Python 3.10+; run from the repository root:

```powershell
$env:GOTOOLCHAIN = 'go' + (Get-Content .go-version -Raw).Trim()
python packaging/tools/finalize.py --tag v0.2.0 --out dist/packaging/v0.2.0
if ($LASTEXITCODE) { throw 'Finalization failed; do not submit partial output' }
```

The finalizer only performs read requests. It:

1. Requires a real public, complete, non-prerelease GitHub Release for the tag.
2. Resolves the tag to its actual commit, including annotated tags.
3. Fetches the five actual API-listed assets and SHA256SUMS.txt. It checks GitHub
   asset digests when available, byte sizes and canonical checksums.
4. Checks archive members, modes, identities, compiler, binary hashes and source
   consistency. The Go verifier independently reads real executable headers and
   embedded identity; it does not infer a version from the filename.
5. Downloads the actual API source tarball, checks its commit-named root and module,
   and compares every released document with the tagged source.
6. Downloads the real Go proxy .mod and .zip required by FreeBSD Ports, hashes them
   and verifies the module/source agreement. A proxy propagation delay fails safely;
   wait and retry. It never invents a source or module checksum.
7. Writes all five manager recipes plus a real Debian .deb built from the verified
   Linux release executable, licenses and documentation. It records a new JSON lock
   with actual URLs, hashes, source commit/compiler/epoch.

Output directories must not exist; no existing recipes are overwritten. Cache
bytes are rehashed; changed inputs are not silently blessed. Failed generation
may leave diagnostic partial output; inspect it and choose a new output path for
retry. Download caches default to `.cache/packaging/v0.2.0/` and are ignored.
Generated manifests/packages stay under ignored dist/. Review and deliberately
copy only files you intend to maintain or submit.

To reverify the recorded bytes or rebuild Debian from the verified tagged source:

```powershell
python packaging/tools/package.py verify --lock dist/packaging/v0.2.0/release-lock.json --cache .cache/packaging/v0.2.0 --offline --submission
python packaging/tools/package.py deb --lock dist/packaging/v0.2.0/release-lock.json --cache .cache/packaging/v0.2.0 --out dist/debian-source-build
```

The generated .deb is
`dist/packaging/v0.2.0/debian/makemkv-tui_0.2.0-1_amd64.deb`, with a real checksum.
Source rebuilds use the recorded compiler and dependencies. Identical source,
recipe, target, Go/Python/compression versions and source epoch are required for
byte-for-byte reproduction. No checksum or signature is claimed before the bytes
exist. Checksums are integrity checks, not cryptographic release signatures.

## FreeBSD Ports — multimedia/makemkv-tui

The generated source port uses `USES=go:modules`, the real Go module distfiles and
distinfo, staged installation and DOCS. Mandatory notices are installed even with
DOCS disabled. `makemkvcon:multimedia/makemkv` is a separate runtime package
requirement, not bundled engine content; its own licensing/Linux compatibility
requirements still apply.

On FreeBSD with a writable Ports tree and configured poudriere jail:

```sh
cp -R dist/packaging/v0.2.0/freebsd/multimedia/makemkv-tui /path/to/ports/multimedia/
# Add SUBDIR+= makemkv-tui in alphabetic order to multimedia/Makefile.
cd /path/to/ports/multimedia/makemkv-tui
portfmt -D Makefile
portclippy Makefile
portlint -AC
make makesum
# Compare generated distinfo against the finalizer's real hashes/sizes.
make stage
make stage-qa check-plist test
poudriere testport -j YOUR_AMD64_JAIL -p YOUR_PORTS_TREE -o multimedia/makemkv-tui
```

Then in the Ports checkout:

```sh
git add multimedia/Makefile multimedia/makemkv-tui
git diff --cached --check
git diff --cached > /tmp/makemkv-tui-port.patch
```

Submit that patch and logs as a Ports & Packages new-port request at
[FreeBSD Bugzilla](https://bugs.freebsd.org/), following the
[Porter's Handbook](https://docs.freebsd.org/en/books/porters-handbook/quick-porting/).
Native portfmt/portlint/poudriere checks remain required before submission.

## WinGet

The generator writes the three-file manifest tree under
`winget/manifests/m/madarlevi/makemkv-tui/0.2.0/`. It uses actual Windows ZIP URL/hash,
schema 1.12.0, nested portable `makemkv-tui.exe` and the stable command alias.
All required notices are now inside the validated upstream archive; the old
v0.1.0/v0.1.2 workaround is not used.

```powershell
$Manifest = 'dist/packaging/v0.2.0/winget/manifests/m/madarlevi/makemkv-tui/0.2.0'
winget validate --manifest $Manifest
# Only in a disposable test VM, enable local manifests if necessary:
winget settings --enable LocalManifestFiles
winget install --manifest $Manifest
makemkv-tui --version
winget uninstall --id madarlevi.makemkv-tui --exact
```

Fork [microsoft/winget-pkgs](https://github.com/microsoft/winget-pkgs), copy the three
tested files into that exact manifests path, commit them and open a PR. Include
release URL and test results. Schema validity alone does not imply client or
community-policy acceptance. Do not change the host setting just for schema tests.

## Scoop

```powershell
scoop install .\dist\packaging\v0.2.0\scoop\bucket\makemkv-tui.json
makemkv-tui --version
scoop uninstall makemkv-tui
```

The actual ZIP contains stable `makemkv-tui.exe` and complete notices, so no rename
script or extra source-notice downloads are needed. The generated manifest uses
SHA256 verification, GitHub release version checks and a checksum-file-based
update definition. Inspect update hashes before committing new versions.

For a personal bucket, manually create a repository and copy the manifest into
its root bucket/ directory, then `scoop bucket add NAME URL`. No bucket URL is
assumed to exist. For Main, fork [ScoopInstaller/Main](https://github.com/ScoopInstaller/Main),
copy to bucket/makemkv-tui.json, run its checkver/update tests and native
install/uninstall checks, then open a PR. No automatic submission occurs.

## Debian/Ubuntu

The generated amd64 package contains the executable, man page, README/batch/engine
docs, MIT license, full third-party notices, uncompressed copyright, changelog,
control metadata, Installed-Size and md5sums. It creates no services/system users
and never removes user plans, preferences, staging or completed output.
The engine is external; no unavailable Debian engine package name is fabricated.

```sh
dpkg-deb --info dist/packaging/v0.2.0/debian/*.deb
dpkg-deb --contents dist/packaging/v0.2.0/debian/*.deb
lintian dist/packaging/v0.2.0/debian/*.deb
# Only in a disposable Debian/Ubuntu VM:
sudo apt install ./dist/packaging/v0.2.0/debian/makemkv-tui_0.2.0-1_amd64.deb
makemkv-tui --version
sudo apt remove makemkv-tui
```

For direct binary distribution, review/sign as appropriate and publish only with
owner authorization. A Debian archive upload requires a source package, not just
this .deb: file an ITP via `reportbug wnpp`, package Go dependencies according to
Debian policy, build offline with sbuild, run lintian/autopkgtests, upload a signed
source package to mentors.debian.net and request sponsorship. An Ubuntu PPA also
requires a signed source upload. This recipe is not a claim of archive acceptance.

## AUR

```sh
cd dist/packaging/v0.2.0/aur
makepkg --verifysource
makepkg --cleanbuild --syncdeps
namcap PKGBUILD
namcap makemkv-tui-0.2.0-1-*.pkg.tar.zst
makepkg --printsrcinfo > .SRCINFO
```

The recipe hashes the actual tagged source, downloads Go dependencies during
prepare, builds with read-only/offline modules and preserves notices. The existing
AUR makemkv package is optional metadata but required for media processing and
keeps its own engine license. After native tests, register your AUR SSH key,
confirm the name is available, clone
`ssh://aur@aur.archlinux.org/makemkv-tui.git`, copy the tested PKGBUILD and regenerated
.SRCINFO, commit and push manually. If it already exists, contact its maintainer.

## Homebrew personal tap

The formula uses the real tagged source URL/hash and builds with Go on compatible
macOS/Linux systems. It does not invent bottles or macOS asset URLs. Engine
installation remains separate. On a Homebrew machine:

```sh
brew tap-new madarlevi/makemkv-tui
cp dist/packaging/v0.2.0/homebrew/Formula/makemkv-tui.rb "$(brew --repository madarlevi/makemkv-tui)/Formula/"
brew install --build-from-source madarlevi/makemkv-tui/makemkv-tui
brew audit --strict --formula madarlevi/makemkv-tui/makemkv-tui
brew test madarlevi/makemkv-tui/makemkv-tui
brew livecheck madarlevi/makemkv-tui/makemkv-tui
```

Reuse an existing local tap if present. After native macOS/Linux checks, manually
create a tap repository (normally homebrew-makemkv-tui) and push its Formula tree.
No such remote is created or presumed here. Homebrew/core submission has separate
acceptance criteria; this task targets a personal tap.

## Validation limits

Local template rendering, schemas, source/path safety, deterministic .deb structure,
release identity, five cross-builds and publisher retry behavior can be tested
without a release. Native FreeBSD Ports, WinGet client, full Scoop profile,
makepkg/namcap, dpkg/apt and brew tests require their respective systems/tools.
The Linux Actions workflow adds dpkg inspection and extracted frontend execution.
Cross-compilation does not establish native engine, optical drive, FUSE or SSH
behavior. Consult docs/CI_RELEASE_VALIDATION.md for the exact local results.
