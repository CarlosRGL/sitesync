package sync

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/carlosrgl/sitesync/internal/config"
)

// RunHooks runs all *.sh scripts in etc/{conf}/hook/{phase}/ as subprocesses,
// passing the full config as environment variables using original shell names.
func RunHooks(ctx context.Context, cfg *config.Config, phase string, sqlFile string, eventCh chan<- Event, step int) error {
	hookDir := config.HookDir(cfg, phase)

	entries, err := filepath.Glob(filepath.Join(hookDir, "*.sh"))
	if err != nil || len(entries) == 0 {
		return nil // no hooks is fine
	}
	sort.Strings(entries)

	for _, script := range entries {
		eventCh <- Event{Type: EvLog, Step: step,
			Message: fmt.Sprintf("  running hook: %s", filepath.Base(script))}

		cmd := exec.CommandContext(ctx, "bash", script)
		cmd.Env = hookEnv(cfg, sqlFile)
		cmd.Dir = filepath.Dir(cfg.ConfigFilePath())

		if err := streamCmd(ctx, eventCh, step, cmd, false); err != nil {
			return fmt.Errorf("hook %s failed: %w", filepath.Base(script), err)
		}
	}
	return nil
}

// hookEnv builds the environment variables passed to hook scripts.
// Variable names match the original Bash sitesync exactly so existing
// hook scripts work without modification.
func hookEnv(cfg *config.Config, sqlFile string) []string {
	base := os.Environ()
	src := cfg.Source
	dst := cfg.Destination

	// Find the sitesync binary path for the replace subcommand.
	self, _ := os.Executable()
	if self == "" {
		self = "sitesync"
	}

	extra := []string{
		"sqlfile=" + sqlFile,

		// Source
		"src_server=" + src.Server,
		"src_user=" + src.User,
		fmt.Sprintf("src_port=%d", src.Port),
		"src_site_host=" + src.SiteHost,
		"src_site_protocol=" + src.SiteProtocol,
		"src_site_slug=" + src.SiteSlug,
		"src_files_root=" + src.FilesRoot,
		"src_dbname=" + src.DBName,
		"src_dbuser=" + src.DBUser,
		"src_dbhostname=" + src.DBHostname,
		"src_dbpass=" + src.DBPassword,
		"src_type=" + src.Type,

		// Destination
		"dst_site_host=" + dst.SiteHost,
		"dst_site_protocol=" + dst.SiteProtocol,
		"dst_site_slug=" + dst.SiteSlug,
		"dst_files_root=" + dst.FilesRoot,
		"dst_dbname=" + dst.DBName,
		"dst_dbuser=" + dst.DBUser,
		"dst_dbhostname=" + dst.DBHostname,
		"dst_dbpass=" + dst.DBPassword,
		"dst_path_to_mysql=" + dst.PathToMySQL,
		"dst_path_to_rsync=" + dst.PathToRsync,
		"dst_path_to_mysqldump=" + dst.PathToMysqldump,
		"dst_path_to_lftp=" + dst.PathToLftp,

		// Replace utility — hooks that call resilient_replace use the Go binary.
		// Usage: $dst_path_to_resilient_replace <search> <replace> <file>
		"dst_path_to_resilient_replace=" + self + " replace",

		// Legacy: some hooks call `$dst_path_to_php $dst_path_to_resilient_replace`.
		// Point php to echo so it doesn't hard-fail.
		"dst_path_to_php=echo",
	}

	return append(base, extra...)
}

// buildMySQLArgsForHook returns a mysql CLI invocation string suitable for
// use inside hook bash scripts (e.g. for pipe-based SQL execution).
func buildMySQLArgsForHook(cfg *config.Config) string {
	dst := cfg.Destination
	parts := []string{dst.PathToMySQL}
	if dst.PathToMySQL == "" {
		parts[0] = "mysql"
	}
	parts = append(parts, "-h", dst.DBHostname)
	if dst.DBPort != "" {
		parts = append(parts, "-P", dst.DBPort)
	}
	if dst.DBUser != "" {
		parts = append(parts, "-u", dst.DBUser)
	}
	if dst.DBPassword != "" {
		parts = append(parts, fmt.Sprintf("-p%s", dst.DBPassword))
	}
	parts = append(parts, dst.DBName)
	return strings.Join(parts, " ")
}
