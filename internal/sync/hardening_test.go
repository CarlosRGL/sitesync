package sync

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/carlosrgl/sitesync/internal/config"
)

func TestSplitCommandLineHandlesQuotedValues(t *testing.T) {
	args, err := splitCommandLine(`--flag "value with spaces" --path='/tmp/my dir' plain`)
	if err != nil {
		t.Fatalf("splitCommandLine returned error: %v", err)
	}

	want := []string{"--flag", "value with spaces", "--path=/tmp/my dir", "plain"}
	if len(args) != len(want) {
		t.Fatalf("got %d args, want %d: %#v", len(args), len(want), args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("arg %d = %q, want %q", i, args[i], want[i])
		}
	}
}

func TestBuildDumpArgsPreservesQuotedOptions(t *testing.T) {
	cfg := &config.Config{
		Source: config.SourceConfig{
			DBHostname: "localhost",
			DBName:     "prod",
			DBUser:     "root",
		},
		Database: config.DatabaseConfig{
			SQLOptionsStructure: `--defaults-file="/tmp/mysql config.cnf"`,
			SQLOptionsExtra:     `--set-gtid-purged=OFF`,
		},
	}

	args, err := buildDumpArgs(cfg, false)
	if err != nil {
		t.Fatalf("buildDumpArgs returned error: %v", err)
	}

	found := false
	for _, arg := range args {
		if arg == "--defaults-file=/tmp/mysql config.cnf" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("quoted option was not preserved: %#v", args)
	}
}

func TestHookEnvOmitsSecretsByDefault(t *testing.T) {
	cfg := &config.Config{
		Source:      config.SourceConfig{DBPassword: "source-secret", SiteSlug: "/src"},
		Destination: config.DestConfig{DBPassword: "dest-secret", SiteSlug: "/dst"},
	}

	env := hookEnv(cfg, "/tmp/dump.sql", false)
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "src_dbpass=") || strings.Contains(joined, "dst_dbpass=") {
		t.Fatalf("hook env unexpectedly exposed DB password variables: %s", joined)
	}
	if !strings.Contains(joined, "src_root_slug=/src") {
		t.Fatalf("hook env did not expose source slug alias: %s", joined)
	}
	if !strings.Contains(joined, "dst_root_slug=/dst") {
		t.Fatalf("hook env did not expose destination slug alias: %s", joined)
	}
	if !strings.Contains(joined, "dst_path_to_php=") {
		t.Fatalf("hook env did not expose legacy php placeholder: %s", joined)
	}

	env = hookEnv(cfg, "/tmp/dump.sql", true)
	joined = strings.Join(env, "\n")
	if !strings.Contains(joined, "src_dbpass=source-secret") {
		t.Fatalf("hook env did not expose source secret when requested: %s", joined)
	}
	if !strings.Contains(joined, "dst_dbpass=dest-secret") {
		t.Fatalf("hook env did not expose destination secret when requested: %s", joined)
	}
}

func TestHookUsesDBSecrets(t *testing.T) {
	dir := t.TempDir()
	secretHook := filepath.Join(dir, "secret.sh")
	plainHook := filepath.Join(dir, "plain.sh")

	if err := os.WriteFile(secretHook, []byte("echo $dst_dbpass\n"), 0600); err != nil {
		t.Fatalf("write secret hook: %v", err)
	}
	if err := os.WriteFile(plainHook, []byte("echo ok\n"), 0600); err != nil {
		t.Fatalf("write plain hook: %v", err)
	}

	usesSecrets, err := hookUsesDBSecrets(secretHook)
	if err != nil {
		t.Fatalf("hookUsesDBSecrets(secretHook) error: %v", err)
	}
	if !usesSecrets {
		t.Fatal("expected secret hook to request DB password variables")
	}

	usesSecrets, err = hookUsesDBSecrets(plainHook)
	if err != nil {
		t.Fatalf("hookUsesDBSecrets(plainHook) error: %v", err)
	}
	if usesSecrets {
		t.Fatal("did not expect plain hook to request DB password variables")
	}
}

func TestBuildLFTPScriptRedactsPasswordInLogs(t *testing.T) {
	cfg := &config.Config{
		Source: config.SourceConfig{
			Server:       "example.com",
			User:         "deploy",
			SiteProtocol: "ftp://",
		},
		Transport: config.TransportConfig{
			LFTP: config.LFTPConfig{
				Password:      "super-secret",
				MirrorOptions: "--verbose",
			},
		},
	}

	script, logScript := buildLFTPScript(cfg, config.SyncPair{Src: "/remote", Dst: "/local"})
	if !strings.Contains(script, "super-secret") {
		t.Fatal("runtime LFTP script should contain the password for stdin auth")
	}
	if strings.Contains(logScript, "super-secret") {
		t.Fatalf("log script leaked password: %s", logScript)
	}
	if !strings.Contains(logScript, "[REDACTED]") {
		t.Fatalf("log script did not show redaction marker: %s", logScript)
	}
}

func TestStreamCmdReturnsTrailingOutputOnFailure(t *testing.T) {
	ctx := context.Background()
	eventCh := make(chan Event, 16)
	cmd := exec.CommandContext(ctx, "sh", "-c", "echo first >&2; echo second >&2; exit 1")

	err := streamCmd(ctx, eventCh, 4, cmd, true)
	if err == nil {
		t.Fatal("expected streamCmd to fail")
	}
	msg := err.Error()
	if !strings.Contains(msg, "last output:") {
		t.Fatalf("expected error to contain trailing output, got: %s", msg)
	}
	if !strings.Contains(msg, "first") || !strings.Contains(msg, "second") {
		t.Fatalf("expected stderr lines in error, got: %s", msg)
	}
}
