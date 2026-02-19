package syncing

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/carlosrgl/sitesync/internal/config"
	"github.com/carlosrgl/sitesync/internal/logger"
	syncsvc "github.com/carlosrgl/sitesync/internal/sync"
	"github.com/carlosrgl/sitesync/internal/tui/styles"
)

const maxLogs = 500

// Messages
type SyncDoneMsg struct{ Err string }
type BackMsg struct{}

type stepStatus uint8

const (
	statusPending stepStatus = iota
	statusActive
	statusDone
	statusFailed
	statusSkipped
)

type stepState struct {
	status   stepStatus
	progress float64
}

type Model struct {
	cfg      *config.Config
	op       syncsvc.Op
	confName string

	eventCh    <-chan syncsvc.Event
	cancelFn   context.CancelFunc
	steps      [8]stepState // index 1-7; index 0 unused
	logs       []string
	viewport   viewport.Model
	spinner    spinner.Model
	progressBr progress.Model
	logVisible bool
	done       bool
	failed     bool
	failMsg    string
	width      int
	height     int
}

func New(cfg *config.Config, op syncsvc.Op, confName string, log *logger.Logger) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = styles.StepActiveStyle

	pr := progress.New(
		progress.WithDefaultGradient(),
		progress.WithoutPercentage(),
	)

	vp := viewport.New(80, 10)
	vp.Style = styles.LogPanel

	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan syncsvc.Event, 128)

	go syncsvc.Run(ctx, cfg, op, ch, log)

	return Model{
		cfg:        cfg,
		op:         op,
		confName:   confName,
		eventCh:    ch,
		cancelFn:   cancel,
		spinner:    sp,
		progressBr: pr,
		viewport:   vp,
		logVisible: true,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		waitForEvent(m.eventCh),
	)
}

func waitForEvent(ch <-chan syncsvc.Event) tea.Cmd {
	return func() tea.Msg {
		e, ok := <-ch
		if !ok {
			return syncsvc.Event{Type: syncsvc.EvDone}
		}
		return e
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width - 4
		logH := msg.Height - 14 // header(3) + steps(~7) + separator(1) + padding(3)
		if logH < 4 {
			logH = 4
		}
		m.viewport.Height = logH

	case syncsvc.Event:
		m = m.applyEvent(msg)
		if !m.done && !m.failed {
			cmds = append(cmds, waitForEvent(m.eventCh))
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case progress.FrameMsg:
		pm, cmd := m.progressBr.Update(msg)
		m.progressBr = pm.(progress.Model)
		cmds = append(cmds, cmd)

	case tea.KeyMsg:
		switch msg.String() {
		case "l":
			m.logVisible = !m.logVisible
		case "q", "ctrl+c":
			if m.done || m.failed {
				return m, func() tea.Msg { return BackMsg{} }
			}
			// Abort: cancel the context
			m.cancelFn()
		}
	}

	// Update viewport
	var vcmd tea.Cmd
	m.viewport, vcmd = m.viewport.Update(msg)
	cmds = append(cmds, vcmd)

	return m, tea.Batch(cmds...)
}

func (m Model) applyEvent(ev syncsvc.Event) Model {
	switch ev.Type {
	case syncsvc.EvStepStart:
		if ev.Step >= 1 && ev.Step <= 7 {
			m.steps[ev.Step].status = statusActive
		}
	case syncsvc.EvStepDone:
		if ev.Step >= 1 && ev.Step <= 7 {
			m.steps[ev.Step].status = statusDone
			m.steps[ev.Step].progress = 1.0
		}
	case syncsvc.EvStepFail:
		if ev.Step >= 1 && ev.Step <= 7 {
			m.steps[ev.Step].status = statusFailed
		}
		m.failed = true
		m.failMsg = ev.Message
	case syncsvc.EvProgress:
		if ev.Step >= 1 && ev.Step <= 7 {
			m.steps[ev.Step].progress = ev.Progress
		}
	case syncsvc.EvLog:
		styled := styleLogLine(ev.Message)
		m.logs = append(m.logs, styled)
		if len(m.logs) > maxLogs {
			m.logs = m.logs[len(m.logs)-maxLogs:]
		}
		m.viewport.SetContent(strings.Join(m.logs, "\n"))
		m.viewport.GotoBottom()
	case syncsvc.EvDone:
		m.done = true
	}
	return m
}

// styleLogLine applies pretty formatting to log lines based on content.
func styleLogLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}

	// Commands: lines containing $ command invocations
	if strings.Contains(trimmed, "$ ") {
		return styles.LogPrefixCmd.Render("  $ ") + styles.Cyan.Render(strings.TrimPrefix(trimmed, "$ "))
	}

	// Info headers: lines starting with ▸
	if strings.HasPrefix(trimmed, "▸") {
		parts := strings.SplitN(trimmed[len("▸"):], ":", 2)
		if len(parts) == 2 {
			return styles.LogPrefixInfo.Render("  ▸ "+strings.TrimSpace(parts[0])+":") +
				styles.NormalItem.Render(parts[1])
		}
		return styles.LogPrefixInfo.Render("  " + trimmed)
	}

	// Completion: ✔
	if strings.HasPrefix(trimmed, "✔") {
		return styles.Success.Render("  " + trimmed)
	}

	// Timing: ⏱
	if strings.Contains(trimmed, "⏱") {
		return styles.LogPrefixDim.Render("  " + trimmed)
	}

	// Numbered pairs: [1/3] replace operations
	if len(trimmed) > 0 && trimmed[0] == '[' && strings.Contains(trimmed, "/") {
		bracket := strings.Index(trimmed, "]")
		if bracket > 0 {
			return styles.LogPrefixInfo.Render("  "+trimmed[:bracket+1]) +
				styles.LogLine.Render(trimmed[bracket+1:])
		}
	}

	// Data: source/target/dump/processing lines
	for _, kw := range []string{"source:", "target:", "dump:", "dump size:", "port:", "host:", "processing"} {
		if strings.Contains(trimmed, kw) {
			return styles.LogPrefixData.Render("  " + trimmed)
		}
	}

	// Default: dim log line
	return styles.LogPrefixDim.Render("  " + trimmed)
}

