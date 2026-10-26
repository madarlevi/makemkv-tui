# Native CI test failures: 2026-10-26

The complete Linux, macOS and Windows JSON logs supplied for commit
`6912022c9565b9190c53c22ae988958364045017` establish the failures below. The
successful internal/makemkv output at the end of the job was not the error.
The release validation gate correctly prevented publication after tests failed.

| Failing test | Linux | macOS | Windows |
| --- | --- | --- | --- |
| TestChangedSourceAndOutputSymlinkRejected | Failed | Failed | Failed |
| TestPlanPathOverrideAndStableID | Passed | Failed | Failed |
| TestCatalogMatchingUnfinishedPlans | Passed | Failed | Failed |
| TestBatchLocalStorageDuplicateAndLegacyResume | Passed | Failed | Failed |

## Root causes and corrections

### Equivalent filesystem paths were compared as strings

Store.Open intentionally canonicalizes the state file's parent directory for
locking and safe I/O. On macOS, /var/folders and /private/var/folders can refer to
the same directory. Windows can return a full account directory instead of its
RUNNER~1 short spelling. A catalog path retains the spelling passed to Plans.
None of those differences means a legacy plan was migrated or lost.

The legacy-resume assertions now use os.Stat and os.SameFile after the save/load
cycle to prove that the original explicit path still names the updated plan.
The catalog assertion separately verifies its returned path and the file's actual
identity. Selection and ID assertions remain. An additional directory-alias test
checks that listing and updating through either spelling retains the same plan
and selected titles.

### The symlink test expected the wrong recovery return contract

A changed ISO is a batch-wide validation error returned by Recover. An unsafe
individual output is instead recorded as a failed title/ISO/batch with full errors
and saved for the TUI to display. Recover returns the persistence result in that
case. Control.Run then blocks extraction until failures are resolved explicitly.
The old test incorrectly interpreted a nil Recover error as accepting the symlink.

The replacement checks all three persisted failed statuses, the failed count and
diagnostics at batch/ISO/title level. It proves that the mocked engine is never
called, the symlink remains intact, and the target directory and its canary file
remain untouched. A second test exercises the same failure policy with an existing
directory at the final filename, without requiring symlink privileges.

Changed-source and output-symlink cases are separate subtests. Missing symlink
privileges produce an explicit test skip instead of the old silent return. No test
is disabled in CI, and no continue-on-error or weaker workflow gate was added.

## Verification

Host: Windows amd64, Go 1.26.10, CGO disabled.

- Reproduced all three path-string failures before the fix by placing TMP/TEMP
  behind a Windows directory junction. The same targeted tests passed afterward.
- Ran the entire Go suite with that aliased temporary directory: 98 top-level
  tests passed, no failures. Two top-level tests were explicitly skipped: optional
  screenshot capture and the new directory-symlink test. The output-symlink
  subtest also reported its unavailable Windows privilege explicitly.
- go vet and gofmt checks passed; all 15 Python tooling tests passed.
- Cross-compilation passed for FreeBSD/Linux/Windows amd64 and macOS amd64/arm64
  with CGO_ENABLED=0. All three workflows passed actionlint 1.7.12.
- Native Linux/macOS execution and privileged symlink cases still require the
  next hosted CI run. No native Unix or race-test success is inferred from these
  cross-builds; this host has no C compiler for the race detector.

Only tests and documentation changed. Batch JSON schema, extraction, publication,
FUSE staging preservation, recovery, preferences and both stop modes are unchanged.
No commits, pushes, tags or releases were created during this fix.

## Applying the follow-up patch

Use the small makemkv-tui-ci-test-fix.patch in your existing clean Git checkout.
It is based on the failing commit above and does not reapply the earlier large
release-preparation patch. First use git apply --check, inspect the diff, then
commit and push main. A new main CI run must pass before making a new release tag.
Rerunning an old tag still tests that tag's old source; do not force-replace it.
