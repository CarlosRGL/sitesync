# sitesync v3

Pull a remote website to your local development environment — database, files, and all. A single Go binary with a full terminal UI.

```
  ⚡ sitesync

  ┌─────────────────────────────────────┐
  │  > mysite          (2h ago)         │
  │    staging         (1d ago)         │
  │    client-site     (never)          │
  │                                     │
  │  / search  │  enter sync  │  q quit │
  └─────────────────────────────────────┘
```

---

## What it does

sitesync synchronises a remote website to your local machine in 7 steps:

| Step | What happens                                                                                                                                                        |
| ---- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1    | Fetch the remote SQL dump over SSH (or use a local/remote file)                                                                                                     |
| 2    | Run find/replace on the dump — URLs, paths, hostnames. **PHP `serialize()` data is handled correctly**: `s:N:` byte-counts are recalculated after every replacement |
| 3    | Run **before hooks** — e.g. truncate cache tables, fix encoding                                                                                                     |
| 4    | Import the modified SQL dump into your local database                                                                                                               |
| 5    | Run **between hooks** — post-import, pre-sync customisations                                                                                                        |
| 6    | Sync files from the remote server via `rsync` or `lftp`                                                                                                             |
| 7    | Run **after hooks** — e.g. fix `.htaccess` rewrites, clear local caches                                                                                             |

Any step can be skipped (`sql` or `files` mode). All steps are streamed live to the TUI with:

- **Progress bars** for rsync file transfers and SQL imports
- **Color-coded logs** — commands, info, data, and timing are styled differently
- **Per-step timing** and total elapsed time
- **Connection info** — source, database, replacements, and transport details shown before sync starts
- **MariaDB compatibility** — automatically strips `/*M!` comments from MariaDB dumps before importing into MySQL
- **Trailing-slash safety** — source paths without a trailing `/` are normalised so rsync copies directory _contents_, not the directory itself

Supports WordPress, Drupal, PrestaShop, SPIP, and any other PHP-serializing CMS.

Works on **macOS** and **Linux** (amd64 / arm64). Can run on servers for server-to-server migration.

---

## Requirements

- SSH access to the remote server
- `rsync` or `lftp` for file sync
- `mysqldump` / `mysql` for database sync
- `gzip` (optional — for compressed dumps)
- **Go 1.23+** only needed if building from source

No PHP, no Bash runtime — a single static binary.

---

## Installation

### Quick install (no Go required)

```bash
curl -fsSL https://gitlab.quai13.net/teamtreize/sitesync/-/raw/main/install.sh | bash
```

This auto-detects your OS (macOS / Linux) and architecture (amd64 / arm64), downloads the correct binary, and installs it to `~/bin`. If no pre-built binary is available, it falls back to building from source (requires Go).

Then run the interactive setup:

```bash
sitesync setup
```

The setup wizard defaults the config directory to `~/.config/sitesync` — a clean XDG-compliant location that works well on both desktops and servers.

### Build from source

```bash
git clone ssh://git@gitlab.quai13.net:2221/teamtreize/sitesync.git
cd sitesync
make install          # builds and copies to ~/bin/sitesync
sitesync setup        # interactive environment setup
```

### Cross-compile release binaries

```bash
make release          # builds for darwin/amd64, darwin/arm64, linux/amd64, linux/arm64
ls dist/              # upload these to GitLab releases
```

The `setup` command walks you through:

1. Setting the config directory (`SITESYNC_ETC`)
2. Adding the env variable to your shell profile
3. Installing the binary to `~/bin` (or a custom path)
4. Migrating any existing shell configs to TOML

### Update

Just re-run the install script or `git pull && make install`. Configs are never touched.

### Verify

```bash
sitesync version
# sitesync 3.0.0
```

---

## Quick start

### 1. Create a config

Use the TUI editor (press `n` in the picker) or create one manually:

```bash
mkdir -p $SITESYNC_ETC/mysite
cp sample/config.toml $SITESYNC_ETC/mysite/config.toml
```

Edit `config.toml` with your remote server details, DB credentials, and find/replace pairs.

### 2. Set up SSH key auth (recommended)

```bash
ssh-copy-id user@your-server.com
```

This avoids password prompts during sync.

### 3. Run the TUI

```bash
sitesync
```

Use arrow keys to pick a site, press `enter`, choose what to sync, and watch the live progress view.

### 4. Run headlessly (for scripts / cron)

