package ui

import (
	"time"

	"github.com/charmbracelet/lipgloss"
)

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

// Theme holds the colors that define a visual theme.
// To add a new theme, append a Theme{} literal to the Themes slice below.
type Theme struct {
	Name    string
	Accent  lipgloss.Color // primary accent: titles, selected items, borders
	Dim     lipgloss.Color // darkened accent: noise/glitch chars on splash screen
	Muted   lipgloss.Color // secondary text: hints, descriptions, inactive
	Text    lipgloss.Color // default text content
	Success lipgloss.Color // suggestions / positive indicators
	Queued  lipgloss.Color // pending/queued changes — must not repeat any other slot in this theme
}

// Themes is the ordered list of available themes. Press 't' to cycle through them.
var Themes = []Theme{
	{Name: "Purple", Accent: "#7C3AED", Dim: "#3B1A7A", Muted: "#6B7280", Text: "#F9FAFB", Success: "#10B981", Queued: "#F59E0B"},
	{Name: "Teal",   Accent: "#0D9488", Dim: "#134E4A", Muted: "#6B7280", Text: "#F9FAFB", Success: "#FBBF24", Queued: "#F97316"},
	{Name: "Amber",  Accent: "#D97706", Dim: "#78350F", Muted: "#6B7280", Text: "#F9FAFB", Success: "#10B981", Queued: "#FBBF24"},
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
	queuedStyle = lipgloss.NewStyle().Foreground(t.Queued)
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

// ── layout & timing ──────────────────────────────────────────────────────────
// These are the values most likely to need tuning — change them here.
const (
	visibleItems = 5 // rows shown in the KindSelect inline picker

	// Left column width bounds (chars). Content-driven; capped/floored by these.
	leftColMaxW = 35
	leftColMinW = 28

	// Worst-case box height used to compute a stable vertical anchor.
	// Cover the tallest tab (PKG, ~15 lines) + chrome (pad + separator + hints + border).
	tallestBoxH = 24

	// Width of KindTextInput fields and the fallback column width before layout is computed.
	optionInputWidth = 40

	// How long a bell flash stays visible before clearing.
	bellClearDur = 150 * time.Millisecond
)

// Style vars — reassigned by applyTheme, read by view functions.
var (
	titleStyle           lipgloss.Style
	subtitleStyle        lipgloss.Style
	selectedStyle        lipgloss.Style
	normalStyle          lipgloss.Style
	mutedStyle           lipgloss.Style
	greenStyle           lipgloss.Style
	queuedStyle          lipgloss.Style
	boxStyle             lipgloss.Style
	infoTableBorderStyle lipgloss.Style
	activeTabStyle       lipgloss.Style

	// Fixed styles — not part of the theme palette.
	errorStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444"))
	pendingRemoveStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Strikethrough(true)

	cursorStr           = "› "
	noCursorStr         = "  "
	radioOn             = "● "
	radioOff            = "○ "
	kindTextInputMarker = "▶ " // 2-cell width, same as radioOn/radioOff
	kindSelectMarker    = "≡ " // 2-cell width, same as radioOn/radioOff
	kindCycleMarker     = "↻ " // 2-cell width, same as radioOn/radioOff
	kindListCollapsed   = "▸ " // expandable list, collapsed
	kindListExpanded    = "▾ " // expandable list, expanded
)
