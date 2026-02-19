package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// MigrateResult holds the output of a shell config migration.
type MigrateResult struct {
	Config     *Config
	Preview    string
	FieldCount int
	Unknown    []string
}

// MigrateShellConfig reads etc/{name}/config (shell format) and produces
// a Config struct + TOML preview. Does not write any files.
func MigrateShellConfig(name string) (*MigrateResult, error) {
	shellPath := filepath.Join(etcDir(), name, "config")
	data, err := os.ReadFile(shellPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", shellPath, err)
	}

	vars, arrays, err := parseShellConfig(string(data))
	if err != nil {
		return nil, err
	}

	cfg := DefaultConfig()
	cfg.Site.Name = name
	unknown := applyVars(vars, arrays, &cfg)

	// Build a TOML preview string
	var sb strings.Builder
	enc := newSimpleTomlWriter(&sb)
	enc.write(&cfg)

	return &MigrateResult{
		Config:     &cfg,
		Preview:    sb.String(),
		FieldCount: len(vars),
		Unknown:    unknown,
	}, nil
}

// ListShellConfigs returns config names that have a shell config but no config.toml.
func ListShellConfigs() ([]string, error) {
	base := etcDir()
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		shellPath := filepath.Join(base, e.Name(), "config")
		tomlPath := filepath.Join(base, e.Name(), "config.toml")
		if _, err := os.Stat(shellPath); err == nil {
			if _, err := os.Stat(tomlPath); os.IsNotExist(err) {
				names = append(names, e.Name())
			}
		}
	}
	return names, nil
}

// ── Shell config parser ───────────────────────────────────────────────────────

var (
	// Simple assignment: KEY="value" or KEY=value (no spaces around =)
	// Handles optional semicolons before comments: KEY="value"; # comment
	reAssign = regexp.MustCompile(`(?m)^([a-zA-Z_][a-zA-Z0-9_]*)=["']?([^"'\n#]*)["']?\s*;?\s*(?:#.*)?$`)
	// Array append: KEY+=("value") or KEY=("value")
	reArrayAppend = regexp.MustCompile(`(?m)^([a-zA-Z_][a-zA-Z0-9_]*)\+?=\("([^"]*)"\)\s*;?\s*(?:#.*)?$`)
	// Variable reference inside a value: $varname or ${varname}
	reVarRef = regexp.MustCompile(`\$\{?([a-zA-Z_][a-zA-Z0-9_]*)\}?`)
)

