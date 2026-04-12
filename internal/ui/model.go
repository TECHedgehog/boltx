package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/paginator"
	"github.com/charmbracelet/bubbles/spinner"
	bubblesTable "github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"boltx/internal/detect"
)

// Key bindings shared across stages.
var (
	keyNav      = key.NewBinding(key.WithKeys("up", "down", "k", "j"), key.WithHelp("↑↓/jk", "navigate"))
	keyPageNav  = key.NewBinding(key.WithKeys("left", "right", "h", "l"), key.WithHelp("←→/hl", "page"))
	keySelect   = key.NewBinding(key.WithKeys("enter", " "), key.WithHelp("enter/space", "toggle"))
	keyConfirm  = key.NewBinding(key.WithKeys("enter", " "), key.WithHelp("enter", "confirm"))
	keyBack     = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back"))
	keyQuit     = key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q", "quit"))
	keyHelpMore = key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "more"))
	keyHelpLess = key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "less"))
)

type stage int

const (
	stageMenu           stage = iota
	stageUseCase
	stageApplyOrReview
	stageCategoryReview
)

// CategoryOption is a single toggleable setting within a category page.
type CategoryOption struct {
	Label   string
	Checked bool
}

// CategoryPage groups related options under a category name.
type CategoryPage struct {
	Name    string
	Options []CategoryOption
}

// buildCategoryPages returns all category pages with defaults pre-filled for the given use case.
func buildCategoryPages(uc detect.UseCase) []CategoryPage {
	vps := uc == detect.UseCaseVPS
	return []CategoryPage{
		{
			Name: "Firewall",
			Options: []CategoryOption{
				{"Enable firewall", vps},
				{"Allow SSH (port 22)", vps},
				{"Allow HTTP (port 80)", vps},
				{"Allow HTTPS (port 443)", vps},
			},
		},
		{
			Name: "SSH hardening",
			Options: []CategoryOption{
				{"Disable root login", vps},
				{"Disable password authentication", vps},
			},
		},
		{
			Name: "Users",
			Options: []CategoryOption{
				{"Create a new sudo user", false},
			},
		},
		{
			Name: "Packages",
			Options: []CategoryOption{
				{"git", true},
				{"curl", true},
				{"vim", !vps},
				{"htop", true},
				{"build-essential / base-devel", !vps},
				{"ufw", vps},
				{"fail2ban", vps},
			},
		},
	}
}

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
	infoTable     bubblesTable.Model
	help          help.Model
	helpExpanded  bool

	// stageApplyOrReview
	applyOrReviewCursor int

	// stageCategoryReview
	paginator          paginator.Model
	categoryPages      []CategoryPage
	categoryPageCursor int

	// Fixed layout — computed once after detection and terminal size are both known.
	// leftColW and fixedH never change after layoutReady is set, keeping the box
	// stable as the user navigates between screens.
	rightContentW int  // measured width of the rendered info table (set after detection)
	rightContentH int  // measured height of the rendered info table
	leftColW      int  // left column content width (wraps text that exceeds it)
	fixedH        int  // fixed content-area height for both columns
	layoutReady   bool // true once leftColW and fixedH have been locked in
}

var menuItems = []string{
	"Start setup",
	"Quit",
}

var useCaseOptions = []detect.UseCase{detect.UseCaseVPS, detect.UseCaseDevMachine}

// useCaseDescs summarises the pre-selected defaults for each use case,
// shown as a hint below the radio option on the use case screen.
var useCaseDescs = []string{
	"Firewall, SSH hardening, server packages",
	"Dev tools and common packages",
}

