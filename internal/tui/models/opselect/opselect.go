package opselect

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	syncsvc "github.com/carlosrgl/sitesync/internal/sync"
	"github.com/carlosrgl/sitesync/internal/tui/styles"
)

// Messages
type OpChosenMsg struct{ Op syncsvc.Op }
type BackMsg struct{}

type choice struct {
	label string
	desc  string
	op    syncsvc.Op
}

var choices = []choice{
	{"SQL + Files", "Sync the database and all file pairs", syncsvc.OpAll},
	{"SQL only", "Import database dump only (skip rsync/lftp)", syncsvc.OpSQL},
	{"Files only", "Sync files only (skip database)", syncsvc.OpFiles},
}

type keyMap struct {
	Up     key.Binding
	Down   key.Binding
	Select key.Binding
	Back   key.Binding
}

var keys = keyMap{
	Up:     key.NewBinding(key.WithKeys("up", "k")),
	Down:   key.NewBinding(key.WithKeys("down", "j")),
	Select: key.NewBinding(key.WithKeys("enter")),
	Back:   key.NewBinding(key.WithKeys("b", "esc")),
}

type Model struct {
	cursor   int
	confName string
	width    int
	height   int
}

func New(confName string) Model {
	return Model{confName: confName}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, keys.Down):
			if m.cursor < len(choices)-1 {
				m.cursor++
			}
		case key.Matches(msg, keys.Select):
			op := choices[m.cursor].op
			return m, func() tea.Msg { return OpChosenMsg{Op: op} }
		case key.Matches(msg, keys.Back):
			return m, func() tea.Msg { return BackMsg{} }
		}
	}
	return m, nil
}

func (m Model) View() string {
	title := styles.Title.Render("Select operation")
	sub := styles.Subtitle.Render("Site: " + styles.Bold.Render(m.confName))

	var rows []string
	rows = append(rows, title, sub, "")

	for i, c := range choices {
		var indicator, label, desc string
		if i == m.cursor {
			indicator = styles.StepActiveStyle.Render("▶")
			label = styles.SelectedItem.Render(c.label)
			desc = styles.Muted.Render("  " + c.desc)
		} else {
			indicator = "  "
			label = styles.NormalItem.Render(c.label)
			desc = styles.DimItem.Render("  " + c.desc)
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, indicator, " ", label))
		rows = append(rows, desc)
		rows = append(rows, "")
	}

	help := styles.RenderHelp("↑/↓", "navigate", "enter", "confirm", "b", "back")
	footer := styles.StatusBar.Render(help)
	rows = append(rows, footer)

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}
