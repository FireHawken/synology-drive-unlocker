# synology-drive-unlocker

> Reroute existing Synology Drive sync sessions to system or dot-prefixed local folders that the official client refuses to pick directly.

[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

> [!CAUTION]
> **Use at your own risk.** This utility writes directly to the Synology Drive Client's local databases — undocumented internals that the vendor can change, break, or actively block at any release. The author makes **no warranty** and accepts **no liability** for any data loss, sync corruption, account issues, or other damage caused by using this tool. If a future Synology Drive update changes the schema or detection logic, this tool may silently corrupt your data. **You alone are responsible for the consequences of running it.** Make sure you have independent backups of any data you cannot afford to lose before using this tool, full stop.

`synology-drive-unlocker` is a small, single-binary terminal utility that helps you work around a long-standing limitation of the **Synology Drive Client**: in its sync-task creation dialog you cannot pick OS-managed folders (e.g. `C:\Users\you\AppData\…`) or folders whose name starts with a dot (e.g. `~/.config`, `~/.ssh`). You can, however, pick a *subfolder* of one — and the daemon itself happily syncs anything you point it at after task creation.

This tool flips the bit the GUI refuses to flip: it edits the client's local SQLite databases (with backups, while the client is stopped) so that an existing session points at the folder you actually wanted.

The workaround itself isn't novel — Synology users have been swapping paths in `sys.sqlite` by hand for years, e.g. in [this community thread](https://community.synology.com/enu/forum/1/post/152457). `drive-unlocker` just packages it into something you don't have to do with a hex editor and a prayer.

---

## What it does

1. Detects your Synology Drive Client database directory:
   `%LOCALAPPDATA%\SynologyDrive\data\db` on Windows, and either
   `~/.SynologyDrive/data/db` or
   `~/Library/Application Support/SynologyDrive/data/db` on macOS.
2. Refuses to do anything if the client is running or a previous session left
   `*.sqlite-wal` / `*.sqlite-shm` files behind.
3. Lists your existing sync sessions, lets you pick one with the arrow keys.
4. Lets you pick (or paste) the new local folder.
5. Shows a clear diff of what will change.
6. Snapshots **all four** database files (`sys.sqlite`, `file-status.sqlite`,
   `filter.sqlite`, `history.sqlite`) into a timestamped backup directory.
7. Atomically rewrites the relevant rows in `sys.sqlite` and (if its schema is
   present) `file-status.sqlite`, then runs `PRAGMA wal_checkpoint(TRUNCATE)`
   so no journal artefacts linger.
8. Lets you roll back any change from the same TUI by picking a backup.

It only edits two databases. `filter.sqlite` and `history.sqlite` are
backed up but never modified — the only side effect is that `history.sqlite`
will contain stale path references for entries created before the swap. Those
are cosmetic and have no bearing on sync correctness.

## Why use it

- **Sync `~/.ssh`, `~/.config`, `AppData`, or any system folder** that the
  client's folder picker greys out.
- **Reuse existing sync tasks** without recreating them and re-uploading
  everything.
- **Reversible** — every run snapshots all four databases first, and the same
  TUI can restore any of them.

## How it works (one-screen technical summary)

The Synology Drive Client tracks sync tasks in `sys.sqlite`. The relevant
columns are:

| File              | Table           | Column        | Notes                                          |
|-------------------|-----------------|---------------|------------------------------------------------|
| `sys.sqlite`      | `session_table` | `sync_folder` | Local folder per task. Trailing `\` on Win.    |
| `sys.sqlite`      | `system_table`  | `open_folder` | Tray-icon "Open" target. May or may not match. |
| `file-status.sqlite` | `statinfo`   | `path`        | Path cache. May not exist for fresh tasks.     |

`drive-unlocker` updates `sync_folder` in a transaction, automatically updates
`open_folder` if (and only if) it pointed at the session you're editing, and
defensively rewrites `statinfo.path` rows when the table exists.

# Safety

> [!WARNING]
> **No warranty, no liability — you are on your own.** This tool edits an undocumented database belonging to a closed-source application. Synology can change the schema, rename files, or add detection that breaks or rejects modified state — at any point, without notice, possibly silently. When that happens, this utility may misbehave and **damage or destroy your synced data**. The safeguards listed below reduce risk; they do not eliminate it. The author bears **zero responsibility** for any data loss, corruption, or downstream consequences. **You run this software at your own risk.** If the data on either side of the sync matters, take an independent backup *before* you run the tool — not after.

The mechanical safeguards the tool does provide:

- The client process is checked via `tasklist` before any write.
- WAL/SHM presence aborts with an error — they imply an unclean shutdown
  (the client must be properly closed, not just killed).
- Backups are created **before** any database is opened for writing. They live
  in `<dbDir>/.unlocker-backups/backup-<ISO timestamp>/` alongside a
  `meta.json` describing the change.
- Updates run inside a single SQLite transaction with an integrity check
  (the existing `sync_folder` value must match what we read pre-edit, or we
  bail out without writing).
- Path collisions (the new folder equals, contains, or is contained by another
  session's `sync_folder`) are detected and block the apply step.

These checks protect against the *predictable* failure modes. They cannot
protect against the unpredictable ones — schema drift in a future Synology
release, an undocumented column we don't know about, a subtle quirk of your
specific setup. **Independent off-tool backups are non-negotiable.**

# Installation

## From a release

Download the matching asset from the [Releases](https://github.com/FireHawken/synology-drive-unlocker/releases) page and run it from any terminal.

- Windows: `drive-unlocker.exe`
- Apple Silicon Mac: `synology-drive-unlocker_<version>_darwin_arm64.tar.gz`
- Intel Mac: `synology-drive-unlocker_<version>_darwin_amd64.tar.gz`

## From source

Requires Go 1.25 or newer.

```sh
git clone https://github.com/FireHawken/synology-drive-unlocker.git
cd synology-drive-unlocker
go build -ldflags="-s -w" -trimpath -o synology-drive-unlocker .
```

The resulting binary is around 8 MB. If you want it smaller and don't mind
the occasional false-positive antivirus flag, run it through UPX:

```sh
upx --best --lzma synology-drive-unlocker.exe   # ~3 MB
```

# Usage

> [!IMPORTANT]
> **Read before running.** This tool may corrupt or destroy your synced data — see the warnings at the top of this file and in the [Safety](#safety) section. The author is not liable for any consequences. Synology may change or block the database mechanism this tool depends on at any time, which can lead to data damage. **Take an independent backup of the source folder you intend to sync before proceeding, and do not run this if you cannot afford to lose that data.** Continuing means you accept the risk yourself.

> **Stop Synology Drive Client first.** On Windows, right-click the tray icon
> and choose *Quit*. On macOS, quit Synology Drive from the menu bar icon.
> Don't kill the process unless you have to - it must finalise its WAL files.

```sh
# Windows
.\drive-unlocker.exe

# macOS
./synology-drive-unlocker
```

You'll see the main menu with a preflight banner:

```
Synology Drive Folder Unlocker

╭─────────────────────────────────────────────────────────────────────╮
│ ✓  Synology Drive database directory  C:\Users\you\…\SynologyDrive  │
│ ✓  Synology Drive Client stopped       cloud-drive-ui.exe           │
│ ✓  no WAL/SHM files in db directory                                 │
╰─────────────────────────────────────────────────────────────────────╯

▸ Change a sync session's local folder
  Restore from backup
  Quit
```

Pick **Change a sync session's local folder**, choose the session, pick the
new local folder (`↑↓` to navigate, `→`/`Enter` to descend, `.` to choose
the current directory, `i` to type/paste a path), review the diff, hit
**Apply**.

That's it — restart the Synology Drive Client and your session will sync
the new folder.

## Restoring

Pick **Restore from backup** in the main menu. The list is newest-first and
shows the change each backup describes (session ID, old → new path, time).
Confirm and the four databases are copied back over the live ones.

# Platform support

| OS      | Status                                                                |
|---------|-----------------------------------------------------------------------|
| Windows | Supported. Tested on Windows 11.                                      |
| macOS   | Supported, but needs more real-world testing across client versions. |
| Linux   | Stub only. Database paths and process detection are not implemented. |

macOS detection checks both known Synology layouts:
`~/.SynologyDrive/data/db` and
`~/Library/Application Support/SynologyDrive/data/db`. Process preflight checks
for `SynologyDrive`, `Synology Drive Client`, `cloud-drive-daemon`, and
`cloud-drive-ui`.

Adding Linux support is a matter of filling in
`internal/platform/platform_linux.go` and validating the process names used by
the Linux client - pull requests welcome.

# Project layout

```
synology-drive-unlocker/
├── main.go                      # CLI entrypoint
├── internal/
│   ├── paths/                   # Synology-style path normalisation + collision checks
│   ├── db/
│   │   ├── sys.go               # session_table + system_table.open_folder
│   │   └── filestatus.go        # statinfo (defensive — schema not always present)
│   ├── backup/                  # snapshots, meta.json, restore
│   ├── platform/                # OS-specific paths + WAL detection
│   ├── process/                 # IsRunning(name) — Windows uses tasklist
    └── tui/                     # Bubble Tea screens (menu, sessions, picker, confirm, restore)
```

# Testing

```sh
go test ./...
```

The test suite includes an end-to-end check (`internal/tui/apply_test.go`)
that builds a synthetic `sys.sqlite` fixture (via `db.MakeSysFixture`) plus
empty companion databases in a tempdir, runs the full backup-and-update flow
against them, and asserts both the on-disk result and that a subsequent
restore rolls everything back. No real-world Synology Drive databases are
needed — fixture schemas are reproduced from observation in
[internal/db/testdata.go](internal/db/testdata.go).

# Built with

- [charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea) — TUI framework
- [charmbracelet/bubbles](https://github.com/charmbracelet/bubbles) — list / textinput / spinner widgets
- [charmbracelet/lipgloss](https://github.com/charmbracelet/lipgloss) — styling
- [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) — pure-Go SQLite, so the binary cross-compiles without CGO

# Contributing

Issues and PRs are welcome. Please:

- Run `gofmt -w .` and `go vet ./...` before opening a PR.
- Add or update a test in the package you touched. If you need a populated
  `sys.sqlite`, use `db.MakeSysFixture` with a custom slice of
  `db.FixtureSession` rather than checking real database files in.
- Keep user-facing strings in English; localisation isn't on the roadmap.

# License

[MIT](LICENSE). The licence's "AS IS" / no-warranty clauses are load-bearing here — see the warnings at the top of this file.
