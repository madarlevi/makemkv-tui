# Engine integration findings and parity plan

The product goal is full Windows MakeMKV GUI parity through a keyboard interface.
This release is an incremental CLI-backed implementation, not a claim of complete parity.

## What was inspected

Inspected the official MakeMKV 2.0.0 source archive on 2026-10-18:

- [Official Linux source announcement](https://forum.makemkv.com/forum/viewtopic.php?f=3&t=224)
- [Source archive](https://www.makemkv.com/download/makemkv-oss-2.0.0.tar.gz)
- Observed archive SHA-256: `435316b2d219eb48c880526557addd076b5f5e6de5171424c7651f9cac95b161`.
  This records the inspected artifact; it is not an independently authenticated signature.
- [Robot CLI documentation](https://www.makemkv.com/developers/usage.txt)

No MakeMKV implementation code or engine binaries are distributed with this frontend.
The descriptions below summarize source behavior. The source archive itself includes
different licensing notices for different files; its availability does not make all
components open source.

## How the GUI controls track selection

In the inspected archive, paths below are relative to `makemkv-oss-2.0.0/`:

1. `makemkvgui/src/uisync.cpp:76`: checkbox changes call the item's
   `set_Enabled` method. Expansion is also synchronized with engine item state.
2. `makemkvgui/src/margui.cpp:118`: `AP_UiItem::set_Flag` reads and updates
   item flags, passes the low/high words of its 64-bit handle and the flags to
   `apCallSetUiItemState`. Enabled is flag bit 0; expanded is bit 1.
3. `makemkvgui/src/margui.cpp:271`: writable item attributes use
   `apCallSetUiItemInfo`. This is the route relevant to names and editable
   metadata, subject to each item's writable attributes.
4. `makemkvgui/src/margui.cpp:82`: extraction is requested through
   `apCallSaveAllSelectedTitlesToMkv`.
5. `makemkvgui/inc/lgpl/aproxy.h` defines these calls and item handles, with ABI
   identifier `A0001`. `client.cpp` checks the engine/client ABI handshake.
6. `makemkvgui/src/main.cpp` selects shared-memory transport by default and a
   standard-pipe transport with the GUI's `-std` option. That GUI option is not
   exposed here as a fabricated robot CLI selection argument.
7. `clt_pipe.cpp` packs a command word, argument counts and payload sizes, with
   little-endian framing and a compact special case for some messages.
8. `margui.cpp:339` and `uisync.cpp:757` show distinct chapter handles and
   chapter objects in the GUI interface, beyond the title chapter count in the
   robot scan.

This establishes a concrete internal RPC route, not a stable public Go API or
installed-engine compatibility guarantee. No Windows GUI binary was instrumented;
the findings come from the official cross-platform GUI source.

A future RPC adapter should verify the handshake, bounds-check framing, implement
callbacks and job cancellation, expose item writable/state capabilities and run
against real FreeBSD/Linux/macOS engines. It must reject mismatched engines safely.
This increment does not send untested internal messages to an engine.

## Profiles: supported route with limits

The MakeMKV author's [track selection response](https://forum.makemkv.com/forum/viewtopic.php?t=6884)
documents passing a profile through the console. The author's
[selection-rule description](https://forum.makemkv.com/forum/viewtopic.php?t=4386)
describes language/type predicates and ordering rules.

This frontend accepts an existing profile using `--profile path`, validates it as a
regular file, and forwards it as one `--profile=<path>` argument. It does not
generate undocumented switches or alter MakeMKV's global settings.

Profiles are not treated as a substitute for arbitrary per-title stream handles.
The [profile example/discussion](https://forum.makemkv.com/forum/viewtopic.php?t=18366)
and [version-related selection discussion](https://forum.makemkv.com/forum/viewtopic.php?t=18951)
show that preference/profile selection behavior has changed. A supplied profile
must be tested with the installed engine. Merely accepting the flag does not verify
that a desired track was included.

## Implemented alternative: identify, review, remux

After extraction, press r on a completed file:

- `mkvmerge -J <actual-output.mkv>` provides real output track IDs and metadata.
- The TUI allows individual audio/subtitle selection and applies stored language
  preferences as initial choices. Video remains included.
- The confirmation screen lists keep/exclude choices before processing.
- `--audio-tracks`, `--subtitle-tracks`, `--no-audio` and `--no-subtitles`
  target that actual MKV; original MakeMKV SINFO IDs are never reused as MKV IDs.
- Remuxing writes a new staged file and publishes it without replacing a destination.
  The original remains intact. Chapters and attachments follow mkvmerge's normal
  preservation behavior. Exit code 1 is reported as completed with warnings.

This is stream-copy remuxing, not disc processing or decryption.
It needs additional disk space and cannot recover tracks MakeMKV omitted.
It is therefore a useful implemented alternative, not full source-track parity.

Reference: [mkvmerge manual](https://mkvtoolnix.download/doc/mkvmerge.html),
including identification, track IDs, track selection and exit-code semantics.

## Capability matrix

| Product capability | Current state | Remaining work |
|---|---|---|
| ISO, DVD, Blu-ray, BDMV, optical devices | Path browser, typed sources, drive discovery | Native media/device validation |
| Title selection | Implemented with sparse engine IDs | RPC adapter cross-check |
| Expand/collapse titles and streams | Implemented locally | Derived/core stream parent relationships |
| Audio/subtitle source selection | Unavailable in robot backend | RPC adapter and engine tests |
| Audio/subtitle selection in output | Implemented via optional MKVToolNix | Native integration/media tests |
| Language preferences | Saved; applied to output-track review | Direct engine rule application after verification |
| Codecs/languages/duration/size | All reported attributes retained and named | Additional engine-specific metadata |
| Chapters | Reported count; remux count; preserved in remux | Individual names/timestamps and editing through RPC |
| Output folder and filenames | Implemented through staged files and no-replace publishing | Direct writable engine attributes where supported |
| Progress, warnings, errors, logs | Implemented with saved session logs | RPC prompts and richer job states |
| Preferences | Versioned JSON; atomic saves | Broader GUI settings as backend capabilities expand |
| SSH/headless | Terminal frontend; no display/Qt runtime dependency | Native SSH terminal smoke tests |
| Full GUI behavior | Goal, not achieved | Disc backup, eject/control, expert editing, profile management and further GUI workflows |

## Increment order and acceptance gates

1. **This increment:** engine boundary, hierarchy, preferences, source browsing,
   output safety, optional track review/remux, documentation and tests.
2. **RPC feasibility:** a separate adapter behind `backend.Engine`, read-only
   handshake/scan on FreeBSD amd64 first. No TUI-specific transport logic.
3. **RPC selection:** enumerate real item handles, toggle enabled state, inspect
   chapter objects and editable names, extract a controlled fixture disc.
   Confirm output track identities and cancellation before enabling UI controls.
4. **Broader parity:** chapter/metadata editing, derived stream relationships,
   profile/settings management, backup and optical-drive controls. Inventory
   other Windows GUI actions explicitly before claiming parity.
5. **Platform qualification:** native FreeBSD amd64, then Linux and macOS,
   including SSH, missing/expired engine licenses, unreadable media, output-full,
   interrupted jobs, duplicate names and engine ABI mismatches.

The MakeMKV engine remains a separate dependency with separate terms. A free,
open-source frontend does not make that engine permanently free.
