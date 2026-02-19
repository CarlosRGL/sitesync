package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/carlosrgl/sitesync/internal/config"
	"github.com/carlosrgl/sitesync/internal/logger"
)

// Run executes the full sync workflow in a goroutine, sending progress events
// to eventCh. It closes eventCh when done (success or failure).
//
// Call as: go Run(ctx, cfg, op, eventCh)
func Run(ctx context.Context, cfg *config.Config, op Op, eventCh chan<- Event, log *logger.Logger) {
	defer close(eventCh)

	tmpDir := config.TmpDir()
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		eventCh <- Event{Type: EvStepFail, Step: 1,
			Message: fmt.Sprintf("cannot create tmp dir: %v", err)}
		return
	}

	// Derive config name from file path for the dump file name.
	confName := filepath.Base(filepath.Dir(cfg.ConfigFilePath()))
	dumpPath := DumpFilePath(tmpDir, confName)

	skipSQL := op == OpFiles
	skipFiles := op == OpSQL

	steps := []struct {
		name string
		fn   func() error
	}{
		{"Fetch SQL dump", func() error {
			if skipSQL {
				return nil
			}
			return FetchDump(ctx, cfg, dumpPath, eventCh)
		}},
		{"Find / Replace", func() error {
			if skipSQL {
				return nil
			}
			if fi, err := os.Stat(dumpPath); err == nil {
				eventCh <- Event{Type: EvLog, Step: 2,
					Message: fmt.Sprintf("  processing %s (%s)", filepath.Base(dumpPath), humanSize(fi.Size()))}
			}
			for i, pair := range cfg.Replace {
				eventCh <- Event{Type: EvLog, Step: 2,
					Message: fmt.Sprintf("  [%d/%d] %q → %q", i+1, len(cfg.Replace), pair.Search, pair.Replace)}
				if err := ResilientReplaceFile(pair.Search, pair.Replace, dumpPath, ReplaceOptions{}); err != nil {
					return fmt.Errorf("replace %q: %w", pair.Search, err)
				}
				// Emit progress for replace step
				p := float64(i+1) / float64(len(cfg.Replace))
				eventCh <- Event{Type: EvProgress, Step: 2, Progress: p}
			}
			return nil
		}},
		{"Before hooks", func() error {
			if skipSQL {
				return nil
			}
			return RunHooks(ctx, cfg, "before", dumpPath, eventCh, 3)
		}},
		{"Import SQL", func() error {
			if skipSQL {
				return nil
			}
			return ImportDump(ctx, cfg, dumpPath, eventCh, 4)
		}},
		{"Between hooks", func() error {
			if skipSQL {
				return nil
			}
			return RunHooks(ctx, cfg, "between", dumpPath, eventCh, 5)
		}},
		{"Sync files", func() error {
			if skipFiles {
				return nil
			}
			return SyncFiles(ctx, cfg, eventCh, 6)
		}},
		{"After hooks", func() error {
			if skipFiles {
				return nil
			}
			return RunHooks(ctx, cfg, "after", dumpPath, eventCh, 7)
		}},
	}

	// Emit connection info
	eventCh <- Event{Type: EvLog, Step: 0,
		Message: fmt.Sprintf("▸ site: %s", confName)}
	if !skipSQL {
		eventCh <- Event{Type: EvLog, Step: 0,
			Message: fmt.Sprintf("▸ source: %s@%s (%s)", cfg.Source.User, cfg.Source.Server, cfg.Source.Type)}
		eventCh <- Event{Type: EvLog, Step: 0,
			Message: fmt.Sprintf("▸ database: %s → %s", cfg.Source.DBName, cfg.Destination.DBName)}
		eventCh <- Event{Type: EvLog, Step: 0,
			Message: fmt.Sprintf("▸ replacements: %d pairs", len(cfg.Replace))}
	}
	if !skipFiles {
		for _, sp := range cfg.Sync {
			eventCh <- Event{Type: EvLog, Step: 0,
				Message: fmt.Sprintf("▸ files: %s → %s", sp.Src, sp.Dst)}
		}
		eventCh <- Event{Type: EvLog, Step: 0,
			Message: fmt.Sprintf("▸ transport: %s", cfg.Transport.Type)}
	}
	eventCh <- Event{Type: EvLog, Step: 0, Message: ""}

	log.Logf("=== sitesync start: %s (op=%v) ===", confName, op)
	syncStart := time.Now()

	for i, step := range steps {
		stepNum := i + 1
		select {
		case <-ctx.Done():
			eventCh <- Event{Type: EvStepFail, Step: stepNum, Message: "cancelled"}
			log.Logf("Step %d cancelled", stepNum)
			return
		default:
		}

		eventCh <- Event{Type: EvStepStart, Step: stepNum}
		log.Logf("Step %d/%d: %s", stepNum, len(steps), step.name)

		stepStart := time.Now()
		if err := step.fn(); err != nil {
			eventCh <- Event{Type: EvStepFail, Step: stepNum, Message: err.Error()}
			log.Logf("Step %d FAILED: %v", stepNum, err)
			return
		}

		elapsed := time.Since(stepStart)
		eventCh <- Event{Type: EvLog, Step: stepNum,
			Message: fmt.Sprintf("  ⏱ %s", formatDuration(elapsed))}
		eventCh <- Event{Type: EvStepDone, Step: stepNum}
		log.Logf("Step %d done (%s)", stepNum, elapsed)
	}

	totalElapsed := time.Since(syncStart)
	eventCh <- Event{Type: EvLog, Step: 0,
		Message: fmt.Sprintf("\n✔ completed in %s", formatDuration(totalElapsed))}

	// Clean up dump file on success.
	if !skipSQL {
		_ = os.Remove(dumpPath)
	}

	log.Logf("=== sitesync done: %s ===", confName)
	eventCh <- Event{Type: EvDone}
}

// RunHeadless runs the engine synchronously without a TUI, printing events
// to stdout. Used with --no-tui flag.
func RunHeadless(ctx context.Context, cfg *config.Config, op Op, log *logger.Logger) error {
	eventCh := make(chan Event, 64)
	go Run(ctx, cfg, op, eventCh, log)

	var lastErr string
	for ev := range eventCh {
		switch ev.Type {
		case EvStepStart:
			fmt.Printf("  ◉ [%d/7] %s ...\n", ev.Step, StepName(ev.Step))
		case EvStepDone:
			fmt.Printf("  ✔ [%d/7] %s done\n", ev.Step, StepName(ev.Step))
		case EvStepFail:
			fmt.Printf("  ✘ [%d/7] %s FAILED: %s\n", ev.Step, StepName(ev.Step), ev.Message)
			lastErr = ev.Message
		case EvProgress:
			fmt.Printf("\r       %3.0f%%", ev.Progress*100)
		case EvLog:
			fmt.Println("    " + ev.Message)
		case EvDone:
			fmt.Println("\n  ✔ sync complete")
		}
	}
	if lastErr != "" {
		return fmt.Errorf("sync failed: %s", lastErr)
	}
	return nil
}

// formatDuration returns a human-readable duration string.
func formatDuration(d time.Duration) string {
	switch {
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	default:
		m := int(d.Minutes())
		s := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm%02ds", m, s)
	}
}

// humanSize returns a human-readable file size.
func humanSize(b int64) string {
	switch {
	case b < 1024:
		return fmt.Sprintf("%d B", b)
	case b < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	case b < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
	default:
		return fmt.Sprintf("%.2f GB", float64(b)/(1024*1024*1024))
	}
}