func parseShellConfig(content string) (vars map[string]string, arrays map[string][]string, err error) {
	vars = make(map[string]string)
	arrays = make(map[string][]string)

	// First pass: collect raw assignments (no interpolation yet)
	for _, m := range reAssign.FindAllStringSubmatch(content, -1) {
		vars[m[1]] = m[2]
	}
	// Collect array appends
	for _, m := range reArrayAppend.FindAllStringSubmatch(content, -1) {
		arrays[m[1]] = append(arrays[m[1]], m[2])
	}

	// Second pass: resolve variable references (multi-pass until stable)
	for range 5 {
		changed := false
		for k, v := range vars {
			resolved := reVarRef.ReplaceAllStringFunc(v, func(ref string) string {
				name := strings.Trim(ref, "${}")
				name = strings.TrimPrefix(name, "$")
				if val, ok := vars[name]; ok && !strings.Contains(val, "$") {
					return val
				}
				return ref
			})
			if resolved != v {
				vars[k] = resolved
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	// Third pass: resolve variable references in array values
	for k, vals := range arrays {
		for i, v := range vals {
			resolved := reVarRef.ReplaceAllStringFunc(v, func(ref string) string {
				name := strings.Trim(ref, "${}")
				name = strings.TrimPrefix(name, "$")
				if val, ok := vars[name]; ok {
					return val
				}
				return ref
			})
			arrays[k][i] = resolved
		}
	}

	return vars, arrays, nil
}

// ── Variable → Config field mapping ─────────────────────────────────────────

// applyVars maps known shell variable names to the Config struct fields.
// Returns a slice of variable names that were not mapped.
func applyVars(vars map[string]string, arrays map[string][]string, cfg *Config) []string {
	known := map[string]bool{}

	set := func(key string, target *string) {
		if v, ok := vars[key]; ok {
			*target = v
			known[key] = true
		}
	}
	setInt := func(key string, target *int) {
		if v, ok := vars[key]; ok {
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
				*target = n
			}
			known[key] = true
		}
	}
	setBool := func(key string, target *bool) {
		if v, ok := vars[key]; ok {
			*target = v != "0" && v != ""
			known[key] = true
		}
	}

	// Source
	set("src_server", &cfg.Source.Server)
	set("src_user", &cfg.Source.User)
	setInt("src_port", &cfg.Source.Port)
	set("src_type", &cfg.Source.Type)
	set("src_file", &cfg.Source.File)
	setBool("compress", &cfg.Source.Compress)
	set("src_dbhostname", &cfg.Source.DBHostname)
	set("src_dbname", &cfg.Source.DBName)
	set("src_dbuser", &cfg.Source.DBUser)
	set("src_dbpass", &cfg.Source.DBPassword)
	set("src_dbport", &cfg.Source.DBPort)
	set("src_site_protocol", &cfg.Source.SiteProtocol)
	set("src_site_host", &cfg.Source.SiteHost)
	set("src_site_slug", &cfg.Source.SiteSlug)
	set("src_files_root", &cfg.Source.FilesRoot)
	// Both old variable names for the remote mysqldump binary
	set("src_path_to_mysqldump", &cfg.Source.PathToMysqldump)
	set("path_to_mysqldump", &cfg.Source.PathToMysqldump)
	set("remote_nice", &cfg.Source.RemoteNice)

	// Destination
	set("dst_site_protocol", &cfg.Destination.SiteProtocol)
	set("dst_site_host", &cfg.Destination.SiteHost)
	set("dst_site_slug", &cfg.Destination.SiteSlug)
	set("dst_files_root", &cfg.Destination.FilesRoot)
	set("dst_dbhostname", &cfg.Destination.DBHostname)
	set("dst_dbname", &cfg.Destination.DBName)
	set("dst_dbuser", &cfg.Destination.DBUser)
	set("dst_dbpass", &cfg.Destination.DBPassword)
	set("dst_dbport", &cfg.Destination.DBPort)
	set("dst_path_to_mysql", &cfg.Destination.PathToMySQL)
	set("dst_path_to_mysqldump", &cfg.Destination.PathToMysqldump)
	set("dst_path_to_rsync", &cfg.Destination.PathToRsync)
	set("dst_path_to_lftp", &cfg.Destination.PathToLftp)
	set("local_nice", &cfg.Destination.LocalNice)

	// Database
	set("sql_options_structure", &cfg.Database.SQLOptionsStructure)
	set("sql_options_extra", &cfg.Database.SQLOptionsExtra)
	// sql_options = $sql_options_structure + extra flags (--routines --skip-triggers etc.)
	// Extract just the extra part by stripping the structure prefix.
	if v, ok := vars["sql_options"]; ok {
		known["sql_options"] = true
		if cfg.Database.SQLOptionsExtra == "" {
			extra := strings.TrimSpace(strings.TrimPrefix(v, vars["sql_options_structure"]))
			if extra != "" {
				cfg.Database.SQLOptionsExtra = extra
			}
		}
	}
	// Extract ignore tables from sql_ignores string: --ignore-table=dbname.tablename
	if v, ok := vars["sql_ignores"]; ok {
		known["sql_ignores"] = true
		re := regexp.MustCompile(`--ignore-table=\S+\.(\S+)`)
		for _, m := range re.FindAllStringSubmatch(v, -1) {
			cfg.Database.IgnoreTables = append(cfg.Database.IgnoreTables, m[1])
		}
	}

	// Transport
	set("transport_type", &cfg.Transport.Type)
	set("rsync_options", &cfg.Transport.RsyncOptions)
	// Extract excludes from rsync_options: --exclude pattern
	if v, ok := vars["rsync_options"]; ok {
		reEx := regexp.MustCompile(`--exclude\s+(\S+)`)
		for _, m := range reEx.FindAllStringSubmatch(v, -1) {
			cfg.Transport.Exclude = append(cfg.Transport.Exclude, m[1])
		}
		// Strip --exclude flags from RsyncOptions (they move to Exclude field)
		cfg.Transport.RsyncOptions = reEx.ReplaceAllString(v, "")
		cfg.Transport.RsyncOptions = strings.TrimSpace(cfg.Transport.RsyncOptions)
	}
	set("lftp_pass", &cfg.Transport.LFTP.Password)
	if v, ok := vars["lftp_src_port"]; ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			cfg.Transport.LFTP.Port = n
		}
		known["lftp_src_port"] = true
	}
	set("lftp_connect_options", &cfg.Transport.LFTP.ConnectOptions)
	set("lftp_mirror_command_options", &cfg.Transport.LFTP.MirrorOptions)

	// Logging
	set("logfile", &cfg.Logging.File)

	// Replace pairs: zip replace_src[i] + replace_dst[i]
	srcs := arrays["replace_src"]
	dsts := arrays["replace_dst"]
	known["replace_src"] = true
	known["replace_dst"] = true
	for i := 0; i < len(srcs) && i < len(dsts); i++ {
		cfg.Replace = append(cfg.Replace, ReplacePair{Search: srcs[i], Replace: dsts[i]})
	}

	// Sync pairs: zip sync_src[i] + sync_dst[i]
	ss := arrays["sync_src"]
	sd := arrays["sync_dst"]
	known["sync_src"] = true
	known["sync_dst"] = true
	for i := 0; i < len(ss) && i < len(sd); i++ {
		cfg.Sync = append(cfg.Sync, SyncPair{Src: ss[i], Dst: sd[i]})
	}

	// Collect unknown variables
	// (variables that were in the file but we have no mapping for)
	ignored := map[string]bool{
		// These are in config-base and are framework internals, not site-specific
		"red": true, "green": true, "yellow": true, "blue": true,
		"magenta": true, "cyan": true, "nc": true, "bold": true,
		"logfile_enabled": true, "verbose": true,
		// Deprecated v2 variables — no longer needed in v3
		"dst_path_to_php":               true,
		"dst_path_to_resilient_replace": true,
		"resilient_replace_options":     true,
		// Helper/intermediate variables whose values are resolved into other fields
		"src_site_web": true, // used to build replace pairs; already interpolated
		"exclude_dirs": true, // folded into rsync_options during variable resolution
	}
	// Also ignore numbered-suffix variants of helper variables (src_site_host2, dst_site_host3, etc.)
	// These are intermediate vars used to build replace pairs; values are already resolved.
	ignoredPrefixes := []string{
		"src_site_web", "dst_site_host", "src_site_host", "src_site_site",
		"replace_src_", "replace_dst_", "sync_src_", "sync_dst_",
	}
	var unknown []string
	for k := range vars {
		if known[k] || ignored[k] {
			continue
		}
		skip := false
		for _, pfx := range ignoredPrefixes {
			if strings.HasPrefix(k, pfx) {
				skip = true
				break
			}
		}
		if !skip {
			unknown = append(unknown, k)
		}
	}
	return unknown
}

// ── Minimal TOML writer for preview ─────────────────────────────────────────

// newSimpleTomlWriter returns a helper that writes a human-readable TOML preview.
// We use the BurntSushi encoder for the actual save; this is just for --dry-run preview.
type simpleTomlWriter struct {
	sb *strings.Builder
}

func newSimpleTomlWriter(sb *strings.Builder) *simpleTomlWriter {
	return &simpleTomlWriter{sb: sb}
}

func (w *simpleTomlWriter) write(cfg *Config) {
	enc := fmt.Sprintf
	w.sb.WriteString(enc("[site]\nname = %q\ndescription = %q\n\n", cfg.Site.Name, cfg.Site.Description))
	w.sb.WriteString(fmt.Sprintf("[source]\nserver = %q\nuser = %q\nport = %d\ntype = %q\n\n",
		cfg.Source.Server, cfg.Source.User, cfg.Source.Port, cfg.Source.Type))
	// (abbreviated preview — the real file uses toml.Encoder)
	w.sb.WriteString(fmt.Sprintf("# ... %d replace pairs, %d sync pairs\n",
		len(cfg.Replace), len(cfg.Sync)))
}
