package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"boltx/internal/detect"
)

type stage int

const (
	stageMenu    stage = iota
	stageUseCase
)

// detectDoneMsg carries the result of the async environment detection.
type detectDoneMsg struct {
	env    detect.Environment
	osInfo detect.OSInfo
}

// Model holds all TUI state.
type Model struct {
	width  int
	height int

	stage      stage
	menuCursor int

	detecting     bool
	spinner       spinner.Model
	env           detect.Environment
	osInfo        detect.OSInfo
	useCaseCursor int
}

var menuItems = []string{
	"Start setup",
	"Quit",
}

// NewModel returns the initial model.
func NewModel() Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(purple)
	return Model{
		detecting: true,
		spinner:   s,
	}
}

// Init fires environment and OS detection as soon as the program starts.
func (m Model) Init() tea.Cmd {
	return tea.Batch(doDetect, m.spinner.Tick)
}

// doDetect runs detection in the background and returns the result as a message.
func doDetect() tea.Msg {
	return detectDoneMsg{
		env:    detect.Detect(),
		osInfo: detect.DetectOS(),
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case spinner.TickMsg:
		if m.detecting {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case detectDoneMsg:
		m.detecting = false
		m.env = msg.env
		m.osInfo = msg.osInfo
		m.useCaseCursor = int(msg.env.SuggestedUseCase())

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit

		case "q", "esc":
			if m.stage == stageMenu {
				return m, tea.Quit
			}
			m.stage = stageMenu

		case "up", "k":
			switch m.stage {
			case stageMenu:
				if m.menuCursor > 0 {
					m.menuCursor--
				}
			case stageUseCase:
				if !m.detecting && m.useCaseCursor > 0 {
					m.useCaseCursor--
				}
			}

		case "down", "j":
			switch m.stage {
			case stageMenu:
				if m.menuCursor < len(menuItems)-1 {
					m.menuCursor++
				}
			case stageUseCase:
				if !m.detecting && m.useCaseCursor < 1 {
					m.useCaseCursor++
				}
			}

		case "enter", " ":
			switch m.stage {
			case stageMenu:
				switch m.menuCursor {
				case 0:
					m.stage = stageUseCase
				case 1:
					return m, tea.Quit
				}
			case stageUseCase:
				if !m.detecting {
					// confirmed — Stage 3 will go here
				}
			}
		}
	}

	return m, nil
}

// View builds the two-column layout centered in the terminal.
//
// Left column:  interactive content vertically centered, hints pinned at bottom.
// Right column: info table independently centered in the full column height.
func (m Model) View() string {
	leftMain := m.viewLeft()
	hints := m.viewHints()
	rightContent := m.viewRight()

	mainH := lipgloss.Height(leftMain)
	hintsH := lipgloss.Height(hints)
	rightH := lipgloss.Height(rightContent)

	// +2 buffer ensures both sides always have centering room even when
	// one side's natural height equals the other's.
	contentH := max(mainH, rightH) + 2
	totalH := contentH + hintsH

	// Left column: main content centered in contentH, hints pinned below.
	centeredLeft := lipgloss.NewStyle().
		Height(contentH).
		Align(lipgloss.Left, lipgloss.Center).
		Render(leftMain)
	leftBlock := lipgloss.JoinVertical(lipgloss.Left, centeredLeft, hints)
	leftCol := lipgloss.NewStyle().Padding(0, 3).Render(leftBlock)

	// Right column: table independently centered in totalH.
	// Horizontal padding only — vertical padding would break JoinHorizontal.
	centeredRight := lipgloss.NewStyle().
		Height(totalH).
		Align(lipgloss.Left, lipgloss.Center).
		Render(rightContent)
	rightCol := lipgloss.NewStyle().Padding(0, 2).Render(centeredRight)

	box := boxStyle.Render(lipgloss.JoinHorizontal(lipgloss.Top, leftCol, rightCol))

	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}

// viewLeft returns the main interactive content for the left column (no hints).
func (m Model) viewLeft() string {
	switch m.stage {
	case stageUseCase:
		return m.viewUseCase()
	default:
		return m.viewMenu()
	}
}

