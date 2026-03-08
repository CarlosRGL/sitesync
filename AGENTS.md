# AGENTS.md — AI Agent Guide to sitesync

This document provides AI agents with comprehensive context about the sitesync codebase, architecture, conventions, and development patterns.

---

## Project Overview

**sitesync** is a Go-based tool for synchronizing remote websites to local development environments. It handles database dumps, file transfers, find/replace operations (including PHP `serialize()` data), and custom hooks.

### Core Features

1. **7-step sync workflow**: Fetch remote SQL → Replace strings (with PHP serialize support) → Run before hooks → Import DB → Run between hooks → Sync files → Run after hooks
2. **Interactive TUI**: Built with Bubble Tea, showing live progress, logs, and errors
3. **Headless mode**: Scriptable CLI for automation and cron jobs
4. **Config system**: TOML-based site configurations with migration from legacy shell configs
5. **Interactive SSH password prompts**: Supports both TUI and headless password entry when SSH keys aren't available
6. **Single-site bootstrap**: Quick setup mode for users with one migration target

### Tech Stack

- **Language**: Go 1.23+
- **TUI Framework**: charmbracelet/bubbletea v1.2.4 + bubbles + lipgloss + huh
- **CLI Framework**: spf13/cobra
- **Config Format**: TOML (BurntSushi/toml)
- **Architecture**: Event-driven sync engine with pub/sub to TUI
- **External Commands**: ssh, scp, rsync, mysqldump, mysql (via os/exec)

---

## Architecture

### High-Level Flow

```
User Input → Cobra CLI → TUI App or Headless Runner → Sync Engine → Steps → Events → UI/Logger
```

### Key Components

#### 1. CLI Layer (`cmd/sitesync/`)

- **main.go**: Cobra root command, version, environment detection
- **setup.go**: Interactive environment setup wizard (SITESYNC_ETC, shell profiles, single-site vs multi-site)
- **update.go**: Config migration from shell to TOML

#### 2. Config Layer (`internal/config/`)

- **config.go**: Config structs (`Config`, `Site`, `Source`, `Destination`, etc.), `DefaultConfig()`, `StarterConfig(name)` generator
- **loader.go**: `Load()`, `Save()`, `ListConfigs()`, `ValidateConfigName()` (exported for use by setup/installer)
- **migrate.go**: Shell config → TOML converter

#### 3. Sync Engine (`internal/sync/`)

- **engine.go**: `Run(ctx, cfg, eventChan)` orchestrates the 7 steps, emits events, handles retries
- **events.go**: Event contract (`EventType`, `Event`, `AuthReply`) for engine→consumer communication
- **auth.go**: SSH password retry coordinator (`runSSHCommandWithPasswordPrompt`, `shouldPromptForSSHPassword`, askpass helper generation)
- **database.go**: Step 1 (fetch dump) and Step 4 (import), uses password-aware SSH wrappers
- **replace.go**: Step 2 with PHP serialize() awareness (`ResilientReplaceLine`, byte-count recalculation)
- **hooks.go**: Steps 3, 5, 7 (before/between/after hook execution)
- **files.go**: Step 6 (rsync/lftp), password-aware transport

#### 4. TUI Layer (`internal/tui/`)

