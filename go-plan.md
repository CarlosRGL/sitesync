# sitesync — Go + Bubble Tea TUI Rewrite Plan

## Summary

Rewrite the `sitesync` bash+PHP tool as a Go application with a full Bubble Tea TUI. Preserve all functionality (7-step sync workflow, hook system, PHP serialize-aware find/replace) while migrating config to TOML and delivering a polished interactive interface.

**Choices**: Go · Bubble Tea/Lip Gloss · Huh (forms) · TOML config · resilient_replace ported to Go (no PHP dep)

---

## 1. Go Module Structure

```
sitesync/
├── cmd/
│   └── sitesync/
│       └── main.go                # Entry point, Cobra CLI wiring
├── internal/
│   ├── config/
│   │   ├── config.go              # All TOML struct definitions
│   │   ├── loader.go              # ListConfigs(), Load(name), Save(name, cfg)
│   │   └── migrate.go             # Shell config → TOML one-time converter
│   ├── sync/
│   │   ├── engine.go              # 7-step orchestrator, goroutine runner
│   │   ├── database.go            # Steps 1 (dump) and 4 (import)
│   │   ├── replace.go             # PHP serialize()-aware find/replace (Go port)
│   │   ├── replace_test.go        # Unit tests for replace logic
│   │   ├── hooks.go               # Steps 3, 5, 7 - hook script execution
│   │   ├── files.go               # Step 6 - rsync / lftp subprocess
│   │   └── events.go              # Event/message types for engine→TUI channel
│   ├── tui/
│   │   ├── app.go                 # Root Bubble Tea model, screen router
│   │   ├── models/
│   │   │   ├── picker.go          # Screen 1: site list picker
│   │   │   ├── opselect.go        # Screen 2: sql/files/both selector
│   │   │   ├── syncing.go         # Screen 3: live 7-step progress + log
│   │   │   └── editor.go          # Screen 4: config wizard (new/edit site)
│   │   └── styles/
│   │       └── styles.go          # Centralised Lip Gloss palette + borders
│   └── logger/
│       └── logger.go              # Thread-safe append-only log/sitesync.log
├── go.mod
├── go.sum
├── sample/
│   └── config.toml                # Annotated reference TOML
├── etc/                           # User configs (unchanged location)
├── log/
└── tmp/
```

---

## 2. TOML Config Schema

Full field mapping from old shell variables:

```toml
[site]
name        = "mysite"
description = ""

[source]
server           = "www.example.com"   # src_server
user             = "username"           # src_user
port             = 22                   # src_port
type             = "remote_base"        # src_type: remote_base|local_base|remote_file|local_file
file             = ""                   # src_file
compress         = true
db_hostname      = "localhost"          # src_dbhostname
db_port          = ""
db_name          = ""                   # src_dbname
db_user          = ""                   # src_dbuser
db_password      = ""                   # src_dbpass
site_protocol    = "http://"
site_host        = "www.example.com"
site_slug        = ""
files_root       = "/remote/path/www"
path_to_mysqldump = "mysqldump"
remote_nice      = "ionice -c3 nice"

[destination]
site_protocol    = "http://"
site_host        = "local.example.com"
site_slug        = ""
files_root       = "/home/user/www"
db_hostname      = "localhost"
db_port          = ""
db_name          = ""
db_user          = ""
db_password      = ""
path_to_mysql    = "mysql"
path_to_mysqldump = "mysqldump"
path_to_rsync    = "rsync"
path_to_lftp     = "lftp"
local_nice       = "ionice -c3 nice"

[database]
sql_options_structure = "--default-character-set=utf8"
sql_options_extra     = "--routines --skip-triggers"
ignore_tables         = []

[[replace]]
search  = "http://www.example.com"
replace = "http://local.example.com"

[[sync]]
src = "/remote/path/www"
dst = "/home/user/www"

[transport]
type          = "rsync"
rsync_options = "-uvrpztl"
exclude       = ["/sitesync/", ".git/", ".DS_Store"]

[transport.lftp]
password        = ""
port            = 21
connect_options = ""
mirror_options  = "--parallel=16 --verbose --only-newer"

[hooks]
path = "hook"   # relative to the config dir

[logging]
file = "log/sitesync.log"
```

---

## 3. TUI Screen Flow

