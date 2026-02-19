package picker

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/carlosrgl/sitesync/internal/config"
	"github.com/carlosrgl/sitesync/internal/tui/styles"
)

// Messages emitted by this model
type ConfSelectedMsg struct{ Name string }
type NewConfMsg struct{}
type EditConfMsg struct{ Name string }

// item implements list.Item
type item struct {
	entry config.ConfigEntry
}

func (i item) Title() string { return i.entry.Name }
func (i item) Description() string {
	if i.entry.LastModified.IsZero() {
		return "never synced"
	}
	age := time.Since(i.entry.LastModified)
	switch {
	case age < time.Minute:
		return "just now"
	case age < time.Hour:
		return fmt.Sprintf("%dm ago", int(age.Minutes()))
	case age < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(age.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(age.Hours()/24))
	}
}
func (i item) FilterValue() string { return i.entry.Name }

// key bindings
type keyMap struct {
	New    key.Binding
	Edit   key.Binding
	Select key.Binding
	Search key.Binding
	Quit   key.Binding
}

var keys = keyMap{
	New: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "new site"),
	),
	Edit: key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "edit"),
	),
	Select: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select"),
	),
	Search: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "search"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
}

// Model is the site-picker screen.
type Model struct {
	list   list.Model
	width  int
	height int
}

// New creates a picker populated with available configs.
func New(entries []config.ConfigEntry) Model {
	items := make([]list.Item, len(entries))
	for i, e := range entries {
		items[i] = item{entry: e}
	}

	d := list.NewDefaultDelegate()
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.
		Foreground(styles.ColorSelected).
		BorderLeftForeground(styles.ColorSelected)
	d.Styles.SelectedDesc = d.Styles.SelectedDesc.
		Foreground(styles.ColorMuted).
		BorderLeftForeground(styles.ColorSelected)

	l := list.New(items, d, 60, 20)
	l.Title = "sitesync"
	l.Styles.Title = lipgloss.NewStyle().
		Foreground(styles.ColorPrimary).
		Bold(true).
		Padding(0, 1)
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{keys.Search, keys.New, keys.Edit}
	}
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)

	return Model{list: l}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width-4, msg.Height-4)
		return m, nil

	case tea.KeyMsg:
		// Don't capture keys when the list is filtering
		if m.list.FilterState() == list.Filtering {
			break
		}
		switch {
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, keys.New):
			return m, func() tea.Msg { return NewConfMsg{} }
		case key.Matches(msg, keys.Edit):
			if it, ok := m.list.SelectedItem().(item); ok {
				return m, func() tea.Msg { return EditConfMsg{Name: it.entry.Name} }
			}
		case key.Matches(msg, keys.Select):
			if it, ok := m.list.SelectedItem().(item); ok {
				return m, func() tea.Msg { return ConfSelectedMsg{Name: it.entry.Name} }
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	help := styles.RenderHelp(
		"/", "search",
		"enter", "sync",
		"e", "edit",
		"n", "new",
		"q", "quit",
	)
	footer := styles.StatusBar.Render(help)
	return lipgloss.JoinVertical(lipgloss.Left, m.list.View(), footer)
}

// Reload replaces the list items (call after creating/editing a config).
func (m *Model) Reload(entries []config.ConfigEntry) {
	items := make([]list.Item, len(entries))
	for i, e := range entries {
		items[i] = item{entry: e}
	}
	m.list.SetItems(items)
}
