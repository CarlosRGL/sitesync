package sync

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/carlosrgl/sitesync/internal/config"
)

// FetchDump implements Step 1: fetch the remote or local SQL dump.
// It writes the dump to dumpPath (a temp file path).
func FetchDump(ctx context.Context, cfg *config.Config, dumpPath string, eventCh chan<- Event) error {
	sendLog := func(msg string) {
		sendEvent(ctx, eventCh, Event{Type: EvLog, Step: 1, Message: msg})
	}

	var err error
	switch cfg.Source.Type {
	case "local_file":
		sendLog(fmt.Sprintf("  source: local file %s", cfg.Source.File))
		err = copyFile(cfg.Source.File, dumpPath)

	case "remote_file":
		sendLog(fmt.Sprintf("  source: %s@%s:%s", cfg.Source.User, cfg.Source.Server, cfg.Source.File))
		sendLog(fmt.Sprintf("  port: %d", cfg.Source.Port))
		err = scpFetch(ctx, cfg, dumpPath, sendLog)

	case "local_base":
		sendLog(fmt.Sprintf("  source: local mysqldump → %s", cfg.Source.DBName))
		err = dumpLocalDB(ctx, cfg, dumpPath, sendLog)

	case "remote_base":
		sendLog(fmt.Sprintf("  source: %s@%s → %s", cfg.Source.User, cfg.Source.Server, cfg.Source.DBName))
		sendLog(fmt.Sprintf("  host: %s  port: %d", cfg.Source.Server, cfg.Source.Port))
		err = dumpRemoteDB(ctx, cfg, dumpPath, sendLog)

	default:
		return fmt.Errorf("unknown source type %q", cfg.Source.Type)
	}

	if err != nil {
		return err
	}

	// Report dump file size.
	if fi, statErr := os.Stat(dumpPath); statErr == nil {
		sendLog(fmt.Sprintf("  dump: %s (%s)", filepath.Base(dumpPath), humanSize(fi.Size())))
	}
	return nil
}

// ImportDump implements Step 4: import the SQL dump into the local database.
func ImportDump(ctx context.Context, cfg *config.Config, dumpPath string, eventCh chan<- Event, step int) error {
	sendLog := func(msg string) {
		sendEvent(ctx, eventCh, Event{Type: EvLog, Step: step, Message: msg})
	}

	sendLog(fmt.Sprintf("  target: %s@%s → %s", cfg.Destination.DBUser, cfg.Destination.DBHostname, cfg.Destination.DBName))

	f, err := os.Open(dumpPath)
	if err != nil {
		return fmt.Errorf("open dump file: %w", err)
	}
	defer f.Close()

	// Get file size for progress reporting.
	fi, _ := f.Stat()
	fileSize := fi.Size()
	sendLog(fmt.Sprintf("  dump size: %s", humanSize(fileSize)))

	var reader io.Reader = f

	// If the file is gzip-compressed, decompress using stdlib — no external gunzip needed.
	if strings.HasSuffix(dumpPath, ".gz") {
		gr, gzErr := gzip.NewReader(f)
		if gzErr != nil {
			return fmt.Errorf("open gzip reader: %w", gzErr)
		}
		defer gr.Close()
		reader = gr
	}

	// Strip MariaDB-specific comments that break MySQL import,
	// but only when the dump actually contains them.
	if isMariaDBDump(dumpPath) {
		sendLog("  detected MariaDB dump, stripping M! comments")
		reader = newMariaDBStripper(reader, sendLog)
	}

	mysqlArgs := buildMySQLArgs(cfg)
	mysql := exec.CommandContext(ctx, mysqlBin(cfg), mysqlArgs...)
	// Wrap reader with progress tracking.
	if fileSize > 0 {
		reader = &progressReader{
			r:       reader,
			total:   fileSize,
			eventCh: eventCh,
			step:    step,
			ctx:     ctx,
		}
	}
	mysql.Stdin = reader

	// Log the command with password redacted.
	sendLog(fmt.Sprintf("  $ %s %s", mysqlBin(cfg), redactArgs(mysqlArgs)))

	return streamCmd(ctx, eventCh, step, mysql, true)
}

// ── helpers ─────────────────────────────────────────────────────────────────

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func scpFetch(ctx context.Context, cfg *config.Config, dumpPath string, log func(string)) error {
	port := fmt.Sprintf("%d", cfg.Source.Port)
	src := fmt.Sprintf("%s@%s:%s", cfg.Source.User, cfg.Source.Server, cfg.Source.File)
	cmd := exec.CommandContext(ctx, "scp", "-P", port, src, dumpPath)
	log(fmt.Sprintf("  $ scp -P %s %s %s", port, src, dumpPath))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("scp: %w\n%s", err, out)
	}
	return nil
}