var applyOrReviewOptions = []string{
	"Apply recommended settings",
	"Review and customize",
}

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

	p := paginator.New()
	p.Type = paginator.Dots
	p.ActiveDot = lipgloss.NewStyle().Foreground(purple).Render("•")
	p.InactiveDot = lipgloss.NewStyle().Foreground(muted).Render("◦")

	return Model{
		detecting: true,
		spinner:   s,
		help:      h,
		paginator: p,
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
		m = computeLayout(m)

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
		// Measure the right column content so computeLayout can calculate leftColW.
		raw := m.infoTable.View()
		if idx := strings.Index(raw, "\n"); idx >= 0 {
			raw = raw[idx+1:]
		}
		rendered := infoTableBorderStyle.Render(raw)
		m.rightContentW = lipgloss.Width(rendered)
		m.rightContentH = lipgloss.Height(rendered)
		m = computeLayout(m)

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit

		case "?":
			m.helpExpanded = !m.helpExpanded

		case "q", "esc":
			switch m.stage {
			case stageMenu:
				return m, tea.Quit
			case stageUseCase:
				m.stage = stageMenu
			case stageApplyOrReview:
				m.stage = stageUseCase
			case stageCategoryReview:
				m.stage = stageApplyOrReview
				m.paginator.Page = 0
				m.categoryPageCursor = 0
			}

		case "left", "h":
			if m.stage == stageCategoryReview {
				m.paginator.PrevPage()
				m.categoryPageCursor = 0
			}

		case "right", "l":
			if m.stage == stageCategoryReview && !m.paginator.OnLastPage() {
				m.paginator.NextPage()
				m.categoryPageCursor = 0
			}

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
			case stageApplyOrReview:
				if m.applyOrReviewCursor > 0 {
					m.applyOrReviewCursor--
				}
			case stageCategoryReview:
				if m.categoryPageCursor > 0 {
					m.categoryPageCursor--
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
			case stageApplyOrReview:
				if m.applyOrReviewCursor < len(applyOrReviewOptions)-1 {
					m.applyOrReviewCursor++
				}
			case stageCategoryReview:
				curPage := m.categoryPages[m.paginator.Page]
				// On the last page, cursor can reach a "Confirm →" item beyond the options.
				maxCursor := len(curPage.Options) - 1
				if m.paginator.OnLastPage() {
					maxCursor++
				}
				if m.categoryPageCursor < maxCursor {
					m.categoryPageCursor++
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
					// Build category pages here so changes survive a back/forward trip
					// between stageApplyOrReview and stageCategoryReview.
					m.categoryPages = buildCategoryPages(useCaseOptions[m.useCaseCursor])
					m.paginator.SetTotalPages(len(m.categoryPages))
					m.paginator.Page = 0
					m.categoryPageCursor = 0
					m.applyOrReviewCursor = 0
					m.stage = stageApplyOrReview
				}
			case stageApplyOrReview:
				switch m.applyOrReviewCursor {
				case 0:
					// Apply recommended — Stage 5: run processes (not yet built)
				case 1:
					m.paginator.Page = 0
					m.categoryPageCursor = 0
					m.stage = stageCategoryReview
				}
			case stageCategoryReview:
				curPage := m.categoryPages[m.paginator.Page]
				if m.categoryPageCursor < len(curPage.Options) {
					m.categoryPages[m.paginator.Page].Options[m.categoryPageCursor].Checked =
						!m.categoryPages[m.paginator.Page].Options[m.categoryPageCursor].Checked
				} else if m.paginator.OnLastPage() {
					// Confirm → Stage 5 not yet built
				}
			}
		}
	}

	return m, nil
}

// computeLayout locks in leftColW and fixedH the first time both the terminal
// size and the right column measurement are available. After that it is a no-op,
// so the box dimensions never change as the user navigates.
//
// Box width equation:
//
//	border(2) + leftPad(6) + leftColW + rightPad(4) + rightContentW = targetBoxW
//	→ leftColW = targetBoxW − rightContentW − 12
//
// fixedH covers the tallest possible left-column content (Packages page ≈ 16 lines).
func computeLayout(m Model) Model {
	if m.layoutReady || m.width == 0 || m.rightContentW == 0 {
		return m
	}
	targetBoxW := m.width - 4 // 2-char margin each side
	if targetBoxW > 96 {
		targetBoxW = 96
	}
	if targetBoxW < 72 {
		targetBoxW = 72
	}
	m.leftColW = max(targetBoxW-m.rightContentW-12, 28)
	// 16 = tallest left content (Packages page with Confirm item)
	m.fixedH = max(16, m.rightContentH) + 2
	m.layoutReady = true
	return m
}

