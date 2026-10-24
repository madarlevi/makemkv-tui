# Third-party materials and audit

The MIT license in LICENSE covers original makemkv-tui frontend code and original
capture/packaging tools. Copyright 2026 madarlevi is the confirmed owner's handle.
Dependencies keep their licenses; they are not relicensed by this repository.
Full collected notices accompany source and release archives in
[THIRD_PARTY_LICENSES.txt](THIRD_PARTY_LICENSES.txt).

## Audit scope (2026-10-24)

Reviewed repository source/fixtures/docs/assets, resolved module license files,
Go runtime license, and generated Unicode table attribution. No vendored MakeMKV
implementation, proprietary engine components, registration keys, beta keys or
real media are included. Application SVG captures use original rendering code and
synthetic fixture metadata; no vendor logo, artwork or fonts are embedded.

- Charmbracelet Bubble Tea, Bubbles, Lip Gloss and helpers; muesli helpers;
  terminfo, uniseg, colorful, go-osc52, coninput and mattn helpers declare MIT.
- atotto/clipboard, Go runtime and golang.org/x modules use BSD-style notices.
- Unicode-derived tables in rivo/uniseg also require Unicode data notices; the
  Unicode License V3 notice is included, separately from uniseg's MIT license.
- mattn/go-localereader v0.0.1 has **no standalone LICENSE file**. Its distributed
  README explicitly declares MIT and author Yasuhiro Matsumoto (mattn). That
  declaration, author attribution and standard MIT permission terms are included
  without inventing a copyright year. This upstream packaging gap is documented;
  future dependency updates should check whether upstream adds a full license.

The notice collection labels exact module versions. It includes available
resolved-graph module licenses, including some not linked on every platform.
Transitive test-only modules not distributed/linked are not bundled source.
`go.mod`/`go.sum` are authoritative for dependencies, and `go version -m BINARY`
shows modules linked into a particular target. This audit is not a legal opinion
or a guarantee about future dependencies.

## External engines

MakeMKV and optional MKVToolNix are independently installed external programs.
No MakeMKV source code or proprietary binaries are distributed. MakeMKV has its
own licensing and usage requirements: an open-source MIT frontend does not make
its engine permanently free. See the engine vendor's current terms. MKVToolNix
also retains its separate license. This project is not affiliated with either.

## Updating notices

After dependency changes, inspect `go list -m -json all` and all target binaries'
`go version -m` output. Read each module's LICENSE/COPYING/NOTICE and relevant
source/data headers, preserving complete attribution and Unicode notices. Update
this file and THIRD_PARTY_LICENSES.txt together; include both in release archives.
Do not assume a top-level MIT license covers third-party data or copied code.