// viewHints returns the context-sensitive hint line shown at the bottom of the box.
func (m Model) viewHints() string {
	switch m.stage {
	case stageUseCase:
		return helpStyle.Render("↑↓/jk  navigate   enter  confirm   esc  back")
	default:
		return helpStyle.Render("↑↓/jk  navigate   enter  select   q  quit")
	}
}

// viewRight returns the system info table for the right column.
// Uses lipgloss.Table for full border and row-divider support.
// Shows an animated spinner while detection is running.
func (m Model) viewRight() string {
	if m.detecting {
		return m.spinner.View()
	}
	return renderInfoTable(m.env, m.osInfo)
}

// renderInfoTable builds a bordered key-value table from detected system info.
// Rendered manually with box-drawing characters so we have full control over
// the outer border and row dividers. bubbles/table is reserved for future
// interactive tables (lists, package selection, etc.).
func renderInfoTable(env detect.Environment, osInfo detect.OSInfo) string {
	type kv struct{ k, v string }
	var data []kv

	if osInfo.PrettyName != "" {
		data = append(data, kv{"OS", osInfo.PrettyName})
	}
	data = append(data, kv{"Virt", env.Virt.String()})
	if osInfo.Pkg != detect.PkgUnknown {
		data = append(data, kv{"Pkg", osInfo.Pkg.String()})
	}
	if env.ViaSSH {
		data = append(data, kv{"SSH", "connected"})
	}
	ipVal := "private"
	if env.HasPublicIP {
		ipVal = "public"
	}
	data = append(data, kv{"IP", ipVal})

	// Measure column widths from content (all keys/values are ASCII).
	maxK, maxV := 0, 0
	for _, r := range data {
		if len(r.k) > maxK {
			maxK = len(r.k)
		}
		if len(r.v) > maxV {
			maxV = len(r.v)
		}
	}

	// Cell widths include one space of padding on each side.
	kw := maxK + 2
	vw := maxV + 2

	top := mutedStyle.Render("┌" + strings.Repeat("─", kw) + "┬" + strings.Repeat("─", vw) + "┐")
	mid := mutedStyle.Render("├" + strings.Repeat("─", kw) + "┼" + strings.Repeat("─", vw) + "┤")
	bot := mutedStyle.Render("└" + strings.Repeat("─", kw) + "┴" + strings.Repeat("─", vw) + "┘")
	pipe := mutedStyle.Render("│")

	var lines []string
	lines = append(lines, top)
	for i, r := range data {
		// Width() pads content to the column width so all rows are the same size.
		keyCell := " " + mutedStyle.Width(maxK).Render(r.k) + " "
		valCell := " " + normalStyle.Width(maxV).Render(r.v) + " "
		lines = append(lines, pipe+keyCell+pipe+valCell+pipe)
		if i < len(data)-1 {
			lines = append(lines, mid)
		}
	}
	lines = append(lines, bot)

	return strings.Join(lines, "\n")
}

func (m Model) viewMenu() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("boltx") + "\n")
	b.WriteString(subtitleStyle.Render("Linux setup tool") + "\n\n")

	for i, item := range menuItems {
		cursor := noCursorStr
		style := normalStyle
		if m.menuCursor == i {
			cursor = cursorStr
			style = selectedStyle
		}
		b.WriteString(cursor + style.Render(item))
		if i < len(menuItems)-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

func (m Model) viewUseCase() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("boltx") + "\n")
	b.WriteString(subtitleStyle.Render("Use case selection") + "\n\n")

	if m.detecting {
		b.WriteString(mutedStyle.Render("Detecting environment..."))
		return b.String()
	}

	b.WriteString(mutedStyle.Render("Detected:  ") + normalStyle.Render(m.env.Virt.String()) + "\n")
	b.WriteString(mutedStyle.Render("Suggested: ") + greenStyle.Render(m.env.SuggestedUseCase().String()) + "\n\n")

	useCases := []detect.UseCase{detect.UseCaseVPS, detect.UseCaseDevMachine}
	for i, uc := range useCases {
		radio := radioOff
		style := normalStyle
		if m.useCaseCursor == i {
			radio = radioOn
			style = selectedStyle
		}
		b.WriteString(style.Render(radio + uc.String()))
		if i < len(useCases)-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}
