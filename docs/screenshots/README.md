# Application captures

These SVGs are **actual makemkv-tui View renderer output with synthetic data**.
The opt-in Go test installs representative models and calls the same View methods
used by Bubble Tea; it does not draw an imitation interface or run an engine.
The ANSI originals are committed alongside each image for inspection. Synthetic
progress is explicitly labeled. No real media, usernames, license keys or private
paths are used. These are not live FreeBSD/MakeMKV session captures.

Regenerate from the repository root (POSIX shell):

```sh
MAKEMKV_TUI_CAPTURE_DIR="$PWD/docs/screenshots" go test ./internal/tui -run '^TestCaptureScreenshots$' -count=1
go run ./tools/rendercapture --dir docs/screenshots
```

PowerShell:

```powershell
$env:MAKEMKV_TUI_CAPTURE_DIR = (Resolve-Path docs/screenshots).Path
go test ./internal/tui -run '^TestCaptureScreenshots$' -count=1
Remove-Item Env:MAKEMKV_TUI_CAPTURE_DIR
go run ./tools/rendercapture --dir docs/screenshots
```

The SVG renderer preserves ANSI text colors and focus backgrounds, using a local
monospace font. Font glyphs may differ from your terminal. It has no external
assets or scripts. Captures cover title selection, expanded tracks, recursive
batch planning, synthetic progress and the saved summary.

To replace these with genuine interactive terminal screenshots, run the app with
synthetic fixture-backed engine output in a disposable folder. Capture the actual
terminal at a readable size, review every path/metadata/log for sensitive content,
commit sanitized PNGs here and update README image paths/captions. State which
platform/terminal/engine (or mock engine) was used; never present simulated engine
activity as an actual extraction. Do not modify real user plans to stage a capture.