func (m Model) View() string {
	var rows []string

	// ── Header ──────────────────────────────────────────
	title := styles.Title.Render(fmt.Sprintf("⚡ Syncing: %s", m.confName))
	rows = append(rows, title)

	// ── Steps ───────────────────────────────────────────
	for i := 1; i <= 7; i++ {
		rows = append(rows, m.renderStep(i))
	}

	rows = append(rows, "")

	// ── Status ──────────────────────────────────────────
	if m.failed {
		rows = append(rows, styles.Error.Render("  ✘ "+m.failMsg))
		rows = append(rows, "")
	}
	if m.done {
		done := countSteps(m.steps[:], statusDone)
		skipped := countSteps(m.steps[:], statusSkipped)
		summary := fmt.Sprintf("  ✔ Sync complete  (%d steps done", done)
		if skipped > 0 {
			summary += fmt.Sprintf(", %d skipped", skipped)
		}
		summary += ")"
		rows = append(rows, styles.Success.Render(summary))
		rows = append(rows, "")
	}

	// ── Logs ────────────────────────────────────────────
	if m.logVisible {
		logHeader := styles.Muted.Render("  ── logs ") +
			styles.Muted.Render(strings.Repeat("─", max(0, m.width-14)))
		rows = append(rows, logHeader)
		rows = append(rows, m.viewport.View())
		rows = append(rows, "")
	}

	// ── Help ────────────────────────────────────────────
	var helpPairs []string
	helpPairs = append(helpPairs, "l", "toggle log")
	if m.done || m.failed {
		helpPairs = append(helpPairs, "q", "back")
	} else {
		helpPairs = append(helpPairs, "q", "abort")
	}
	rows = append(rows, styles.StatusBar.Render(styles.RenderHelp(helpPairs...)))

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func countSteps(steps []stepState, status stepStatus) int {
	n := 0
	for _, s := range steps {
		if s.status == status {
			n++
		}
	}
	return n
}

func (m Model) renderStep(i int) string {
	st := m.steps[i]
	name := syncsvc.StepName(i)
	num := fmt.Sprintf("%d", i)

	var indicator, styledName, right string

	switch st.status {
	case statusPending:
		indicator = styles.StepPendingStyle.Render(styles.StepPending)
		styledName = styles.Muted.Render(fmt.Sprintf(" %s  %-20s", num, name))
		right = styles.Muted.Render("·")
	case statusActive:
		indicator = styles.StepActiveStyle.Render(styles.StepActive)
		styledName = styles.Primary.Render(fmt.Sprintf(" %s  %-20s", num, name))
		if st.progress > 0 {
			barWidth := m.width - 42
			if barWidth < 10 {
				barWidth = 10
			}
			if barWidth > 40 {
				barWidth = 40
			}
			m.progressBr.Width = barWidth
			pct := st.progress * 100
			right = m.progressBr.ViewAs(st.progress) +
				styles.Cyan.Render(fmt.Sprintf(" %3.0f%%", pct))
		} else {
			right = m.spinner.View()
		}
	case statusDone:
		indicator = styles.StepDoneStyle.Render(styles.StepDone)
		styledName = styles.NormalItem.Render(fmt.Sprintf(" %s  %-20s", num, name))
		right = styles.Success.Render("done")
	case statusFailed:
		indicator = styles.StepFailedStyle.Render(styles.StepFailed)
		styledName = styles.Error.Render(fmt.Sprintf(" %s  %-20s", num, name))
		right = styles.Error.Render("FAILED")
	case statusSkipped:
		indicator = styles.StepSkippedStyle.Render(styles.StepSkipped)
		styledName = styles.Muted.Render(fmt.Sprintf(" %s  %-20s", num, name))
		right = styles.Muted.Render("skip")
	}

	return indicator + styledName + right
}
