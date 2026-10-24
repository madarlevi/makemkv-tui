# Contributing to makemkv-tui

Use Go 1.25.0 or newer. Keep changes focused and describe user-visible behavior.
Open an issue for broad architecture changes. No proprietary engine binaries,
source, media, registration keys or private runtime plans belong in contributions.

## Build and test

```sh
go mod download
gofmt -w ./cmd ./internal ./tools
go vet ./...
go test ./...
CGO_ENABLED=0 GOOS=freebsd GOARCH=amd64 go build -o makemkv-tui-freebsd-amd64 ./cmd/makemkv-tui
```

On native Unix with a C compiler, also run `go test -race ./...`. Cross-builds
verify compilation only. Document your OS, Go/engine version and checks in PRs.
Tests must run without MakeMKV, real discs or a graphical desktop.

## Architecture

| Directory | Responsibility |
| --- | --- |
| cmd/makemkv-tui | CLI, lifecycle, signals and state-path selection |
| internal/makemkv | Robot CSV parsing and process runner |
| internal/media | Engine-independent titles and stream metadata |
| internal/backend | Engine contract, MakeMKV adapter, optional remux |
| internal/batch | Discovery, planning, scheduler and recovery |
| internal/batchstate | Schema, validation, atomic JSON, catalog and OS locks |
| internal/publish | Validated, non-overwriting finalization |
| internal/proccontrol | Subprocess cancellation and Unix process groups |
| internal/tui | Shared explorer, batch screens, logs and error views |
| internal/preferences | Versioned frontend preferences |
| tools | Reproducible release packaging and renderer captures |

## Preservation rules

Never publish before durable engine-success/verification checkpoints. Never mark
completed before validating the final output. Never clean staging until completion
state is saved. Keep verified completed titles across retries and both stop modes.
Never use unchecked rename over an existing output, unsafe copy-overwrite, shell
command interpolation, or hard links for MKV publication. Add fault-injection and
recovery tests for any change to publication, cleanup or persistent state.

Keep schema-1 plans backward-readable; new fields must be optional or have a
migration strategy and legacy tests. Test old plan paths, Unicode, collisions,
full filesystem errors, interrupted copies, both stops and failed state saves.
See [batch protocol](docs/BATCH.md). Missing stream metadata must stay unavailable;
source per-track selection must not be advertised without verified engine support.

## Documentation and licensing

Update help, README, keyboard tables and CHANGELOG with user-facing changes.
Renderer captures use synthetic data; never publish private paths or media names.
Run the commands in docs/screenshots/README.md when changing UI captures.

By contributing, you agree to license your original contribution under the root
MIT license. Identify third-party code, its license and required notices in the PR.
Do not copy engine implementation code into this repository. Dependency changes
must update THIRD_PARTY_LICENSES.txt and include an attribution review.