func dumpLocalDB(ctx context.Context, cfg *config.Config, dumpPath string, log func(string)) error {
	args := buildDumpArgs(cfg, false)
	return runDump(ctx, cfg, args, dumpPath, log)
}

func dumpRemoteDB(ctx context.Context, cfg *config.Config, dumpPath string, log func(string)) error {
	// Build the mysqldump command to run remotely over SSH.
	dumpBin := cfg.Source.PathToMysqldump
	if dumpBin == "" {
		dumpBin = "mysqldump"
	}
	remoteParts := []string{dumpBin}
	remoteParts = append(remoteParts, buildDumpArgs(cfg, true)...)
	remoteCmd := strings.Join(remoteParts, " ")

	sshArgs := []string{
		"-p", fmt.Sprintf("%d", cfg.Source.Port),
		fmt.Sprintf("%s@%s", cfg.Source.User, cfg.Source.Server),
		remoteCmd,
	}
	log(fmt.Sprintf("  $ ssh %s", strings.Join(sshArgs, " ")))

	out, err := os.Create(dumpPath)
	if err != nil {
		return fmt.Errorf("create dump file: %w", err)
	}
	defer out.Close()

	cmd := exec.CommandContext(ctx, "ssh", sshArgs...)
	cmd.Stdout = out
	stderrPipe, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ssh start: %w", err)
	}

	// Wait for stderr goroutine before returning to prevent log-loss race.
	var wg sync.WaitGroup
	if stderrPipe != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sc := bufio.NewScanner(stderrPipe)
			for sc.Scan() {
				log("  " + sc.Text())
			}
		}()
	}
	err = cmd.Wait()
	wg.Wait()
	return err
}

func runDump(ctx context.Context, cfg *config.Config, args []string, dumpPath string, log func(string)) error {
	dumpBin := cfg.Destination.PathToMysqldump
	if dumpBin == "" {
		dumpBin = "mysqldump"
	}
	log(fmt.Sprintf("  $ %s %s", dumpBin, strings.Join(args, " ")))

	out, err := os.Create(dumpPath)
	if err != nil {
		return fmt.Errorf("create dump file: %w", err)
	}
	defer out.Close()

	cmd := exec.CommandContext(ctx, dumpBin, args...)
	cmd.Stdout = out
	stderrPipe, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return err
	}

	// Wait for stderr goroutine before returning to prevent log-loss race.
	var wg sync.WaitGroup
	if stderrPipe != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sc := bufio.NewScanner(stderrPipe)
			for sc.Scan() {
				log("  " + sc.Text())
			}
		}()
	}
	err = cmd.Wait()
	wg.Wait()
	return err
}

func buildDumpArgs(cfg *config.Config, remote bool) []string {
	var args []string

	if cfg.Database.SQLOptionsStructure != "" {
		args = append(args, strings.Fields(cfg.Database.SQLOptionsStructure)...)
	}
	if cfg.Database.SQLOptionsExtra != "" {
		args = append(args, strings.Fields(cfg.Database.SQLOptionsExtra)...)
	}

	src := cfg.Source
	args = append(args, "-h", src.DBHostname)
	if src.DBPort != "" {
		args = append(args, "-P", src.DBPort)
	}
	if src.DBUser != "" {
		args = append(args, "-u", src.DBUser)
	}
	if src.DBPassword != "" {
		args = append(args, fmt.Sprintf("-p%s", src.DBPassword))
	}
	for _, tbl := range cfg.Database.IgnoreTables {
		args = append(args, fmt.Sprintf("--ignore-table=%s.%s", src.DBName, tbl))
	}
	// Append DBName for both local and remote; guard against empty name.
	if src.DBName != "" {
		args = append(args, src.DBName)
	}
	_ = remote // reserved for future use (e.g. --compress flag differentiation)
	return args
}

func buildMySQLArgs(cfg *config.Config) []string {
	dst := cfg.Destination
	var args []string
	args = append(args, "-h", dst.DBHostname)
	if dst.DBPort != "" {
		args = append(args, "-P", dst.DBPort)
	}
	if dst.DBUser != "" {
		args = append(args, "-u", dst.DBUser)
	}
	if dst.DBPassword != "" {
		args = append(args, fmt.Sprintf("-p%s", dst.DBPassword))
	}
	args = append(args, dst.DBName)
	return args
}