```
  sitesync (no args)
        ↓
  ┌─────────────────────────────┐
  │  SCREEN 1: SitePickerModel  │  bubbles/list
  │  > mysite        (2d ago)   │
  │    othersite     (1w ago)   │  [n] New  [e] Edit  [q] Quit
  └────────┬────────────────────┘
           │ enter
           ▼
  ┌─────────────────────────────┐    [n]/[e] ──────────────────────────┐
  │  SCREEN 2: OpSelectModel    │                                       ▼
  │  > SQL + Files              │              ┌──────────────────────────────┐
  │    SQL only                 │  [b] Back    │  SCREEN 4: ConfigEditorModel │
  │    Files only               │              │  huh multi-page form wizard  │
  └────────┬────────────────────┘              │  Page 1: Basic + Source      │
           │ enter                             │  Page 2: Destination         │
           ▼                                   │  Page 3: DB options          │
  ┌─────────────────────────────┐              │  Page 4: Replace pairs       │
  │  SCREEN 3: SyncingModel     │              │  Page 5: File sync + hooks   │
  │  ✓ 1/7  Fetch SQL dump 100% │              │  [ctrl+s] Save [esc] Cancel  │
  │  ▶ 2/7  Find/Replace  [⠸]  │  [q] Abort  └──────────────────────────────┘
  │    3/7  Before hooks   ──   │  [l] Logs
  │    4/7  Import SQL     ──   │
  │    5/7  Between hooks  ──   │
  │    6/7  Sync files     ──   │
  │    7/7  After hooks    ──   │
  │  ─────────────────────────  │
  │  [scrollable log viewport]  │
  └─────────────────────────────┘
```

Step states per row:

- Pending: `    3/7  Before hooks   ──`
- Active: `  ▶ 3/7  Before hooks   [spinner]`
- Done: `  ✓ 3/7  Before hooks   ████████ 100%`
- Failed: `  ✗ 3/7  Before hooks   FAILED`

---

## 4. Key Implementation Details

### 4a. PHP serialize()-aware find/replace (`internal/sync/replace.go`)

Two regex passes (serialized first, then raw):