```bash
sitesync --conf=mysite --no-tui         # SQL + files
sitesync --conf=mysite --no-tui sql     # database only
sitesync --conf=mysite --no-tui files   # files only
```

---

## Configuration

Each site has its own TOML config file at `etc/{name}/config.toml`.

### Minimal example

```toml
[site]
name = "mysite"

[source]
server    = "www.example.com"
user      = "deploy"
type      = "remote_base"      # SSH into server and run mysqldump
db_name   = "prod_db"
db_user   = "db_user"
db_password = "secret"
files_root = "/var/www/mysite"

[destination]
files_root  = "/Users/me/Sites/mysite"
db_name     = "mysite_local"
db_user     = "root"

[[replace]]
search  = "https://www.example.com"
replace = "http://mysite.local"

[[replace]]
search  = "/var/www/mysite"
replace = "/Users/me/Sites/mysite"

[[sync]]
src = "/var/www/mysite"
dst = "/Users/me/Sites/mysite"
```

See `sample/config.toml` for the full reference with every available field.

### Full config reference

#### `[site]`

| Field         | Description                          |
| ------------- | ------------------------------------ |
| `name`        | Display name shown in the TUI picker |
| `description` | Optional one-line description        |

#### `[source]`

| Field               | Default       | Description                                               |
| ------------------- | ------------- | --------------------------------------------------------- |
| `server`            |               | Remote hostname or IP                                     |
| `user`              |               | SSH user                                                  |
| `port`              | `22`          | SSH port                                                  |
| `type`              | `remote_base` | Source type — see below                                   |
| `file`              |               | File path (required when `type` is `*_file`)              |
| `compress`          | `true`        | gzip dump on-the-fly                                      |
| `db_hostname`       | `localhost`   | DB host on the remote server                              |
| `db_name`           |               | Remote DB name                                            |
| `db_user`           |               | Remote DB user (omit to use `~/.my.cnf`)                  |
| `db_password`       |               | Remote DB password                                        |
| `site_host`         |               | Remote site hostname                                      |
| `site_protocol`     | `http://`     | Remote site protocol                                      |
| `site_slug`         |               | Sub-path (e.g. `/blog`)                                   |
| `files_root`        |               | Absolute path to site root on remote server               |
| `path_to_mysqldump` | `mysqldump`   | Override binary path on remote                            |
| `remote_nice`       |               | Prefix command for I/O throttling, e.g. `ionice -c3 nice` |

**Source types:**

| Type          | Behaviour                                               |
| ------------- | ------------------------------------------------------- |
| `remote_base` | SSH into server, run `mysqldump`, stream back (default) |
| `local_base`  | Run `mysqldump` against a local DB                      |
| `remote_file` | `scp` an existing `.sql` or `.sql.gz` from the server   |
| `local_file`  | Use an existing local SQL file                          |

#### `[destination]`

| Field               | Default     | Description                      |
| ------------------- | ----------- | -------------------------------- |
| `site_host`         |             | Local site hostname              |
| `site_protocol`     | `http://`   | Local site protocol              |
| `files_root`        |             | Absolute local path to site root |
| `db_hostname`       | `localhost` | Local DB host                    |
| `db_name`           |             | Local DB name                    |
| `db_user`           |             | Local DB user                    |
| `db_password`       |             | Local DB password                |
| `path_to_mysql`     | `mysql`     | Override binary path             |
| `path_to_mysqldump` | `mysqldump` | Override binary path             |
| `path_to_rsync`     | `rsync`     | Override binary path             |
| `path_to_lftp`      | `lftp`      | Override binary path             |
| `local_nice`        |             | I/O throttling prefix            |

#### `[database]`

| Field                   | Default                        | Description                     |
| ----------------------- | ------------------------------ | ------------------------------- |
| `sql_options_structure` | `--default-character-set=utf8` | Options passed to `mysqldump`   |
| `sql_options_extra`     | `--routines --skip-triggers`   | Additional `mysqldump` flags    |
| `ignore_tables`         | `[]`                           | Tables to exclude from the dump |

```toml
[database]
sql_options_structure = "--default-character-set=utf8mb4"
sql_options_extra     = "--routines --skip-triggers --single-transaction"
ignore_tables = [
  "cache_block",
  "cache_menu",
  "watchdog",
]
```

#### `[[replace]]`

Ordered list of find/replace pairs applied to the SQL dump. Can have as many entries as needed.

