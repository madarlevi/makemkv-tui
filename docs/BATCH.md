# Persistent batch execution and recovery

## Commands

From the existing source directory, build normally and run:

```sh
makemkv-tui batch "/media/isos" --output "/media/output"
makemkv-tui batch "/media/isos" --output "/media/output" --state "/plans/movies.json"
makemkv-tui batch --state-dir
makemkv-tui batch --list
makemkv-tui batch --plans
makemkv-tui batch --resume "/path/printed/at/startup.json"
makemkv-tui batch --resume "/plans/movies.json" --retry-failed
```

Options may appear before or after the input directory. A new batch requires one
input root and an output root. Default plans live in
`$XDG_STATE_HOME/makemkv-tui/batches/`, or
`$HOME/.local/state/makemkv-tui/batches/` if XDG_STATE_HOME is unset. FreeBSD root
normally uses `/root/.local/state/makemkv-tui/batches/`. XDG_STATE_HOME must be
absolute. The home-directory fallback uses HOME (or the platform home API if
unset). New directories use 0700, JSON and lock files 0600. A descriptive source
name plus the batch's persistent random 128-bit ID forms its JSON filename.
Neither input nor output receives a default state file. `--state PATH` takes
precedence and does not require a working home/XDG configuration.

The actual canonical state path is printed before scans/extraction. `--state-dir`
prints the default directory, `--list` lists its JSON paths and statuses, and
`--plans` opens a keyboard chooser (Up/Down or J/K, Enter to resume, Esc to cancel).
An unfinished plan with the same canonical source/output roots in the destination
state directory blocks a new plan and prints its resume command. Use `--new` to
explicitly create a separate plan. A directory catalog lock serializes this
check/publication across instances. Invalid plans are reported, never reset.
Arbitrary plans outside that directory are resumed by explicit path; they are
not searched for across the source/output trees.

Logs are appended to STATE.log. Use --config for frontend preferences and
--profile for an existing MakeMKV profile when creating a plan. Resume uses the
saved roots, preferences and profile; it rejects conflicting new-plan paths.

The state file is created before any scan. If the engine is unavailable, install
it and resume that file. Existing state files are never replaced by a new batch.
Legacy plans (including `.batch-state.json` inside an ISO source directory or
`batch-state.json` in an output directory) remain compatible: pass their exact
path to `--resume`. This does not migrate the plan or change saved source/output
paths, metadata, title selections or settings. To explicitly migrate an inactive
plan, copy its JSON to a local state path and resume that copy; do not run both
copies concurrently because per-path locks cannot coordinate copied plans.

## Planning: select once

Discovery recursively finds regular files with case-insensitive .iso extensions,
sorts their relative paths lexically, and ignores file/directory symlinks. It does
not follow symlink loops. Unreadable discovery directories produce an error.
Case-folded output-directory collisions are rejected, including two sibling
files named movie.iso and movie.ISO.

The existing title-selection screen is reused. Video/audio/subtitle metadata,
duration, size and chapter counts remain available. Each scan's metadata and each
selection change are persisted; returning to an ISO uses that saved information.
The shared explorer colors VIDEO red, AUDIO green and SUB blue with a text legend.
Left/Right and Tab fold titles/streams, Up/Down visits visible rows, and [ / ] scrolls
the lower metadata panel. Enter/PgDn still reviews and advances from any row.
Source streams are read-only; Space changes selection only on title rows.
NO_COLOR disables styling without hiding type labels or selection/focus markers.
Unknown attribute values and optional MakeMKV message codes remain in the saved
metadata; older plans without codes still work using their reported stream type
text. This is an additive metadata extension; existing selections and execution
state are retained. Raw robot output is available with Ctrl+L or STATE.log, never
in the normal title explorer. HDR/video bit depth and chapter timestamps are not
invented when the robot scan does not expose them.

| Key | Planning action |
|---|---|
| Space | Toggle the focused title |
| A / D | Select all / deselect all titles |
| Right / Left | Expand / collapse a title or stream |
| [ / ] | Scroll metadata |
| Enter / PgDn | Mark this ISO reviewed and move forward |
| PgUp | Move backward, preserving selections |
| X | Skip this ISO, including cancelling a current info scan |
| F | Open the summary |
| Ctrl+P | Existing frontend preferences editor |
| Ctrl+L | Batch logs |
| Ctrl+C / q | Save and exit planning |

