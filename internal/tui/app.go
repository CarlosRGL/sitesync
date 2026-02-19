package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/carlosrgl/sitesync/internal/config"
	"github.com/carlosrgl/sitesync/internal/logger"
	syncsvc "github.com/carlosrgl/sitesync/internal/sync"
	"github.com/carlosrgl/sitesync/internal/tui/models/editor"
	"github.com/carlosrgl/sitesync/internal/tui/models/opselect"
	"github.com/carlosrgl/sitesync/internal/tui/models/picker"
	"github.com/carlosrgl/sitesync/internal/tui/models/syncing"
	"github.com/carlosrgl/sitesync/internal/tui/styles"
)

type screen int

const (
	screenPicker screen = iota
	screenOpSelect
	screenSyncing
	screenEditor
)

// AppModel is the root Bubble Tea model. It routes all messages and renders
// to whichever sub-model is currently active.
type AppModel struct {
	screen  screen
	picker  picker.Model
	opsel   opselect.Model
	syncing syncing.Model
	editor  editor.Model
	log     logger.Logger

	// Transient state between screens
	selectedConf string

	width  int
	height int
}

// New creates an initialised AppModel, pre-selecting confName if non-empty.
func New(entries []config.ConfigEntry, preselect string, log logger.Logger) AppModel {
	m := AppModel{
		screen: screenPicker,
		picker: picker.New(entries),
		log:    log,
	}
	// If a config name was passed on the CLI, skip straight to op-select.
	if preselect != "" {
		m.selectedConf = preselect
		m.opsel = opselect.New(preselect)
		m.screen = screenOpSelect
	}
	return m
}

func (m AppModel) Init() tea.Cmd {
	return m.activeInit()
}

func (m AppModel) activeInit() tea.Cmd {
	switch m.screen {
	case screenPicker:
		return m.picker.Init()
	case screenOpSelect:
		return m.opsel.Init()
	case screenSyncing:
		return m.syncing.Init()
	case screenEditor:
		return m.editor.Init()
	}
	return nil
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Window size is forwarded to all sub-models.
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = wsm.Width
		m.height = wsm.Height
	}

	switch m.screen {
	case screenPicker:
		return m.updatePicker(msg)
	case screenOpSelect:
		return m.updateOpSelect(msg)
	case screenSyncing:
		return m.updateSyncing(msg)
	case screenEditor:
		return m.updateEditor(msg)
	}
	return m, nil
}

// ── per-screen update handlers ───────────────────────────────────────────────

func (m AppModel) updatePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case picker.ConfSelectedMsg:
		name := msg.(picker.ConfSelectedMsg).Name
		m.selectedConf = name
		m.opsel = opselect.New(name)
		m.screen = screenOpSelect
		return m, m.opsel.Init()

	case picker.NewConfMsg:
		m.editor = editor.New("new-site", nil)
		m.screen = screenEditor
		return m, m.editor.Init()

	case picker.EditConfMsg:
		name := msg.(picker.EditConfMsg).Name
		cfg, err := config.Load(name)
		if err != nil {
			// Show an error by staying on picker (could surface error in a future toast)
			return m, nil
		}
		m.editor = editor.New(name, cfg)
		m.screen = screenEditor
		return m, m.editor.Init()
	}

	sub, cmd := m.picker.Update(msg)
	m.picker = sub.(picker.Model)
	return m, cmd
}

func (m AppModel) updateOpSelect(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch ev := msg.(type) {
	case opselect.OpChosenMsg:
		cfg, err := config.Load(m.selectedConf)
		if err != nil {
			// Return to picker on error
			m.screen = screenPicker
			return m, nil
		}
		logFile := config.LogFile(cfg)
		log, err := logger.New(logFile)
		if err != nil {
			log = logger.Discard()
		}
		m.syncing = syncing.New(cfg, ev.Op, m.selectedConf, log)
		m.screen = screenSyncing
		return m, m.syncing.Init()

	case opselect.BackMsg:
		m.screen = screenPicker
		return m, m.picker.Init()
	}

	sub, cmd := m.opsel.Update(msg)
	m.opsel = sub.(opselect.Model)
	return m, cmd
}

func (m AppModel) updateSyncing(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case syncing.BackMsg:
		m.screen = screenPicker
		return m, m.picker.Init()
	}

	sub, cmd := m.syncing.Update(msg)
	m.syncing = sub.(syncing.Model)
	return m, cmd
}

func (m AppModel) updateEditor(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch ev := msg.(type) {
	case editor.DoneMsg:
		// Reload picker entries after save
		entries, _ := config.ListConfigs()
		m.picker.Reload(entries)

		if ev.Saved {
			// After saving, jump straight to op-select for the new/edited site
			m.selectedConf = ev.ConfName
			m.opsel = opselect.New(ev.ConfName)
			m.screen = screenOpSelect
			return m, m.opsel.Init()
		}
		m.screen = screenPicker
		return m, m.picker.Init()
	}

	sub, cmd := m.editor.Update(msg)
	m.editor = sub.(editor.Model)
	return m, cmd
}

// ── View ─────────────────────────────────────────────────────────────────────

func (m AppModel) View() string {
	header := styles.Primary.Render("sitesync") +
		styles.Muted.Render("  ·  website sync tool")

	var body string
	switch m.screen {
	case screenPicker:
		body = m.picker.View()
	case screenOpSelect:
		body = m.opsel.View()
	case screenSyncing:
		body = m.syncing.View()
	case screenEditor:
		body = m.editor.View()
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		"",
		body,
	)
}

// ── Op helper exposed for headless use ───────────────────────────────────────

// ParseOp converts a string (sql/files/"") to an Op.
func ParseOp(s string) syncsvc.Op {
	switch s {
	case "sql":
		return syncsvc.OpSQL
	case "files":
		return syncsvc.OpFiles
	default:
		return syncsvc.OpAll
	}
}
