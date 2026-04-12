package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	bubblesTable "github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"boltx/internal/detect"
)

// Key bindings shared across stages.
var (
	keyNav      = key.NewBinding(key.WithKeys("up", "down", "k", "j"), key.WithHelp("↑↓/jk", "navigate"))
	keySelect   = key.NewBinding(key.WithKeys("enter", " "), key.WithHelp("enter", "select"))
	keyConfirm  = key.NewBinding(key.WithKeys("enter", " "), key.WithHelp("enter", "confirm"))
	keyBack     = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back"))
	keyQuit     = key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q", "quit"))
	keyHelpMore = key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "more"))
	keyHelpLess = key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "less"))
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
	osInfo        detect.OSInfo // reserved for Stage 3 (distro-specific commands)
	useCaseCursor int
	infoTable     bubblesTable.Model
	help          help.Model
	helpExpanded  bool
}

var menuItems = []string{
	"Start setup",
	"Quit",
}

var useCaseOptions = []detect.UseCase{detect.UseCaseVPS, detect.UseCaseDevMachine}

// NewModel returns the initial model.
func NewModel() Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(purple)

	h := help.New()
	h.Styles.ShortKey = lipgloss.NewStyle().Foreground(white)
	h.Styles.ShortDesc = lipgloss.NewStyle().Foreground(muted)
	h.Styles.ShortSeparator = lipgloss.NewStyle().Foreground(muted)
	h.Styles.Ellipsis = lipgloss.NewStyle().Foreground(muted)

	return Model{
		detecting: true,
		spinner:   s,
		help:      h,
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
		m.infoTable = buildInfoTable(msg.env, msg.osInfo)

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit

		case "?":
			m.helpExpanded = !m.helpExpanded

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
				if !m.detecting && m.useCaseCursor < len(useCaseOptions)-1 {
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
	hints := m.viewHints(lipgloss.Width(leftMain))
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

// viewHints renders the help line constrained to maxWidth so it never
// widens the box beyond the menu content.
// Collapsed: only quit/back + "? more".
// Expanded: all bindings on one line via ShortHelpView + "? less".
func (m Model) viewHints(maxWidth int) string {
	h := m.help
	h.Width = maxWidth

	var bindings []key.Binding
	if m.helpExpanded {
		switch m.stage {
		case stageUseCase:
			bindings = []key.Binding{keyNav, keyConfirm, keyBack, keyHelpLess}
		default:
			bindings = []key.Binding{keyNav, keySelect, keyQuit, keyHelpLess}
		}
	} else {
		switch m.stage {
		case stageUseCase:
			bindings = []key.Binding{keyBack, keyHelpMore}
		default:
			bindings = []key.Binding{keyQuit, keyHelpMore}
		}
	}
	return h.ShortHelpView(bindings)
}

// viewRight returns the system info table for the right column.
// Shows an animated spinner while detection is running.
func (m Model) viewRight() string {
	if m.detecting {
		return m.spinner.View()
	}
	// bubbles/table always prepends a header line; drop it since we want no headers.
	raw := m.infoTable.View()
	if idx := strings.Index(raw, "\n"); idx >= 0 {
		raw = raw[idx+1:]
	}
	return infoTableBorderStyle.Render(raw)
}

// buildInfoTable constructs a bubbles/table model from detected system info.
// The table is built once when detection completes and stored on the Model.
//
// Rows must contain plain (uncolored) strings. bubbles/table internally calls
// runewidth.Truncate on cell values before measuring; go-runewidth does not
// strip ANSI, so pre-colored strings cause premature truncation.
func buildInfoTable(env detect.Environment, osInfo detect.OSInfo) bubblesTable.Model {
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

	// Column widths derived from content only — the header row is stripped in
	// viewRight, so its title strings don't set a meaningful floor.
	// Padding(0,1) in the cell styles adds one space per side; no manual +2 needed.
	maxK, maxV := 0, 0
	for _, r := range data {
		if len(r.k) > maxK {
			maxK = len(r.k)
		}
		if len(r.v) > maxV {
			maxV = len(r.v)
		}
	}

	cols := []bubblesTable.Column{
		{Title: "Property", Width: maxK},
		{Title: "Value", Width: maxV},
	}

	rows := make([]bubblesTable.Row, len(data))
	for i, r := range data {
		rows[i] = bubblesTable.Row{r.k, r.v}
	}

	s := bubblesTable.Styles{
		Header: lipgloss.NewStyle().Bold(true).Foreground(muted).Padding(0, 1),
		Cell:   lipgloss.NewStyle().Foreground(white).Padding(0, 1),
		// Selected wraps the entire already-cell-padded row, so no extra padding.
		Selected: lipgloss.NewStyle().Foreground(white),
	}

	// WithHeight receives total lines including header; internally it subtracts
	// the header height so the viewport fits exactly len(data) rows.
	return bubblesTable.New(
		bubblesTable.WithColumns(cols),
		bubblesTable.WithRows(rows),
		bubblesTable.WithHeight(len(data)+1),
		bubblesTable.WithStyles(s),
	)
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

	b.WriteString(mutedStyle.Render("Suggested: ") + greenStyle.Render(m.env.SuggestedUseCase().String()) + "\n\n")

	for i, uc := range useCaseOptions {
		radio := radioOff
		style := normalStyle
		if m.useCaseCursor == i {
			radio = radioOn
			style = selectedStyle
		}
		b.WriteString(style.Render(radio + uc.String()))
		if i < len(useCaseOptions)-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}