```toml
[[replace]]
search  = "https://www.example.com"
replace = "http://mysite.local"

[[replace]]
search  = "https://example.com"
replace = "http://mysite.local"

[[replace]]
# Fix DEFINER clauses in stored procedures
search  = " DEFINER=`prod_user`@`localhost`"
replace = " DEFINER=`root`@`localhost`"
```

> **PHP serialize() is handled automatically.** WordPress and other CMSes store serialised PHP arrays in the database. A naive string replacement corrupts those values because `s:N:` lengths become wrong. sitesync recalculates all byte-counts correctly — including a fix for a bug in the original PHP implementation where multiple occurrences in one serialised value were miscounted.

#### `[[sync]]`

One or more source → destination pairs for file synchronisation.

```toml
[[sync]]
src = "/var/www/mysite"
dst = "/Users/me/Sites/mysite"

# Sync only uploads, not the whole site:
[[sync]]
src = "/var/www/mysite/wp-content/uploads"
dst = "/Users/me/Sites/mysite/wp-content/uploads"
```

#### `[transport]`

| Field           | Default                       | Description                              |
| --------------- | ----------------------------- | ---------------------------------------- |
| `type`          | `rsync`                       | `rsync` or `lftp`                        |
| `rsync_options` | `-uvrpztl`                    | rsync flags (excludes move to `exclude`) |
| `exclude`       | `[".git/", ".DS_Store", ...]` | Patterns excluded from file sync         |

```toml
[transport]
type          = "rsync"
rsync_options = "-uvrpztl"
exclude = [
  "/sitesync/",
  "/stats/",
  "/.git/",
  "/node_modules/",
  "/.DS_Store",
]
```

For LFTP:

```toml
[transport]
type = "lftp"

[transport.lftp]
password        = "ftppassword"
port            = 21
connect_options = "set ftp:ssl-allow no;"
mirror_options  = "--parallel=16 --verbose --only-newer"
```

#### `[hooks]`

```toml
[hooks]
path = "hook"   # relative to etc/{name}/
```

Hook scripts live at `etc/{name}/hook/{before,between,after}/*.sh`.

#### `[logging]`

```toml
[logging]
file = "log/sitesync.log"   # relative to project root, or absolute
```

---

## Hooks

Hooks are shell scripts that run at specific points in the sync workflow. They let you customise the process without modifying sitesync itself.

### Directory layout

```
etc/mysite/
└── hook/
    ├── before/          # Run before DB import (step 3)
    │   └── 01-clear-caches.sh
    ├── between/         # Run after DB import, before file sync (step 5)
    │   └── 01-update-settings.sh
    └── after/           # Run after file sync (step 7)
        └── 01-fix-htaccess.sh
```

Scripts run in alphabetical order within each phase. Prefix with numbers to control ordering.

### Available environment variables

Every config field is exported as an environment variable so hook scripts can reference them:

| Variable                        | Value                               |
| ------------------------------- | ----------------------------------- |
| `sqlfile`                       | Absolute path to the SQL dump file  |
| `src_server`                    | Remote server hostname              |
| `src_user`                      | SSH user                            |
| `src_port`                      | SSH port                            |
| `src_site_host`                 | Remote site hostname                |
| `src_site_protocol`             | Remote site protocol                |
| `src_site_slug`                 | Remote site sub-path                |
| `src_files_root`                | Remote files root path              |
| `src_dbname`                    | Remote DB name                      |
| `src_dbuser`                    | Remote DB user                      |
| `src_dbhostname`                | Remote DB hostname                  |
| `dst_site_host`                 | Local site hostname                 |
| `dst_site_protocol`             | Local site protocol                 |
| `dst_files_root`                | Local files root path               |
| `dst_dbname`                    | Local DB name                       |
| `dst_dbuser`                    | Local DB user                       |
| `dst_dbhostname`                | Local DB hostname                   |
| `dst_path_to_mysql`             | Path to `mysql` binary              |
| `dst_path_to_rsync`             | Path to `rsync` binary              |
| `dst_path_to_resilient_replace` | sitesync's own `replace` subcommand |

### Example hooks

**Clear Drupal caches before import** (`hook/before/01-drupal.sh`):

