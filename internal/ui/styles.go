package ui

import "github.com/charmbracelet/lipgloss"

// tabBorderWithBottom returns a RoundedBorder whose three bottom characters
// are replaced. This lets the active tab appear "open" at the bottom (browser
// / Office style) while inactive tabs have a normal closed bottom.
func tabBorderWithBottom(left, middle, right string) lipgloss.Border {
	b := lipgloss.RoundedBorder()
	b.BottomLeft = left
	b.Bottom = middle
	b.BottomRight = right
	return b
}

// Theme holds the four colors that define a visual theme.
// To add a new theme, append a Theme{} literal to the Themes slice below.
type Theme struct {
	Name    string
	Accent  lipgloss.Color // primary accent: titles, selected items, borders
	Muted   lipgloss.Color // secondary text: hints, descriptions, inactive
	Text    lipgloss.Color // default text content
	Success lipgloss.Color // suggestions / positive indicators
}

// Themes is the ordered list of available themes. Press 't' to cycle through them.
var Themes = []Theme{
	{Name: "Purple", Accent: "#7C3AED", Muted: "#6B7280", Text: "#F9FAFB", Success: "#10B981"},
	{Name: "Teal",   Accent: "#0D9488", Muted: "#6B7280", Text: "#F9FAFB", Success: "#FBBF24"},
	{Name: "Amber",  Accent: "#D97706", Muted: "#6B7280", Text: "#F9FAFB", Success: "#10B981"},
}

// applyTheme reassigns all style vars to match theme t.
// Called once at init and again each time the user presses 't'.
func applyTheme(t Theme) {
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(t.Accent)
	subtitleStyle = lipgloss.NewStyle().Foreground(t.Muted)
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(t.Accent)
	normalStyle   = lipgloss.NewStyle().Foreground(t.Text)
	mutedStyle    = lipgloss.NewStyle().Foreground(t.Muted)
	greenStyle    = lipgloss.NewStyle().Foreground(t.Success)
	boxStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Accent)
	infoTableBorderStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Muted)
	activeTabStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Accent).
		Border(tabBorderWithBottom("┘", " ", "└"), true).
		BorderForeground(t.Accent).
		Padding(0, 1)
}

func init() {
	applyTheme(Themes[0])
}

// Style vars — reassigned by applyTheme, read by view functions.
var (
	titleStyle           lipgloss.Style
	subtitleStyle        lipgloss.Style
	selectedStyle        lipgloss.Style
	normalStyle          lipgloss.Style
	mutedStyle           lipgloss.Style
	greenStyle           lipgloss.Style
	boxStyle             lipgloss.Style
	infoTableBorderStyle lipgloss.Style
	activeTabStyle       lipgloss.Style

	cursorStr          = "› "
	noCursorStr        = "  "
	radioOn            = "● "
	radioOff           = "○ "
	kindTextInputMarker = "▶ " // 2-cell width, same as radioOn/radioOff
	kindSelectMarker    = "≡ " // 2-cell width, same as radioOn/radioOff
)