func mysqlBin(cfg *config.Config) string {
	if cfg.Destination.PathToMySQL != "" {
		return cfg.Destination.PathToMySQL
	}
	return "mysql"
}

// redactArgs returns a space-joined arg list with any -p<password> replaced
// by -p[REDACTED] so credentials are never exposed in log output or TUI.
func redactArgs(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		if strings.HasPrefix(a, "-p") && len(a) > 2 {
			out[i] = "-p[REDACTED]"
		} else {
			out[i] = a
		}
	}
	return strings.Join(out, " ")
}

// streamCmd runs cmd and streams stderr (and optionally stdout) lines as EvLog events.
// It waits for all scanner goroutines to finish via WaitGroup before returning,
// preventing a log-loss race where cmd.Wait() exits before the last lines are emitted.
func streamCmd(ctx context.Context, eventCh chan<- Event, step int, cmd *exec.Cmd, stderrOnly bool) error {
	stderrPipe, _ := cmd.StderrPipe()
	var stdoutPipe io.ReadCloser
	if !stderrOnly {
		stdoutPipe, _ = cmd.StdoutPipe()
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	var wg sync.WaitGroup

	scan := func(r io.Reader) {
		defer wg.Done()
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 512*1024), 512*1024)
		for sc.Scan() {
			sendEvent(ctx, eventCh, Event{Type: EvLog, Step: step, Message: sc.Text()})
		}
	}

	if stderrPipe != nil {
		wg.Add(1)
		go scan(stderrPipe)
	}
	if stdoutPipe != nil {
		wg.Add(1)
		go scan(stdoutPipe)
	}

	err := cmd.Wait()
	wg.Wait() // drain all log lines before returning
	return err
}

// DumpFilePath returns the expected path for the SQL dump temp file.
func DumpFilePath(tmpDir, confName string) string {
	return filepath.Join(tmpDir, fmt.Sprintf("%s.sql", confName))
}

// progressReader wraps an io.Reader and emits EvProgress events as data is read.
type progressReader struct {
	r       io.Reader
	total   int64
	read    int64
	eventCh chan<- Event
	step    int
	lastPct int
	ctx     context.Context
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.r.Read(p)
	pr.read += int64(n)
	pct := int(float64(pr.read) / float64(pr.total) * 100)
	// Only emit progress every ~1% to avoid flooding the channel.
	if pct != pr.lastPct && pct <= 100 {
		pr.lastPct = pct
		sendEvent(pr.ctx, pr.eventCh, Event{Type: EvProgress, Step: pr.step, Progress: float64(pct) / 100.0})
	}
	return n, err
}

// ── MariaDB comment stripper ────────────────────────────────────────────────

// mariaDBCommentRe matches MariaDB-specific comments that MySQL rejects:
//   - /*M!999999\- enable the sandbox mode */
var mariaDBCommentRe = regexp.MustCompile(`/\*M!.*?\*/`)

// mariaDBLineRe matches full-line MariaDB comments (the most common case).
var mariaDBLineRe = regexp.MustCompile(`(?m)^\s*/\*M!.*?\*/\s*;?\s*$`)

// newMariaDBStripper returns an io.Reader that strips MariaDB-specific
// comments line-by-line from r before passing data downstream.
func newMariaDBStripper(r io.Reader, logFn func(string)) io.Reader {
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 16*1024*1024), 16*1024*1024) // 16 MB line buffer
		stripped := 0
		for sc.Scan() {
			line := sc.Bytes()
			// Skip full-line MariaDB comments entirely.
			if mariaDBLineRe.Match(line) {
				stripped++
				continue
			}
			// Strip inline MariaDB comments within a line.
			cleaned := mariaDBCommentRe.ReplaceAll(line, nil)
			// Write the (possibly cleaned) line + newline.
			if _, err := pw.Write(append(cleaned, '\n')); err != nil {
				pw.CloseWithError(err)
				return
			}
		}
		if err := sc.Err(); err != nil {
			pw.CloseWithError(err)
			return
		}
		if stripped > 0 {
			logFn(fmt.Sprintf("  stripped %d MariaDB-specific comment line(s)", stripped))
		}
	}()
	return pr
}

// isMariaDBDump does a quick check on the first few KB of a file to detect
// whether it contains MariaDB-specific /*M! comments that need stripping.
func isMariaDBDump(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 8192)
	n, _ := f.Read(buf)
	return bytes.Contains(buf[:n], []byte("/*M!"))
}
