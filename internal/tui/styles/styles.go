package styles

import "github.com/charmbracelet/lipgloss"

// ── Colour palette ──────────────────────────────────────────────────────────

var (
	ColorPrimary  = lipgloss.Color("#7C8CFF") // indigo
	ColorSuccess  = lipgloss.Color("#73D0A0") // mint green
	ColorWarning  = lipgloss.Color("#FFD866") // warm yellow
	ColorError    = lipgloss.Color("#FF6B6B") // soft red
	ColorMuted    = lipgloss.Color("#555555") // dark grey
	ColorText     = lipgloss.Color("#E0E0E0") // off-white
	ColorSubtext  = lipgloss.Color("#888888") // mid grey
	ColorBorder   = lipgloss.Color("#3A3A3A") // subtle border
	ColorSelected = lipgloss.Color("#7C8CFF") // indigo
	ColorAccent   = lipgloss.Color("#C792EA") // purple
	ColorCyan     = lipgloss.Color("#89DDFF") // cyan
	ColorDimText  = lipgloss.Color("#666666") // dim text
)

// ── Typography ──────────────────────────────────────────────────────────────

var (
	Bold    = lipgloss.NewStyle().Bold(true)
	Muted   = lipgloss.NewStyle().Foreground(ColorMuted)
	Faint   = lipgloss.NewStyle().Faint(true)
	Error   = lipgloss.NewStyle().Foreground(ColorError).Bold(true)
	Success = lipgloss.NewStyle().Foreground(ColorSuccess).Bold(true)
	Warning = lipgloss.NewStyle().Foreground(ColorWarning)
	Primary = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
	Accent  = lipgloss.NewStyle().Foreground(ColorAccent)
	Cyan    = lipgloss.NewStyle().Foreground(ColorCyan)
)

// ── Layout ──────────────────────────────────────────────────────────────────

var (
	App = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(0, 1)

	Panel = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(0, 1)

	Title = lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Bold(true).
		Padding(0, 0, 1, 0)

	Subtitle = lipgloss.NewStyle().
			Foreground(ColorSubtext).
			Padding(0, 0, 1, 0)

	StatusBar = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Padding(0, 1)

	SelectedItem = lipgloss.NewStyle().
			Foreground(ColorSelected).
			Bold(true)

	NormalItem = lipgloss.NewStyle().
			Foreground(ColorText)

	DimItem = lipgloss.NewStyle().
		Foreground(ColorMuted)

	LogLine = lipgloss.NewStyle().
		Foreground(ColorSubtext)

	LogPanel = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(ColorBorder).
			Padding(0, 0)
)

// ── Step indicators ─────────────────────────────────────────────────────────

const (
	StepPending = "  ○"
	StepActive  = "  ◉"
	StepDone    = "  ✔"
	StepFailed  = "  ✘"
	StepSkipped = "  ⊘"
)

var (
	StepPendingStyle = lipgloss.NewStyle().Foreground(ColorMuted)
	StepActiveStyle  = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
	StepDoneStyle    = lipgloss.NewStyle().Foreground(ColorSuccess).Bold(true)
	StepFailedStyle  = lipgloss.NewStyle().Foreground(ColorError).Bold(true)
	StepSkippedStyle = lipgloss.NewStyle().Foreground(ColorMuted).Faint(true)
)

// ── Log line prefixes ───────────────────────────────────────────────────────

var (
	LogPrefixCmd  = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
	LogPrefixInfo = lipgloss.NewStyle().Foreground(ColorAccent)
	LogPrefixData = lipgloss.NewStyle().Foreground(ColorWarning)
	LogPrefixDim  = lipgloss.NewStyle().Foreground(ColorDimText)
)

// ── Help footer ─────────────────────────────────────────────────────────────

var (
	HelpKey  = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
	HelpDesc = lipgloss.NewStyle().Foreground(ColorMuted)
	HelpSep  = lipgloss.NewStyle().Foreground(ColorBorder)
)

func RenderHelp(pairs ...string) string {
	var s string
	for i := 0; i < len(pairs)-1; i += 2 {
		if s != "" {
			s += HelpSep.Render("  │  ")
		}
		s += HelpKey.Render(pairs[i]) + " " + HelpDesc.Render(pairs[i+1])
	}
	return s
}