// View builds the two-column layout centered in the terminal.
//
// Left column:  interactive content vertically centered, hints pinned at bottom.
// Right column: info table independently centered in the full column height.
func (m Model) View() string {
	leftMain := m.viewLeft()

	// Use the locked-in width once available; fall back to content width during
	// the brief detection spinner phase before layoutReady is set.
	leftW := m.leftColW
	if leftW == 0 {
		leftW = lipgloss.Width(leftMain)
	}

	hints := m.viewHints(leftW)
	rightContent := m.viewRight()

	hintsH := lipgloss.Height(hints)

	// Use the locked-in content height once available; fall back to adaptive.
	contentH := m.fixedH
	if contentH == 0 {
		mainH := lipgloss.Height(leftMain)
		rightH := lipgloss.Height(rightContent)
		contentH = max(mainH, rightH) + 2
	}
	totalH := contentH + hintsH

	// Left column: content wrapped and centered inside the fixed width × height.
	// Width(leftW) causes lipgloss to soft-wrap any line that exceeds leftW,
	// keeping the box from growing wider as the user moves between screens.
	centeredLeft := lipgloss.NewStyle().
		Width(leftW).
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
	case stageApplyOrReview:
		return m.viewApplyOrReview()
	case stageCategoryReview:
		return m.viewCategoryReview()
	default:
		return m.viewMenu()
	}
}

// viewHints renders the help line constrained to maxWidth so it never
// widens the box beyond the menu content.
func (m Model) viewHints(maxWidth int) string {
	h := m.help
	h.Width = maxWidth

	var bindings []key.Binding
	if m.helpExpanded {
		switch m.stage {
		case stageUseCase, stageApplyOrReview:
			bindings = []key.Binding{keyNav, keyConfirm, keyBack, keyHelpLess}
		case stageCategoryReview:
			bindings = []key.Binding{keyNav, keyPageNav, keySelect, keyBack, keyHelpLess}
		default:
			bindings = []key.Binding{keyNav, keySelect, keyQuit, keyHelpLess}
		}
	} else {
		switch m.stage {
		case stageUseCase, stageApplyOrReview:
			bindings = []key.Binding{keyBack, keyHelpMore}
		case stageCategoryReview:
			bindings = []key.Binding{keyPageNav, keyBack, keyHelpMore}
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
		b.WriteString(style.Render(radio+uc.String()) + "\n")
		b.WriteString(mutedStyle.Render("  "+useCaseDescs[i]))
		if i < len(useCaseOptions)-1 {
			b.WriteString("\n\n")
		}
	}

	return b.String()
}

func (m Model) viewApplyOrReview() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("boltx") + "\n")
	b.WriteString(subtitleStyle.Render("How would you like to proceed?") + "\n\n")

	uc := useCaseOptions[m.useCaseCursor]
	b.WriteString(mutedStyle.Render("Use case: ") + normalStyle.Render(uc.String()) + "\n\n")

	for i, opt := range applyOrReviewOptions {
		radio := radioOff
		style := normalStyle
		if m.applyOrReviewCursor == i {
			radio = radioOn
			style = selectedStyle
		}
		b.WriteString(style.Render(radio + opt))
		if i < len(applyOrReviewOptions)-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

func (m Model) viewCategoryReview() string {
	var b strings.Builder

	page := m.categoryPages[m.paginator.Page]

	b.WriteString(titleStyle.Render("boltx") + "\n")
	b.WriteString(subtitleStyle.Render("Review settings") + "\n\n")
	b.WriteString(sectionStyle.Render(page.Name) + "\n\n")

	for i, opt := range page.Options {
		cursor := noCursorStr
		itemStyle := normalStyle
		if m.categoryPageCursor == i {
			cursor = cursorStr
			itemStyle = selectedStyle
		}
		radio := radioOff
		if opt.Checked {
			radio = radioOn
		}
		b.WriteString(cursor + itemStyle.Render(radio+opt.Label) + "\n")
	}

	b.WriteString("\n" + m.paginator.View())

	// Confirm item only on the last page.
	if m.paginator.OnLastPage() {
		confirmIdx := len(page.Options)
		cursor := noCursorStr
		confirmStyle := normalStyle
		if m.categoryPageCursor == confirmIdx {
			cursor = cursorStr
			confirmStyle = selectedStyle
		}
		b.WriteString("\n\n" + cursor + confirmStyle.Render("Confirm →"))
	}

	return b.String()
}