```bash
#!/bin/bash
# Truncate all cache_* tables in the dump before importing
SQL="$(grep '^CREATE TABLE `cache' "$sqlfile" | sed 's/CREATE TABLE /TRUNCATE /;s/ ($/;/')"
echo "$SQL" >> "$sqlfile"
```

**Fix WordPress `.htaccess` after sync** (`hook/after/01-wordpress.sh`):

```bash
#!/bin/bash
$dst_path_to_resilient_replace -i \
  "RewriteCond %{HTTP_HOST} \^${src_site_host}\$" \
  "RewriteCond %{HTTP_HOST} ^${dst_site_host}$" \
  "${dst_files_root}/.htaccess"
```

**Clear a cache directory** (`hook/after/02-clear-cache.sh`):

```bash
#!/bin/bash
rm -rf "${dst_files_root}/var/cache/"*
```

Ready-made examples for WordPress, PrestaShop 1.5/1.6, SPIP, and Drupal 7 are in `sample/hook/`.

---

## Migrating from v2 (shell config)

If you have existing `etc/*/config` shell-variable config files, convert them to TOML with:

```bash
# Preview what will be generated (no files written)
sitesync migrate --conf=mysite --dry-run

# Convert a single config
sitesync migrate --conf=mysite

# Convert all shell configs that don't yet have a config.toml
sitesync migrate --all
```

The migrator:

- Resolves shell variable interpolations (`$var` references)
- Extracts `replace_src` / `replace_dst` array pairs → `[[replace]]` entries
- Extracts `sync_src` / `sync_dst` pairs → `[[sync]]` entries
- Pulls `--ignore-table=` and `--exclude` flags out of option strings into their own fields
- Lists any variables it couldn't map so you can handle them manually

After migrating, test with `--dry-run` before going full sync:

```bash
sitesync --conf=mysite --no-tui sql
```

---

## CLI reference

```
sitesync [flags] [sql|files]

Flags:
  --conf=NAME     Config name (uses etc/{NAME}/config.toml)
  --no-tui        Run without the interactive interface
  -h, --help      Help for sitesync

Arguments:
  sql             Sync database only
  files           Sync files only
  (none)          Sync both (default)
```

### Subcommands

```bash
sitesync version
# Print version

sitesync setup
# Interactive installer: set etc path, install binary, migrate configs

sitesync replace <search> <replace> <file>
# PHP serialize()-aware find/replace on a single file.
# Identical to the old bin/resilient_replace but with the multi-occurrence bug fixed.

sitesync migrate [--conf=NAME] [--all] [--dry-run]
# Convert shell config files to TOML format.
```

### Examples

```bash
# First-time setup (interactive)
sitesync setup

# Open TUI, pick a site interactively
sitesync

# Open TUI with a site pre-selected
sitesync --conf=mysite

# Headless: sync everything
sitesync --conf=mysite --no-tui

# Headless: database only (useful for quick schema refreshes)
sitesync --conf=mysite --no-tui sql

# Headless: files only (when you've already synced the DB)
sitesync --conf=mysite --no-tui files

# Standalone serialization-safe replace (usable in your own scripts)
sitesync replace "https://prod.example.com" "http://local.test" /path/to/dump.sql
```

---

## TUI navigation

### Site picker (main screen)

| Key       | Action                   |
| --------- | ------------------------ |
| `↑` / `↓` | Navigate the list        |
| `/`       | Filter / search configs  |
| `enter`   | Select site and proceed  |
| `n`       | Create a new config      |
| `e`       | Edit the selected config |
| `q`       | Quit                     |

### Operation selector

| Key         | Action              |
| ----------- | ------------------- |
| `↑` / `↓`   | Navigate            |
| `enter`     | Confirm selection   |
| `b` / `esc` | Back to site picker |

### Sync progress

| Key                   | Action                           |
| --------------------- | -------------------------------- |
| `l`                   | Toggle log panel visibility      |
| `q`                   | Abort (cancels the running step) |
| `q` (after done/fail) | Return to site picker            |

### Config editor

| Key         | Action                     |
| ----------- | -------------------------- |
| `tab`       | Next field                 |
| `shift+tab` | Previous field             |
| `enter`     | Advance through form pages |
| `ctrl+s`    | Save and exit              |
| `esc`       | Cancel (no changes saved)  |

---

## Project structure

```
sitesync/
├── cmd/sitesync/
│   ├── main.go                       # Entry point, Cobra CLI
│   └── setup.go                      # Interactive setup command
├── install.sh                            # curl-pipe installer (no Go needed)
├── Makefile                              # build, install, cross-compile
├── internal/
│   ├── config/
│   │   ├── config.go                 # TOML struct definitions
│   │   ├── loader.go                 # ListConfigs, Load, Save
│   │   └── migrate.go                # Shell → TOML converter
│   ├── sync/
│   │   ├── engine.go                 # 7-step orchestrator
│   │   ├── database.go               # Steps 1 and 4 (dump + import)
│   │   ├── replace.go                # PHP serialize()-aware find/replace
│   │   ├── replace_test.go           # 12 unit tests
│   │   ├── hooks.go                  # Steps 3, 5, 7 (hook runner)
│   │   ├── files.go                  # Step 6 (rsync / lftp)
│   │   └── events.go                 # Event types for engine→TUI channel
│   ├── tui/
│   │   ├── app.go                    # Root Bubble Tea model, screen router
│   │   ├── styles/styles.go          # Lip Gloss colour palette
│   │   └── models/
│   │       ├── picker/               # Screen 1: site list
│   │       ├── opselect/             # Screen 2: operation selector
│   │       ├── syncing/              # Screen 3: live progress + log
│   │       └── editor/               # Screen 4: huh config wizard
│   └── logger/logger.go              # Thread-safe log file writer
├── sample/
│   ├── config.toml                   # Annotated reference config
│   └── hook/                         # Ready-made hooks for common CMSes
│       ├── before/
│       │   ├── drupal-7.sh
│       │   ├── latin1.sh             # Encoding conversion
│       │   ├── prestashop-1.5-1.6.sh
│       │   └── spip.sh
│       └── after/
│           ├── wordpress.sh
│           ├── prestashop-1.5-1.6.sh
│           ├── spip.sh
│           └── chown-after-synchro.sh
└── log/                              # Log files (gitignored)
```

### Runtime directories (inside `$SITESYNC_ETC`)

```
$SITESYNC_ETC/                        # e.g. ~/.config/sitesync
├── mysite/
│   ├── config.toml
│   └── hook/
│       ├── before/
│       ├── between/
│       └── after/
├── another-site/
│   └── config.toml
├── tmp/                              # SQL dumps (auto-cleaned on success)
└── log/                              # Log files
```

---

## Dependencies

| Package                                                               | Purpose                                   |
| --------------------------------------------------------------------- | ----------------------------------------- |
| [charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea) | TUI framework                             |
| [charmbracelet/bubbles](https://github.com/charmbracelet/bubbles)     | List, viewport, spinner, progress widgets |
| [charmbracelet/lipgloss](https://github.com/charmbracelet/lipgloss)   | Terminal styling and layout               |
| [charmbracelet/huh](https://github.com/charmbracelet/huh)             | Interactive forms (config editor)         |
| [BurntSushi/toml](https://github.com/BurntSushi/toml)                 | TOML config parsing and encoding          |
| [spf13/cobra](https://github.com/spf13/cobra)                         | CLI flag and subcommand handling          |

No runtime dependencies — the compiled binary includes everything.

---

## Development

### Run tests

```bash
go test ./...
```

The replace engine has full test coverage including:

- Basic length adjustment
- Multi-occurrence byte count (fixed over the original PHP version)
- UTF-8 byte vs rune length correctness
- Escaped-quote serialized variant (`s:N:\"...\";`)
- Regex mode
- `OnlyIntoSerialized` flag
- Real-world WordPress `wp_options` row

### Run locally without installing

```bash
go run ./cmd/sitesync
```

### Lint

```bash
go vet ./...
```

---

## Upgrade notes from v2

sitesync v3 is a full rewrite. The core behaviour is identical, but there are a few changes to be aware of:

**Config format** — shell-variable configs are replaced by TOML. Use `sitesync migrate` to convert. Old `etc/*/config` files are not read in v3.

**No PHP required** — `bin/resilient_replace` is no longer used. The Go binary handles everything. The `$dst_path_to_resilient_replace` env variable passed to hooks now points to `sitesync replace`.

**Hook scripts are unchanged** — existing `hook/before/*.sh` and `hook/after/*.sh` scripts work without modification. They receive the same environment variables as before. The only difference is `$dst_path_to_php` is set to `echo` (a harmless no-op) since hooks no longer need to invoke PHP.

**`--verbose` flag removed** — the TUI log panel shows everything in real time. Use `--no-tui` for script usage; all output goes to stdout.

**Server-to-server** — v3 works on headless servers. Install with the curl script, set `SITESYNC_ETC`, and run with `--no-tui`.