An ISO with no selected titles is skipped when reviewed. A failed info scan can be
retried with R or skipped with X. Skipping an info scan cancels only that scan,
not the batch controller.

The summary includes selected/skipped files, actual title IDs, each expected
output path, and estimated selected-title bytes (unknown estimates are counted).
Up/Down focuses an ISO; E edits its saved selections. Enter opens a final
confirmation and **Y** is the only planning action that starts extraction.
The executor independently refuses to run while any ISO is unreviewed. Filenames
in batch mode are fixed; the single-source filename/template features are unchanged.

Once execution starts, selections are locked so completed-title records cannot
accidentally be discarded. Resume never reopens the selection wizard for a fully
reviewed plan. If it was previously confirmed, --resume automatically continues.
A ready plan that was never confirmed returns to its summary for explicit approval.

## Paths and stable title IDs

For a source alma/körte.ISO and selected title ID 7, the destination is:

```text
OUTPUT/alma/körte/title7.mkv
```

Different parent directories keep identical ISO basenames separate. Arguments are
passed without a shell, including spaces, Unicode, quotes, ampersands and dollar
signs. Paths below the saved roots must remain local and symlink-free.

Batch info and mkv calls both explicitly set --minlength=0. This avoids title ID
renumbering when MakeMKV's global minimum-length preference changes. Single-source
behavior retains its existing engine preference defaults.

Before extracting an unfinished ISO, a metadata-only scan checks the selected,
unfinished IDs against planned duration, byte size, source filename and segment
attributes. Missing or changed mappings fail safely; restore the planned engine
and settings rather than extracting another title under an old ID. This check
does not repeat the selection wizard and does not scan completed-only ISOs.

Source paths, size and nanosecond modification time are checked on resume and
before each title. Profiles are snapshotted with SHA-256. Changed/missing sources
or profiles require restoration; roots are not silently relocated. Source ISO
contents are not fully hashed, so changes that preserve both size and mtime are
not detected by the source fingerprint. Completed MKVs are fully hashed.

## Execution and stop modes

Jobs and selected titles execute sequentially in deterministic path/ID order.
The dashboard shows ISO/title positions, process and title progress, persisted
overall progress, completed/pending/interrupted/failed counts, operation, elapsed
execution time, recent logs/errors and the stored stop request.

Overall progress is (completed selected titles + current title fraction) /
selected titles. Titles have equal weight; it is not a byte-weighted estimate.
Completed counts come from saved state, not the planning screen's ISO position.

| Key | Running action |
|---|---|
| Ctrl+C | Stop immediately; stop scheduling and cancel the active subprocess |
| Ctrl+C again | Force termination rather than waiting for the grace interval |
| S | Finish the current ISO, then pause before starting another ISO |
| Ctrl+L | View recent batch logs; PgUp/PgDown scroll; Esc closes |
| Ctrl+E | Expand full errors; Up/Down or PgUp/PgDown scroll; Esc closes |

Immediate stop first signals cancellation, independently of JSON disk I/O. The
request is persisted, the active extraction becomes interrupted, completed titles
stay completed, and the batch becomes paused. On FreeBSD/Linux/macOS the runner
sends SIGTERM to the process group and escalates to SIGKILL after two seconds;
a second Ctrl+C requests force immediately. Surviving group members are handled
before the runner returns. Other platforms use their process-kill fallback.

S does not cancel the process. It persists after_iso and stopping, finishes every
remaining selected title in the current ISO, saves each completion, then exits
paused. If that finishes the entire plan, the final state is completed.
An ordinary resume consumes a stop request whose ISO boundary was already reached.
If a crash interrupted a graceful stop before that boundary, recovery honors the
remaining request, finishes that ISO and pauses.

SIGINT, SIGTERM and SIGHUP take the immediate-stop path. A terminal renderer failure
also cancels the worker and waits before releasing the state lock. SIGKILL and
power loss cannot run shutdown handlers; stale running records and isolated
attempt directories are reconciled at the next resume. A detached engine after
an uncatchable frontend kill has no authority to publish a final output: only the
frontend's durable commit path does that.

## Failure policy and retries

