# Changelog

## Unreleased — initial public release preparation

- Adopt github.com/madarlevi/makemkv-tui and verified 2026 madarlevi MIT attribution.
- Add version reporting, public documentation, contribution/security templates,
  renderer captures, platform CI and draft-only release packaging with checksums.
- Replace MKV hard-link publication with native no-replace rename where available
  and a validated exclusive-copy fallback. Preserve staged successes on failure;
  recover success receipts/checkpoints before re-extraction. Clean attempts only
  after final validation and durable completion state. Add full Ctrl+E errors and
  regression tests for publication faults, races, cancellation and crash windows.
- Keep schema 1 readable with optional staged-artifact and engine-success fields.

No version tag or public release has been created by this preparation.

## Color-coded title and track explorer

The existing single-source and batch pickers now share an expandable scrolling
tree, red VIDEO / green AUDIO / blue SUB labels, focus highlighting, a color legend
and a structured metadata pane. Adds NO_COLOR and ANSI palette fallbacks, bounded
Unicode rendering, parent-aware folding and raw-protocol isolation in diagnostic
logs. Retains SINFO message codes alongside per-title/per-stream attributes for
localized type classification; legacy plans remain readable. Enter/PgDn planning,
selection persistence, extraction confirmation and both stop modes are preserved.
No extraction commands or track-selection capabilities were added. Fixture tests
cover multiple tracks, missing/unknown metadata, CSV, Unicode, resizing and large
trees; unavailable HDR/video bit-depth/chapter fields remain explicitly limited.

## FreeBSD FUSE batch-state persistence fix

Default plans now use XDG/HOME local state storage with descriptive persistent
batch IDs, printed paths, saved-plan browsing/listing, duplicate-plan detection
and an explicit --new override. Legacy explicit resume paths stay in place.
State initialization no longer calls os.Root.Link/linkat: both initial saves and
updates sync a unique temporary file, close it, and publish with os.Rename before
syncing the directory. Write/sync/close/rename failures preserve the prior plan;
directory-sync errors explicitly report uncertain durability and halt execution.
Adds filesystem fault tests, local/legacy path tests and plan chooser coverage.

## Persistent interactive ISO batches

Adds recursive deterministic discovery, the existing title picker as a saved planning wizard, preserved output directories, atomic JSON state, OS-backed locks, title-level checkpoints/recovery, verified no-replace publication, immediate and after-ISO stops, retry history, source/title identity checks, live batch progress and logs. Single-source behavior remains available. Batch filesystem confinement now requires Go 1.25; no new module dependency was added.

## Parity foundation increment

Preserves ISO discovery, title multiselect, extraction, progress, cancellation, CSV parsing and mocked runner tests. Adds an engine contract, source browser, drive discovery, local title/stream expansion, named attributes and chapter counts, output filenames, atomic no-replace publishing, versioned preferences, session logs and optional MKVToolNix track review/remux.

The official 2.0.0 GUI RPC control path was inspected and documented. Direct source-track selection and detailed chapter control remain future work behind backend capabilities. Added MIT licensing, dependency notices, go.sum, regression tests and full target-platform cross-builds.

