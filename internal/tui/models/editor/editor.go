package editor

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/carlosrgl/sitesync/internal/config"
	"github.com/carlosrgl/sitesync/internal/tui/styles"
)

// DoneMsg is sent when the editor finishes (saved=true) or is cancelled.
type DoneMsg struct {
	Saved    bool
	ConfName string
}

// Model wraps a huh form for editing/creating a site config.
type Model struct {
	form     *huh.Form
	cfg      config.Config
	confName string
	isNew    bool
	err      error
	width    int
	height   int

	// Form field values (intermediary string versions)
	portStr    string
	lftpPortStr string
	replaceText string // "search==>replace" lines
	syncText    string // "src==>dst" lines
	excludeText string // one per line
	ignoreText  string // one per line
}

// New creates an editor pre-populated from an existing config.
// If cfg is nil, a default config is used (new site mode).
func New(confName string, cfg *config.Config) Model {
	isNew := cfg == nil
	m := Model{
		confName: confName,
		isNew:    isNew,
	}
	if cfg != nil {
		m.cfg = *cfg
	} else {
		m.cfg = config.DefaultConfig()
		m.cfg.Site.Name = confName
	}
	m.portStr = fmt.Sprintf("%d", m.cfg.Source.Port)
	if m.cfg.Transport.LFTP.Port == 0 {
		m.cfg.Transport.LFTP.Port = 21
	}
	m.lftpPortStr = fmt.Sprintf("%d", m.cfg.Transport.LFTP.Port)
	m.replaceText = replacePairsToText(m.cfg.Replace)
	m.syncText = syncPairsToText(m.cfg.Sync)
	m.excludeText = strings.Join(m.cfg.Transport.Exclude, "\n")
	m.ignoreText = strings.Join(m.cfg.Database.IgnoreTables, "\n")

	m.form = m.buildForm()
	return m
}

func (m *Model) buildForm() *huh.Form {
	cfg := &m.cfg

	return huh.NewForm(
		// Page 1: Basic + Source
		huh.NewGroup(
			huh.NewInput().
				Title("Config name").
				Description("Identifier used in etc/{name}/").
				Value(&m.confName),
			huh.NewInput().
				Title("Site description").
				Value(&cfg.Site.Description),
			huh.NewInput().
				Title("Remote server").
				Description("SSH / rsync hostname").
				Value(&cfg.Source.Server),
			huh.NewInput().
				Title("SSH user").
				Value(&cfg.Source.User),
			huh.NewInput().
				Title("SSH port").
				Value(&m.portStr),
			huh.NewSelect[string]().
				Title("Source type").
				Options(
					huh.NewOption("Remote DB dump (SSH)", "remote_base"),
					huh.NewOption("Local DB dump", "local_base"),
					huh.NewOption("Remote SQL file", "remote_file"),
					huh.NewOption("Local SQL file", "local_file"),
				).
				Value(&cfg.Source.Type),
			huh.NewInput().
				Title("Remote files root").
				Description("Absolute path on the remote server").
				Value(&cfg.Source.FilesRoot),
			huh.NewInput().
				Title("Remote site URL").
				Description("e.g. https://www.example.com").
				Value(&cfg.Source.SiteHost),
		),
		// Page 2: Source DB
		huh.NewGroup(
			huh.NewNote().
				Title("Source database credentials").
				Description("Leave blank to use ~/.my.cnf"),
			huh.NewInput().Title("DB hostname").Value(&cfg.Source.DBHostname),
			huh.NewInput().Title("DB name").Value(&cfg.Source.DBName),
			huh.NewInput().Title("DB user").Value(&cfg.Source.DBUser),
			huh.NewInput().Title("DB password").
				EchoMode(huh.EchoModePassword).
				Value(&cfg.Source.DBPassword),
		),
		// Page 3: Destination
		huh.NewGroup(
			huh.NewNote().
				Title("Local (destination) settings"),
			huh.NewInput().
				Title("Local files root").
				Description("Absolute path on this machine").
				Value(&cfg.Destination.FilesRoot),
			huh.NewInput().
				Title("Local site URL").
				Description("e.g. http://mysite.local").
				Value(&cfg.Destination.SiteHost),
			huh.NewInput().Title("Local DB hostname").Value(&cfg.Destination.DBHostname),
			huh.NewInput().Title("Local DB name").Value(&cfg.Destination.DBName),
			huh.NewInput().Title("Local DB user").Value(&cfg.Destination.DBUser),
			huh.NewInput().Title("Local DB password").
				EchoMode(huh.EchoModePassword).
				Value(&cfg.Destination.DBPassword),
		),
		// Page 4: Find/Replace pairs
		huh.NewGroup(
			huh.NewNote().
				Title("Find / Replace pairs").
				Description("One pair per line in the format:\n  search==>replace\nApplied to the SQL dump in order."),
			huh.NewText().
				Title("Replace pairs").
				Value(&m.replaceText),
		),
		// Page 5: File sync + Transport
		huh.NewGroup(
			huh.NewNote().
				Title("File synchronisation"),
			huh.NewText().
				Title("Sync pairs (src==>dst)").
				Description("One pair per line: /remote/path==>/local/path").
				Value(&m.syncText),
			huh.NewSelect[string]().
				Title("Transport").
				Options(
					huh.NewOption("rsync", "rsync"),
					huh.NewOption("lftp", "lftp"),
				).
				Value(&cfg.Transport.Type),
			huh.NewInput().
				Title("rsync options").
				Value(&cfg.Transport.RsyncOptions),
			huh.NewText().
				Title("Exclude patterns (one per line)").
				Value(&m.excludeText),
		),
		// Page 6: Advanced / DB options
		huh.NewGroup(
			huh.NewNote().
				Title("Advanced options"),
			huh.NewInput().
				Title("mysqldump structure options").
				Value(&cfg.Database.SQLOptionsStructure),
			huh.NewInput().
				Title("mysqldump extra options").
				Value(&cfg.Database.SQLOptionsExtra),
			huh.NewText().
				Title("Ignored tables (one per line)").
				Value(&m.ignoreText),
			huh.NewInput().
				Title("Log file").
				Value(&cfg.Logging.File),
		),
	).WithShowHelp(true).WithShowErrors(true)
}

