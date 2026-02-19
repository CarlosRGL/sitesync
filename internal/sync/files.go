package sync

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/carlosrgl/sitesync/internal/config"
)

// SyncFiles implements Step 6: transfer files via rsync or lftp.
func SyncFiles(ctx context.Context, cfg *config.Config, eventCh chan<- Event, step int) error {
	if len(cfg.Sync) == 0 {
		eventCh <- Event{Type: EvLog, Step: step, Message: "No sync pairs configured, skipping file sync"}
		return nil
	}

	switch cfg.Transport.Type {
	case "lftp":
		return syncLFTP(ctx, cfg, eventCh, step)
	default:
		return syncRsync(ctx, cfg, eventCh, step)
	}
}

func syncRsync(ctx context.Context, cfg *config.Config, eventCh chan<- Event, step int) error {
	rsyncBin := cfg.Destination.PathToRsync
	if rsyncBin == "" {
		rsyncBin = "rsync"
	}

	total := len(cfg.Sync)
	for idx, pair := range cfg.Sync {
		args := buildRsyncArgs(cfg, pair)
		eventCh <- Event{Type: EvLog, Step: step,
			Message: fmt.Sprintf("  $ %s %s", rsyncBin, strings.Join(args, " "))}

		cmd := exec.CommandContext(ctx, rsyncBin, args...)
		baseProgress := float64(idx) / float64(total)
		sliceSize := 1.0 / float64(total)
		if err := streamCmdWithProgress(ctx, eventCh, step, cmd, baseProgress, sliceSize); err != nil {
			return fmt.Errorf("rsync %s → %s: %w", pair.Src, pair.Dst, err)
		}
	}
	return nil
}

func buildRsyncArgs(cfg *config.Config, pair config.SyncPair) []string {
	t := cfg.Transport
	src := cfg.Source
	dst := cfg.Destination

	opts := t.RsyncOptions
	if opts == "" {
		opts = "-uvrpztl"
	}
	args := strings.Fields(opts)

	// SSH transport options
	sshOpt := fmt.Sprintf("ssh -p %d", src.Port)
	args = append(args, "-e", sshOpt)

	// Progress reporting
	args = append(args, "--info=progress2")

	// Exclusions
	for _, ex := range t.Exclude {
		args = append(args, "--exclude", ex)
	}

	_ = dst // destination is already in pair.Dst

	// source: user@host:path
	remoteSrc := fmt.Sprintf("%s@%s:%s", src.User, src.Server, pair.Src)
	args = append(args, remoteSrc, pair.Dst)

	return args
}

func syncLFTP(ctx context.Context, cfg *config.Config, eventCh chan<- Event, step int) error {
	lftpBin := cfg.Destination.PathToLftp
	if lftpBin == "" {
		lftpBin = "lftp"
	}

	lf := cfg.Transport.LFTP
	for _, pair := range cfg.Sync {
		script := buildLFTPScript(cfg, pair)
		eventCh <- Event{Type: EvLog, Step: step,
			Message: fmt.Sprintf("  $ %s -c '...'", lftpBin)}
		eventCh <- Event{Type: EvLog, Step: step,
			Message: "  " + script}

		cmd := exec.CommandContext(ctx, lftpBin, "-c", script)
		_ = lf
		if err := streamCmd(ctx, eventCh, step, cmd, false); err != nil {
			return fmt.Errorf("lftp %s → %s: %w", pair.Src, pair.Dst, err)
		}
	}
	return nil
}

func buildLFTPScript(cfg *config.Config, pair config.SyncPair) string {
	src := cfg.Source
	lf := cfg.Transport.LFTP
	t := cfg.Transport

	mirrorOpts := lf.MirrorOptions
	if mirrorOpts == "" {
		mirrorOpts = "--parallel=4 --verbose --only-newer"
	}
	for _, ex := range t.Exclude {
		mirrorOpts += fmt.Sprintf(" --exclude %s", ex)
	}

	port := lf.Port
	if port == 0 {
		port = 21
	}

	connect := lf.ConnectOptions

	var sb strings.Builder
	if connect != "" {
		sb.WriteString(connect + "; ")
	}

	protocol := strings.TrimSuffix(src.SiteProtocol, "://")
	if protocol == "" {
		protocol = "ftp"
	}
	url := fmt.Sprintf("%s://%s@%s:%d%s", protocol, src.User, src.Server, port, pair.Src)
	if lf.Password != "" {
		url = fmt.Sprintf("%s://%s:%s@%s:%d%s", protocol, src.User, lf.Password, src.Server, port, pair.Src)
	}

	sb.WriteString(fmt.Sprintf("open %s; ", url))
	sb.WriteString(fmt.Sprintf("mirror %s . %s", mirrorOpts, pair.Dst))
	return sb.String()
}

// reRsyncProgress matches rsync --info=progress2 output lines like:
//
//	32,768 100%    2.74MB/s    0:00:00 (xfr#1, to-chk=0/1)
var reRsyncProgress = regexp.MustCompile(`(\d+)%`)

// streamCmdWithProgress runs a command, parses rsync --info=progress2 output
// for percentage updates, and emits EvProgress events. baseProgress is the
// starting progress (0.0–1.0) and sliceSize is the fraction of total progress
// this command represents.
func streamCmdWithProgress(ctx context.Context, eventCh chan<- Event, step int, cmd *exec.Cmd, baseProgress, sliceSize float64) error {
	stderrPipe, _ := cmd.StderrPipe()
	stdoutPipe, _ := cmd.StdoutPipe()

	if err := cmd.Start(); err != nil {
		return err
	}

	// scanCR splits input on \r or \n so we can parse rsync --info=progress2
	// output which uses \r to update the same line.
	scanCR := func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}
		for i, b := range data {
			if b == '\r' || b == '\n' {
				return i + 1, data[:i], nil
			}
		}
		if atEOF {
			return len(data), data, nil
		}
		return 0, nil, nil
	}

	scanProgress := func(r io.Reader) {
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 512*1024), 512*1024)
		sc.Split(scanCR)
		for sc.Scan() {
			line := sc.Text()
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			// Try to parse rsync progress percentage
			if m := reRsyncProgress.FindStringSubmatch(trimmed); len(m) >= 2 {
				if pct, err := strconv.Atoi(m[1]); err == nil {
					p := baseProgress + (float64(pct)/100.0)*sliceSize
					if p > 1.0 {
						p = 1.0
					}
					eventCh <- Event{Type: EvProgress, Step: step, Progress: p}
					continue // don't log raw progress lines
				}
			}
			eventCh <- Event{Type: EvLog, Step: step, Message: trimmed}
		}
	}

	if stderrPipe != nil {
		go scanProgress(stderrPipe)
	}
	if stdoutPipe != nil {
		go scanProgress(stdoutPipe)
	}

	return cmd.Wait()
}
