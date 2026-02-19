package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// ConfigEntry describes a discovered config in the etc/ directory.
type ConfigEntry struct {
	Name         string
	Path         string // absolute path to config.toml
	LastModified time.Time
}

// etcDir returns the path to the etc/ directory.
//
// Resolution order:
//  1. $SITESYNC_ETC environment variable (explicit override — recommended for global installs)
//  2. Walk up from the current working directory looking for an etc/ sibling
//  3. Fall back to ./etc
//
// The walk stops one level before the filesystem root to avoid matching the
// system /etc directory.
func etcDir() string {
	// 1. Explicit env override
	if v := os.Getenv("SITESYNC_ETC"); v != "" {
		return v
	}

	// 2. Walk up from cwd — stop before root so we never match /etc
	cwd, _ := os.Getwd()
	dir := cwd
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root — do not check /etc (system directory)
			break
		}
		candidate := filepath.Join(dir, "etc")
		if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
			return candidate
		}
		dir = parent
	}

	// 3. Fallback
	return filepath.Join(cwd, "etc")
}

// ListConfigs returns all named configs found under etc/
// Each named config lives at etc/{name}/config.toml
func ListConfigs() ([]ConfigEntry, error) {
	base := etcDir()
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading etc/: %w", err)
	}

	var configs []ConfigEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		cfgPath := filepath.Join(base, e.Name(), "config.toml")
		fi, err := os.Stat(cfgPath)
		if err != nil {
			continue // not a sitesync config dir
		}
		configs = append(configs, ConfigEntry{
			Name:         e.Name(),
			Path:         cfgPath,
			LastModified: fi.ModTime(),
		})
	}
	sort.Slice(configs, func(i, j int) bool {
		return configs[i].Name < configs[j].Name
	})
	return configs, nil
}

// Load reads the named config from etc/{name}/config.toml.
func Load(name string) (*Config, error) {
	cfgPath := filepath.Join(etcDir(), name, "config.toml")
	return LoadFromPath(cfgPath)
}

// LoadFromPath reads a config from an explicit file path.
func LoadFromPath(path string) (*Config, error) {
	cfg := DefaultConfig()
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("loading %s: %w", path, err)
	}
	cfg.configFilePath = path
	resolveConfigVariables(&cfg)
	return &cfg, nil
}

// resolveConfigVariables replaces $var / ${var} references in Replace and Sync
// pairs with actual values from the config. This is a safety net for configs
// migrated from the old shell format that still contain literal variable names.
func resolveConfigVariables(cfg *Config) {
	reVar := regexp.MustCompile(`\$\{?([a-zA-Z_][a-zA-Z0-9_]*)\}?`)

	vars := map[string]string{
		"src_site_protocol": cfg.Source.SiteProtocol,
		"src_site_host":     cfg.Source.SiteHost,
		"src_site_site":     cfg.Source.SiteHost, // common alias
		"src_site_slug":     cfg.Source.SiteSlug,
		"src_files_root":    cfg.Source.FilesRoot,
		"src_dbuser":        cfg.Source.DBUser,
		"src_dbhostname":    cfg.Source.DBHostname,
		"src_dbname":        cfg.Source.DBName,
		"src_server":        cfg.Source.Server,
		"dst_site_protocol": cfg.Destination.SiteProtocol,
		"dst_site_host":     cfg.Destination.SiteHost,
		"dst_site_slug":     cfg.Destination.SiteSlug,
		"dst_files_root":    cfg.Destination.FilesRoot,
		"dst_dbuser":        cfg.Destination.DBUser,
		"dst_dbhostname":    cfg.Destination.DBHostname,
		"dst_dbname":        cfg.Destination.DBName,
	}

	resolve := func(s string) string {
		if !strings.Contains(s, "$") {
			return s
		}
		return reVar.ReplaceAllStringFunc(s, func(ref string) string {
			m := reVar.FindStringSubmatch(ref)
			if len(m) < 2 {
				return ref
			}
			if val, ok := vars[m[1]]; ok && val != "" {
				return val
			}
			return ref
		})
	}

	for i := range cfg.Replace {
		cfg.Replace[i].Search = resolve(cfg.Replace[i].Search)
		cfg.Replace[i].Replace = resolve(cfg.Replace[i].Replace)
	}
	for i := range cfg.Sync {
		cfg.Sync[i].Src = resolve(cfg.Sync[i].Src)
		cfg.Sync[i].Dst = resolve(cfg.Sync[i].Dst)
	}
}

// Save writes cfg to etc/{name}/config.toml, creating directories as needed.
func Save(name string, cfg *Config) error {
	dir := filepath.Join(etcDir(), name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	path := filepath.Join(dir, "config.toml")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating config file: %w", err)
	}
	defer f.Close()

	enc := toml.NewEncoder(f)
	if err := enc.Encode(cfg); err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	cfg.configFilePath = path
	return nil
}

// HookDir returns the absolute path to the hook directory for a given phase.
// phase is one of: before, between, after
func HookDir(cfg *Config, phase string) string {
	hookBase := cfg.Hooks.Path
	if !filepath.IsAbs(hookBase) {
		hookBase = filepath.Join(filepath.Dir(cfg.configFilePath), hookBase)
	}
	return filepath.Join(hookBase, phase)
}

// LogFile returns the absolute path to the log file.
func LogFile(cfg *Config) string {
	lf := cfg.Logging.File
	if lf == "" {
		lf = "log/sitesync.log"
	}
	if filepath.IsAbs(lf) {
		return lf
	}
	// Resolve relative to project root (parent of etc/)
	root := filepath.Dir(etcDir())
	return filepath.Join(root, lf)
}

// TmpDir returns the absolute path to the temp directory (inside the etc dir).
func TmpDir() string {
	return filepath.Join(etcDir(), "tmp")
}