```
phpSerialPattern        = `s:(\d+):"((?:[^"\\]|\\.)*?)";`
phpSerialPatternEscaped = `s:(\d+):\"((?:[^"\\]|\\.)*?)\";`
```

For each match, count occurrences of search string inside `inner`, then:

```
newN = oldN + count * (len([]byte(replace)) - len([]byte(search)))
newInner = strings.ReplaceAll(inner, search, replace)
```

**Fix over PHP original**: PHP only finds the first occurrence in a serialized value (bug). Go counts _all_ occurrences → correct byte count always.

**Buffer size**: Use `bufio.Scanner` with 4MB buffer for long SQL lines. Process line-by-line; write to tempfile, then `os.Rename`.

**UTF-8**: Lengths use `len([]byte(s))` (byte count), not `len(s)` (rune count).

### 4b. Engine → TUI event channel

```go
// events.go
type EventType uint8
const (
    EvStepStart EventType = iota
    EvStepDone
    EvStepFail
    EvLog
    EvProgress  // float64 0.0-1.0
    EvDone
)
type Event struct {
    Type     EventType
    Step     int
    Message  string
    Progress float64
}
```

Engine runs in a goroutine, sends to `chan<- Event`. TUI drains one event per `waitForEvent()` tea.Cmd, keeping Bubble Tea's message loop alive:

```go
func waitForEvent(ch <-chan sync.Event) tea.Cmd {
    return func() tea.Msg { e, _ := <-ch; return e }
}
```

Subprocess stdout/stderr is read by two goroutines (one each), each line is sent as `EvLog`. Context cancellation stops the subprocess on abort.

### 4c. Hooks — env-var bridge

Hooks can no longer be `source`d. They run as `bash script.sh` subprocesses with all config fields exported as env vars using the **original shell variable names** so existing hook scripts work unchanged:

```
sqlfile, src_server, src_user, src_port, src_site_host, src_site_protocol,
src_site_slug, src_files_root, src_dbname, src_dbuser, src_dbhostname,
dst_site_host, dst_site_protocol, dst_site_slug, dst_files_root,
dst_dbname, dst_dbuser, dst_dbhostname, dst_path_to_mysql,
dst_path_to_rsync, dst_path_to_resilient_replace, ...
```

Set `dst_path_to_resilient_replace=sitesync replace` so the Go binary handles it.
Set `dst_path_to_php=echo` to prevent hard failures on old hook scripts that call `$dst_path_to_php`.

### 4d. Config editor (huh embedded form)

Use `charmbracelet/huh` in embedded Bubble Tea mode (not `form.Run()`).
Replace pairs and sync pairs use a multi-line textarea (`search==>replace` per line) — simpler than dynamic form rows.
On save: serialize back to `[]ReplacePair` by splitting lines on `==>`.

---

## 5. Go Dependencies

```
github.com/charmbracelet/bubbletea   v1.2.x
github.com/charmbracelet/bubbles     v0.20.x   # list, viewport, spinner, progress
github.com/charmbracelet/lipgloss    v1.0.x
github.com/charmbracelet/huh         v0.6.x    # forms/wizard
github.com/BurntSushi/toml           v1.4.x    # config parser
github.com/spf13/cobra               v1.8.x    # CLI flags
```

No other external deps. All sync logic uses stdlib (`os/exec`, `bufio`, `regexp`, etc.).

---

## 6. Migration Command

`sitesync migrate [--conf=NAME] [--all] [--dry-run]`

Algorithm:

1. Regex-parse shell `etc/{name}/config` for `KEY="VALUE"` assignments
2. Resolve `$var` interpolations (multi-pass until stable)
3. Extract `replace_src[]` / `replace_dst[]` array appends → `[]ReplacePair`
4. Extract `sync_src[]` / `sync_dst[]` → `[]SyncPair`
5. Extract ignore tables from `--ignore-table=` flags
6. Extract rsync excludes from `--exclude` flags
7. Map all known variable names to Config struct fields
8. Write `config.toml` alongside the old file
9. Print summary of mapped fields and any unknowns

---

## 7. CLI Subcommands (Cobra)

```
sitesync                          # Launch TUI (default)
sitesync --conf=NAME              # Launch TUI pre-selecting NAME
sitesync --conf=NAME --no-tui     # Headless run (sql + files)
sitesync --conf=NAME --no-tui sql    # Headless, sql only
sitesync --conf=NAME --no-tui files  # Headless, files only
sitesync migrate [--conf=NAME] [--all] [--dry-run]
sitesync replace <search> <replace> <file>  # Standalone resilient_replace
sitesync version
```

---

## 8. Files to Create (in order)

| #   | File                              | Notes                                  |
| --- | --------------------------------- | -------------------------------------- |
| 1   | `go.mod`                          | Module root                            |
| 2   | `internal/config/config.go`       | All structs                            |
| 3   | `internal/config/loader.go`       | ListConfigs, Load, Save                |
| 4   | `internal/sync/events.go`         | Event types                            |
| 5   | `internal/sync/replace.go`        | Core logic                             |
| 6   | `internal/sync/replace_test.go`   | Tests; validate before building on top |
| 7   | `internal/logger/logger.go`       | File logger                            |
| 8   | `internal/sync/database.go`       | Steps 1 + 4                            |
| 9   | `internal/sync/hooks.go`          | Steps 3, 5, 7                          |
| 10  | `internal/sync/files.go`          | Step 6                                 |
| 11  | `internal/sync/engine.go`         | 7-step orchestrator                    |
| 12  | `internal/tui/styles/styles.go`   | Lip Gloss palette                      |
| 13  | `internal/tui/models/picker.go`   | Screen 1                               |
| 14  | `internal/tui/models/opselect.go` | Screen 2                               |
| 15  | `internal/tui/models/syncing.go`  | Screen 3 (most complex)                |
| 16  | `internal/tui/models/editor.go`   | Screen 4                               |
| 17  | `internal/tui/app.go`             | Root model                             |
| 18  | `cmd/sitesync/main.go`            | Entry point + Cobra                    |
| 19  | `internal/config/migrate.go`      | Migration tool                         |
| 20  | `sample/config.toml`              | Annotated reference                    |

---

## 9. Critical Reference Files (existing)

- `sitesync` (root) — authoritative spec for all 7 steps, skip logic, error handling, env-var naming
- `bin/resilient_replace` — PHP source to port; byte-count arithmetic and two-pass strategy
- `inc/config-base` — all default values and variable names → maps to TOML fields + hook env exports
- `sample/config` — full config template; basis for annotated `sample/config.toml`
- `sample/hook/after/wordpress.sh` — most complex hook; defines the `$dst_path_to_php $dst_path_to_resilient_replace` env-var contract

---

## 10. Verification

```bash
# 1. Unit tests for replace.go (day 1)
go test ./internal/sync/ -run TestResilientReplace -v
# Must pass: basic, multi-occurrence, no-op, raw-only, UTF-8 byte length, regex mode

# 2. Config round-trip
go test ./internal/config/ -run TestLoadSave -v

# 3. Migration on existing samples
go run ./cmd/sitesync migrate --conf=sample --dry-run
go run ./cmd/sitesync migrate --all

# 4. Headless smoke test (local_file source type)
go run ./cmd/sitesync --conf=test --no-tui sql

# 5. Hook execution test
# Create etc/test/hook/before/01-echo.sh, run, check log output

# 6. TUI navigation (manual)
go run ./cmd/sitesync
# Walk: picker → opselect → editor (new site) → save → picker → sync

# 7. Full rsync integration (requires SSH access)
go run ./cmd/sitesync --conf=mysite files

# 8. End-to-end WordPress test
# Verify serialized wp_options byte counts after URL replacement
```