- **app.go**: Root Bubble Tea model, screen router, handles screen transitions
- **models/picker/**: Screen 1 — site list with search
- **models/opselect/**: Screen 2 — operation selector (SQL+files / SQL only / files only)
- **models/syncing/**: Screen 3 — live sync progress with logs, step status, password input field
- **models/editor/**: Screen 4 — huh-based config editor
- **styles/styles.go**: Consistent color palette (success, error, info, dim, etc.)

#### 5. Logger (`internal/logger/`)

- **logger.go**: Thread-safe file logger with configurable path

---

## Directory Structure

```
sitesync/
├── cmd/sitesync/               # CLI entry points
│   ├── main.go                 # Cobra root, version command
│   ├── setup.go                # Environment + config setup wizard
│   └── update.go               # Config migration (shell → TOML)
├── internal/
│   ├── config/
│   │   ├── config.go           # Config structs, defaults, StarterConfig()
│   │   ├── loader.go           # Load, Save, ListConfigs, ValidateConfigName
│   │   └── migrate.go          # Shell → TOML converter
│   ├── sync/
│   │   ├── engine.go           # 7-step orchestrator
│   │   ├── events.go           # Event types (EvStepStart, EvAuthRequest, etc.)
│   │   ├── auth.go             # SSH password retry + askpass helper
│   │   ├── database.go         # Steps 1 and 4 (dump + import)
│   │   ├── replace.go          # PHP serialize()-aware find/replace
│   │   ├── replace_test.go     # Table-driven tests, benchmarks, fuzz
│   │   ├── hooks.go            # Steps 3, 5, 7 (hook runner)
│   │   └── files.go            # Step 6 (rsync / lftp)
│   ├── tui/
│   │   ├── app.go              # Root Bubble Tea model
│   │   ├── styles/styles.go    # Lip Gloss color palette
│   │   └── models/             # Screen-specific Bubble Tea models
│   │       ├── picker/         # Site list
│   │       ├── opselect/       # Operation selector
│   │       ├── syncing/        # Live sync progress + password input
│   │       └── editor/         # Config editor wizard
│   └── logger/
│       └── logger.go           # Thread-safe log file writer
├── sample/
│   ├── config.toml             # Annotated reference config
│   └── hook/                   # Ready-made hooks for WordPress, Drupal, etc.
├── bin/                        # Compiled binaries (gitignored)
├── etc/                        # Site configs (100+ real-world examples)
├── install.sh                  # Curl-pipe installer with TTY-aware bootstrap
├── Makefile                    # Build, test, lint, release targets
└── README.md                   # User documentation
```

---

## Development Patterns

### Event-Driven Architecture

The sync engine emits events to consumers (TUI or headless runner):

```go
type EventType int

const (
    EvStepStart       // Step beginning (name, number)
    EvStepDone        // Step completed successfully
    EvStepFail        // Step failed (error, allows retry/continue/quit)
    EvStepSkip        // Step skipped
    EvStepProgress    // Progress update (bytes, percentage)
    EvLog             // Log message (info, command, data, timing)
    EvAuthRequest     // SSH password needed (NEW)
)

type Event struct {
    Type         EventType
    StepName     string
    StepNumber   int
    Message      string
    Error        error
    AuthReplyCh  chan<- AuthReply  // For EvAuthRequest
    // ... other fields
}
```

**Consumer responsibilities:**

- TUI: Updates UI models, renders progress bars, handles EvAuthRequest with textinput
- Headless: Prints logs to stdout, handles EvAuthRequest with term.ReadPassword()

### SSH Password Handling Pattern

SSH commands that may require authentication follow this retry pattern:

1. **First attempt**: Try with `-o BatchMode=yes` (non-interactive)
2. **Error detection**: Check stderr for "Permission denied" or "keyboard-interactive"
3. **Password request**: Emit `EvAuthRequest` event with reply channel
4. **Wait for reply**: Consumer sends `AuthReply{Password: "...", Cancel: false}`
5. **Retry**: Generate askpass helper script, set `SSH_ASKPASS` + `DISPLAY=:0`, retry command
6. **Cache**: Store password in context (`authState`) for subsequent SSH commands

**Key functions in `internal/sync/auth.go`:**

- `runSSHCommandWithPasswordPrompt(ctx, eventCh, attemptCallback)`: Main retry coordinator
- `shouldPromptForSSHPassword(stderr string) bool`: Error detection (matches "permission denied", "keyboard-interactive")
- `askpassScript(password string) string`: Generates temporary shell script for SSH_ASKPASS
- `authState` context key: Cached password for session

**Integration points:**

- `database.go`: `scpFetch()`, `dumpRemoteDB()` wrap their ssh/scp commands
- `files.go`: `syncRsync()` wraps rsync with SSH transport
- `engine.go`: `RunHeadless()` handles EvAuthRequest with `promptHiddenPassword()`
- `tui/models/syncing/syncing.go`: TUI screen handles EvAuthRequest with textinput field

### Config Generation Pattern

**Two modes:**

1. **DefaultConfig()**: Empty config with all defaults, used for new multi-site setups
2. **StarterConfig(name string)**: Pre-filled config with sensible defaults based on site name

**StarterConfig() prefills:**

- `Site.Name = name`
- `Source.DBName = name + "_remote"`
- `Destination.DBName = name + "_local"`
- `Destination.FilesRoot = ~/Sites/{name}`
- Replace pairs: URL and path replacements
- Sync pairs: Common patterns for WordPress/Laravel/generic PHP

**Usage:**

- `cmd/sitesync/setup.go`: Prompts for single vs multi-site, validates name, calls StarterConfig()
- `install.sh`: TTY-aware bootstrap calls `write_starter_config()` which templates a TOML file

### Testing Philosophy

- **Unit tests**: Table-driven tests for pure functions (replace.go, auth detection)
- **Integration tests**: config migration (update_test.go)
- **Fuzz testing**: `ResilientReplaceLine` with random inputs
- **Coverage target**: 80%+ for critical paths (replace, config, auth detection)

**Run tests:**

```bash
make test          # go test ./...
make test-race     # go test -race ./...
make coverage      # generates coverage.html
make fuzz          # fuzz ResilientReplaceLine for 30s
```

---

## Key Implementation Details

### PHP serialize() Handling

The `ResilientReplaceLine` function (in `internal/sync/replace.go`) correctly handles PHP serialized data:

1. Finds all `s:N:"...";` patterns
2. Performs replacements within quoted strings
3. Recalculates `N` (byte count, not character count)
4. Handles escaped quotes: `s:N:\"...\";`
5. **Bug fix over original PHP version**: Correctly counts multiple occurrences of the same pattern in one serialized blob

**Critical constraint**: Byte count is for UTF-8 bytes, not rune count. Use `len(str)`, not `utf8.RuneCountInString(str)`.

### TOML Config Validation

- Config names must not be empty, contain dots (`.`), or path separators (`/`)
- `ValidateConfigName()` is exported from `internal/config/loader.go` for use by setup/installer
- Config files live at `$SITESYNC_ETC/{name}/config.toml`

### Shell Profile Detection

`cmd/sitesync/setup.go` detects and patches shell profiles:

- Checks for `.zshrc`, `.bashrc`, `.bash_profile`, `.profile`
- Adds `export SITESYNC_ETC="..."`
- Idempotent: Doesn't duplicate existing entries

### Bubble Tea TUI Patterns

**Model updates:**

```go
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        // Handle user input
    case MyCustomMsg:
        // Handle custom message types
    }
    return m, cmd
}
```

**Password input field** (in `syncing.go`):

- `textinput.Model` with `EchoMode = textinput.EchoPassword`
- Temporarily overrides normal key handling when active
- `sendAuthReply()` sends password to engine via `AuthReplyCh`

### Error Handling Conventions

- **Sync steps**: Return `error`, engine emits `EvStepFail`, UI offers retry/continue/quit
- **TUI operations**: Return error via tea.Cmd as a message type
- **CLI validation**: Return error immediately with `fmt.Errorf()`, cobra prints to stderr

---

## Common Development Tasks

### Adding a New Config Field

1. Add field to struct in `internal/config/config.go`
2. Add TOML tag: `` `toml:"field_name"` ``
3. Update `DefaultConfig()` if the field has a default value
4. Update `StarterConfig()` if the field should be pre-filled
5. Update `sample/config.toml` with documentation
6. Update README.md config reference section

### Adding a New Sync Step

1. Define step number constant in `internal/sync/engine.go`
2. Add step execution in `Run()` switch case
3. Emit `EvStepStart` before step
4. Emit `EvStepDone` on success, `EvStepFail` on error
5. Update README.md "What it does" table

### Adding a New Event Type

1. Define constant in `internal/sync/events.go`
2. Add fields needed for this event to `Event` struct
3. Update consumers:
   - `internal/sync/engine.go` (RunHeadless)
   - `internal/tui/models/syncing/syncing.go` (TUI)
4. Document in AGENTS.md and code comments

### Adding SSH Command Retry Support to a New Function

1. Wrap command execution in `runSSHCommandWithPasswordPrompt()` callback
2. Pass `eventCh` from function signature or engine context
3. Extract authState from context: `state := getAuthState(ctx)`
4. In callback: Build command with `sshArgs(port, batchMode)`, pass `extraEnv` from authState
5. Handle stdout/stderr as needed in callback
6. See `database.go` (`scpFetch`, `dumpRemoteDB`) or `files.go` (`syncRsync`) for examples

---

## Constraints and Caveats

### SSH Password Support

- **Pattern matching**: `shouldPromptForSSHPassword()` matches common SSH error messages but may need adjustment for exotic SSH servers
- **Not tested live**: As of implementation, the retry flow is verified by unit tests and compilation but has NOT been tested against a real password-only SSH server
- **Recommended**: SSH key authentication is still the preferred method; password prompts are a fallback

### File Sync Limitations

- **rsync only**: File sync via rsync or lftp, no native Go implementation
- **No progress for lftp**: Progress parsing only supported for rsync
- **Trailing slash normalization**: Source paths without trailing `/` are normalized by the engine

### Hook Execution

- **Shell dependency**: Hooks require `/bin/sh` (or `/bin/bash` if script uses bash features)
- **Environment variable exposure**: Password variables (`src_dbpass`, `dst_dbpass`) only exported when hook script references them (security constraint)

### Database Support

- **MySQL/MariaDB only**: No PostgreSQL, SQLite, etc.
- **MariaDB comment stripping**: Engine auto-removes `/*M!` comments before MySQL import

---

## Release Process

```bash
make release     # Cross-compile for darwin/linux, amd64/arm64
make publish     # Upload to GitHub releases
```

Binaries are built for:

- darwin/amd64
- darwin/arm64
- linux/amd64
- linux/arm64

The `install.sh` script auto-detects OS/architecture and downloads the appropriate binary.

---

## Code Style and Conventions

### Go Conventions

- **Formatting**: `gofmt` (enforced by `make lint`)
- **Naming**: Camel case for exports, lowercase for private
- **Error handling**: Explicit checks, no panic in library code
- **Context**: Passed explicitly, used for cancellation and auth state

### TUI Conventions

- **Colors**: Use `internal/tui/styles` package constants (Success, Error, Info, Dim)
- **Layout**: Use `lipgloss.JoinVertical` / `JoinHorizontal` for composition
- **Updates**: Prefer pure functions, avoid side effects in Update()

### Config Conventions

- **Paths**: Absolute paths stored in config, resolved at load time
- **Defaults**: Set in `DefaultConfig()`, not scattered in code
- **Validation**: Done at load time, not sync time

---

## Debugging Tips

### TUI Issues

- Run with `--no-tui` to see raw output
- Check log file (configured in TOML or default `log/sitesync.log`)
- Use `tea.Println()` for debug output that doesn't break TUI

### SSH Issues

- Test SSH command manually: `ssh -v user@host 'command'`
- Check `shouldPromptForSSHPassword()` patterns match your SSH server errors
- Verify `SSH_ASKPASS` mechanism works: `DISPLAY=:0 SSH_ASKPASS=/tmp/script ssh ...`

### Config Issues

- Use `sitesync migrate --dry-run` to preview TOML generation
- Validate TOML syntax with `toml-test` or online validator
- Check `$SITESYNC_ETC` is set and points to correct directory

### Replace Issues

- Test patterns with `sitesync replace` command directly
- Check test cases in `internal/sync/replace_test.go` for examples
- Verify UTF-8 byte counting vs rune counting

---

## Contributing Guidelines

1. **Tests required**: New features must have unit tests
2. **No breaking changes**: Config format is stable, avoid backwards-incompatible changes
3. **Update docs**: README.md and AGENTS.md must be updated
4. **Lint clean**: Run `make lint` before committing
5. **Conventional commits**: Use descriptive commit messages

---

## External Resources

- [Bubble Tea Docs](https://github.com/charmbracelet/bubbletea)
- [Cobra CLI Guide](https://github.com/spf13/cobra)
- [TOML Spec](https://toml.io/en/)
- [PHP serialize() Format](https://www.php.net/manual/en/function.serialize.php)

---

## Quick Reference

### Important File Locations

- **Entry point**: `cmd/sitesync/main.go`
- **Sync orchestrator**: `internal/sync/engine.go`
- **TUI root**: `internal/tui/app.go`
- **Config loader**: `internal/config/loader.go`
- **Replace engine**: `internal/sync/replace.go`
- **Auth coordinator**: `internal/sync/auth.go`

### Important Functions

- `sync.Run(ctx, cfg, eventCh)` — Start sync workflow
- `config.Load(name)` — Load site config
- `config.StarterConfig(name)` — Generate prefilled config
- `sync.runSSHCommandWithPasswordPrompt()` — SSH with password retry
- `replace.ResilientReplaceLine()` — PHP-aware string replacement

### Important Constants

- `EvAuthRequest` — SSH password needed event
- `StepCount = 7` — Total sync steps
- Default port: 22 (SSH)
- Default DB hostname: "localhost"

---

**Last updated**: March 2026
**Target audience**: AI coding agents, contributors, maintainers