The policy is **fail fast**: the first scan-preflight or extraction failure stops
the execution queue, records errors, and marks affected unfinished work failed.
Completed titles stay intact. A pending graceful-stop request does not turn an
error into success; the batch is failed, not falsely completed or safely paused.

Fix the cause, then use --retry-failed or R in the stopped dashboard/summary.
Retries preserve selections, completed-file records and error history. Attempts
increment when extraction starts; retries are attempts minus the initial attempt.
ISO attempts count preflight/execution episodes, so resuming an unfinished ISO
increments its ISO retry counter independently of individual title retries.
Interrupted titles restart automatically; failed titles need an explicit retry.

A state-write failure cancels processing and stops scheduling. Recovery uses the
last valid checkpoint. No title is published before its success/verification
checkpoint is durable.

## Durable commit protocol

Each extraction has a unique attempt directory:

```text
OUTPUT/.makemkv-batch/BATCH_ID/ISO_INDEX-TITLE_ID-ATTEMPT_ID/
```

Batch extraction uses this private attempt directly. The engine never writes to
a final title pathname. The old nested single-source staging path is no longer
used by batch extraction, preventing outer cleanup from deleting a successfully
extracted inner file after publication failure.

1. Save running status, attempt ID, current ISO/title and counters; create and sync
   the private staging directory before starting the engine.
2. Require process success and MSG:5005 acknowledging one saved title. Save an
   engine-success receipt in the attempt, including actual engine filename.
3. Save the engine-success checkpoint, validate regular/nonempty/EBML output,
   hash all bytes with SHA256, sync file/directory and save verification in JSON.
4. Publish without overwriting. Linux/macOS use native no-replace rename when
   available. FreeBSD/Windows use the safe-copy fallback described below.
5. Validate the final output against the saved verification, persist completed
   status, then clean only that completed attempt. Failed completion saves retain
   the attempt and allow reconciliation with the final file on resume.

No production MKV publishing uses os.Link/linkat. An ordinary rename after merely
checking absence is not safe against a creation race and is never used here.
Native unsupported-operation or cross-device errors permit the fallback; actual
I/O or permission failures retain their full error and stop rather than being
misreported as lack of support. The affected FreeBSD FUSE mount has not been
reproduced locally; working ln and mv alone cannot identify the original EIO cause.

### Exclusive-copy fallback

The fallback first copies to a unique .publish-*.tmp file within the private
attempt, syncs/closes it and verifies its full hash. A complete matching prepared
copy may be reused on retry. It then creates the final pathname with O_EXCL,
copies the prepared bytes, syncs/closes, validates the final hash, and syncs parent
directories where supported. Filesystem errors are kept in full in JSON and in
Ctrl+E's scrollable error view. No existing destination is overwritten, including
one created between the initial check and final publication.

This is **not an atomic publication**: the final name is visible during copying.
A crash can leave a partial final file. Such a file is never accepted as complete
or automatically overwritten/removed, because recovery cannot safely prove it
was not replaced by an external writer. Deliberately move it aside, keep the
original staging and JSON, then retry. The verified staging can be published
without another engine extraction. Allow up to three current-title copies:
original engine file, prepared temporary copy and final copy. Incomplete copies
and interrupted unacknowledged attempts are retained in isolation, not aggressively
removed. Operators may reclaim them after confirming which plan/attempt owns them.

Engine success plus a nonempty EBML signature and full-file size/hash is a
practical completion/integrity check, not full Matroska decoding or playback proof.
Filesystem/device flush semantics ultimately determine physical durability.

## Recovery decisions

| Persisted/disk condition | Recovery |
|---|---|
| completed + matching final size/hash | Keep completed; never extract again |
| verified checkpoint + matching final | Adopt completed and save before cleanup |
| engine receipt/checkpoint + complete stage | Validate, save verification and publish without reripping |
| verified checkpoint + stage, final absent | Retry publication without reripping |
| stale running without engine-success proof | Mark interrupted; retain old attempt, restart in a new attempt |
| interrupted extraction without success proof | Restart from zero; do not adopt partial MKV |
| final exists without matching verification | Record collision; never overwrite or blindly adopt |
| completed file missing/changed | Record failure; restore or explicitly retry |
| malformed/incompatible JSON | Actionable error; never silently reset |

