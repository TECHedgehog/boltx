package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Model holds all TUI state.
type Model struct {
	width  int
	height int
	cursor int
}

var menuItems = []string{
	"Start setup",
	"Quit",
}

// NewModel returns the initial model.
func NewModel() Model {
	return Model{}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(menuItems)-1 {
				m.cursor++
			}
		case "enter", " ":
			switch m.cursor {
			case 0: // Start setup — will navigate to Stage 2 next
			case 1:
				return m, tea.Quit
			}
		}
	}

	return m, nil
}

func (m Model) View() string {
	content := m.viewMenu()
	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
	}
	return content
}

func (m Model) viewMenu() string {
	var b strings.Builder

	// find the widest item label (no cursor prefix) so the title
	// centers over the menu text, not the full block width
	maxItemWidth := 0
	for _, item := range menuItems {
		if w := lipgloss.Width(normalStyle.Render(item)); w > maxItemWidth {
			maxItemWidth = w
		}
	}

	b.WriteString(noCursorStr + titleStyle.Width(maxItemWidth).Align(lipgloss.Center).Render("boltx") + "\n")
	b.WriteString(noCursorStr + subtitleStyle.Render("Linux setup tool") + "\n\n")

	for i, item := range menuItems {
		cursor := noCursorStr
		style := normalStyle
		if m.cursor == i {
			cursor = cursorStr
			style = selectedStyle
		}
		b.WriteString(cursor + style.Render(item) + "\n")
	}

	b.WriteString("\n" + helpStyle.Render("↑↓/jk  navigate   enter  select   q  quit"))

	return b.String()
}