func (m Model) Init() tea.Cmd {
	return m.form.Init()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = msg.Width
		m.height = msg.Height
	}

	form, cmd := m.form.Update(msg)
	m.form = form.(*huh.Form)

	switch m.form.State {
	case huh.StateCompleted:
		// Flush intermediary strings back into the config struct.
		m.flushFields()
		if err := config.Save(m.confName, &m.cfg); err != nil {
			m.err = err
			return m, func() tea.Msg { return DoneMsg{Saved: false, ConfName: m.confName} }
		}
		return m, func() tea.Msg { return DoneMsg{Saved: true, ConfName: m.confName} }

	case huh.StateAborted:
		return m, func() tea.Msg { return DoneMsg{Saved: false, ConfName: m.confName} }
	}

	return m, cmd
}

func (m *Model) flushFields() {
	// Parse port strings
	var port int
	fmt.Sscan(m.portStr, &port)
	if port == 0 {
		port = 22
	}
	m.cfg.Source.Port = port

	var lftpPort int
	fmt.Sscan(m.lftpPortStr, &lftpPort)
	if lftpPort == 0 {
		lftpPort = 21
	}
	m.cfg.Transport.LFTP.Port = lftpPort

	m.cfg.Site.Name = m.confName
	m.cfg.Replace = textToReplacePairs(m.replaceText)
	m.cfg.Sync = textToSyncPairs(m.syncText)
	m.cfg.Transport.Exclude = textToLines(m.excludeText)
	m.cfg.Database.IgnoreTables = textToLines(m.ignoreText)
}

func (m Model) View() string {
	title := styles.Title.Render("Configure site")
	var parts []string
	if m.isNew {
		parts = append(parts, title, styles.Subtitle.Render("New configuration"), "")
	} else {
		parts = append(parts, title, styles.Subtitle.Render("Editing: "+m.confName), "")
	}
	parts = append(parts, m.form.View())
	if m.err != nil {
		parts = append(parts, "", styles.Error.Render("Error: "+m.err.Error()))
	}
	return strings.Join(parts, "\n")
}

// ── text ↔ pair helpers ──────────────────────────────────────────────────────

func replacePairsToText(pairs []config.ReplacePair) string {
	lines := make([]string, len(pairs))
	for i, p := range pairs {
		lines[i] = p.Search + "==>" + p.Replace
	}
	return strings.Join(lines, "\n")
}

func textToReplacePairs(text string) []config.ReplacePair {
	var pairs []config.ReplacePair
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if idx := strings.Index(line, "==>"); idx >= 0 {
			pairs = append(pairs, config.ReplacePair{
				Search:  line[:idx],
				Replace: line[idx+3:],
			})
		}
	}
	return pairs
}

func syncPairsToText(pairs []config.SyncPair) string {
	lines := make([]string, len(pairs))
	for i, p := range pairs {
		lines[i] = p.Src + "==>" + p.Dst
	}
	return strings.Join(lines, "\n")
}

func textToSyncPairs(text string) []config.SyncPair {
	var pairs []config.SyncPair
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if idx := strings.Index(line, "==>"); idx >= 0 {
			pairs = append(pairs, config.SyncPair{
				Src: line[:idx],
				Dst: line[idx+3:],
			})
		}
	}
	return pairs
}

func textToLines(text string) []string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}
