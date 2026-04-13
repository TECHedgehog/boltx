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

var (
	purple = lipgloss.Color("#7C3AED")
	muted  = lipgloss.Color("#6B7280")
	white  = lipgloss.Color("#F9FAFB")
	green  = lipgloss.Color("#10B981")

	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(purple)
	subtitleStyle = lipgloss.NewStyle().Foreground(muted)
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(purple)
	normalStyle   = lipgloss.NewStyle().Foreground(white)
	mutedStyle    = lipgloss.NewStyle().Foreground(muted)
	greenStyle    = lipgloss.NewStyle().Foreground(green)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(purple)

	// infoTableBorderStyle wraps the right-column info table.
	// Defined here so viewRight does not allocate a new style every frame.
	infoTableBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(muted)

	sectionStyle = lipgloss.NewStyle().Bold(true).Foreground(white)

	cursorStr   = "› "
	noCursorStr = "  "
	radioOn     = "● "
	radioOff    = "○ "

	// activeTabStyle: open bottom (┘ space └) so the tab visually connects to
	// the content below — like a browser or Office tab.
	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(purple).
			Border(tabBorderWithBottom("┘", " ", "└"), true).
			BorderForeground(purple).
			Padding(0, 1)

	// inactiveTabStyle: closed bottom (┴ ─ ┴) — the tab sits above the baseline.
	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(muted).
				Border(tabBorderWithBottom("┴", "─", "┴"), true).
				BorderForeground(muted).
				Padding(0, 1)
)