A receipt is accepted only for the planned source/title/filename and successful
engine acknowledgement. Truncated receipt files alone are not proof of success;
recovery preserves them and reports the ambiguity. Transient filesystem failures
must not clear a valid engine-success checkpoint. An explicit failed-title retry
keeps valid staging proof. Failed/invalid staging remains isolated and is not
mistaken for successful output.

## JSON state and locking

internal/batchstate owns schema version 1, validation, atomic storage and locking.
It is separate from internal/batch, which owns planning, recovery and scheduling.

The JSON includes roots, discovered paths/source fingerprints, metadata, per-ISO
preferences, selections, planned outputs, current ISO/title, statuses, verification,
attempt/retry counters, errors, stop requests, progress and UTC timestamps.
All required batch and item statuses are represented.

Every meaningful transition writes a same-directory temporary file, syncs it,
atomically renames it over the old JSON, and syncs the parent directory where
supported. Temporary-file close errors and short writes are failures. Both
initial publication and replacement use `os.Rename`, never a state-file hard link
or a truncate/overwrite fallback. Initialization refuses an existing destination
under the exclusive state lock; external tools must respect that lock too.
Failed/truncated temporary
writes cannot destroy the previous plan. Unknown schema versions, invalid paths,
inconsistent selections, invalid checkpoints and unknown JSON fields are rejected.
If directory sync fails after rename, the error explicitly says the new JSON is
published but durability is unconfirmed; execution stops. Pre-rename failures
retain the previous JSON, and temporary files are cleaned up with errors reported.

### FreeBSD FUSE `linkat` diagnostic

The previous initialization branch called `os.Root.Link(temp, destination)`.
Go's Unix implementation resolves both parent directory descriptors and invokes
`linkat(oldDirFD, tempBasename, newDirFD, stateBasename, 0)`. It wraps an error
with operation name `linkat`; later saves previously used `Root.Rename` instead.
Thus the reported failure identifies initial hard-link publication, not a failed
JSON write or an ordinary state replacement rename. An existing destination
would normally produce EEXIST; the reported EIO came from that linkat operation.

Successful ordinary `ln` and `mv -f` tests do not establish the cause of EIO for
that descriptor-relative call. No general lack of hard-link/rename support is
inferred. Identifying the driver/kernel cause requires tracing on the affected
FreeBSD mount (for example `truss -f -o /tmp/makemkv.trace` before the old command),
comparing the syscall, directory descriptors, relative paths and flags against
the successful `ln` call. This workspace cannot reproduce that mount. State
publication now uses pathname-based `os.Rename` in the canonical state directory,
and new default plans are local. Explicit legacy locations use the same rename
implementation; a failure is reported without deleting the previous plan.
Output MKV publication uses the separate no-replace/copy protocol above.

A stable STATE.lock file is held throughout load/recovery/planning/execution.
FreeBSD/Linux/macOS use nonblocking flock; Windows uses LockFileEx. The OS releases
the lock when a process exits or the machine restarts. Diagnostic owner metadata
left behind is detected and recovered only after the OS lock is successfully
reacquired. PID reuse is not used to guess ownership. Do not delete a live lock
file: the lock is deliberately tied to its stable inode.

Batch logs are appended to STATE.log. Ctrl+L shows the latest 500 in-memory lines;
the disk log retains the whole execution history. No additional service, GUI or
desktop session is needed.

## Tests and deployment validation

The automated suite covers discovery/order/Unicode, parent hierarchy and repeated
basenames, selection persistence, the planning start barrier, skipping scans,
atomic failure handling, corrupt/schema validation, process locking and stale
lock recovery after real helper-process exit, sequential extraction, both stops,
disconnect cancellation, interrupted/stale recovery, prepared/finalized commit
windows, hash mismatches, partial cleanup, collisions, failed retries, persistence
failure and changed title identities. Engine processing is mocked.

The Unix subprocess tests additionally exercise SIGTERM escalation and a second
force request. Native Windows tests, vet and target cross-compilation can run in
this workspace; Unix runtime/race tests are configured in CI and still need to
run on actual target systems. Native FreeBSD media and SSH integration remain
unverified here. The frontend's MIT license does not change MakeMKV engine terms.


