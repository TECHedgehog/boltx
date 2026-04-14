package ui

import (
	"fmt"
	"regexp"
	"strings"

	"boltx/internal/detect"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	bubblesTable "github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ansiEscape matches ANSI SGR escape sequences so they can be stripped from
// rendered strings before re-applying a different color.
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// Key bindings shared across stages.
var (
	keyNav      = key.NewBinding(key.WithKeys("up", "down", "k", "j"), key.WithHelp("↑↓/jk", "navigate"))
	keyTabNav   = key.NewBinding(key.WithKeys("left", "right", "h", "l"), key.WithHelp("←→/hl", "tab"))
	keySelect   = key.NewBinding(key.WithKeys("enter", " "), key.WithHelp("enter/space", "toggle"))
	keyConfirm  = key.NewBinding(key.WithKeys("enter", " "), key.WithHelp("enter", "confirm"))
	keyBack     = key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q/esc", "back"))
	keyQuit     = key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q/esc", "quit"))
	keyHelpMore = key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "more"))
	keyHelpLess = key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "less"))
)

// maxOptionsPerPage is the maximum number of options shown per tab sub-page.
// When a category has more options than this, it gains extra sub-pages.
const maxOptionsPerPage = 8

type stage int

const (
	stageMenu stage = iota
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

// subPageCount returns the number of sub-pages needed for nOptions.
func subPageCount(nOptions int) int {
	if nOptions == 0 {
		return 1
	}
	return (nOptions + maxOptionsPerPage - 1) / maxOptionsPerPage
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
	categoryPages      []CategoryPage
	activeTab          int // which category tab is visible
	tabSubPage         int // sub-page within the active tab (for overflow)
	categoryPageCursor int // cursor position within the current sub-page

	// Layout — recomputed on every terminal resize and after detection completes.
	rightContentW int // measured width of the rendered info table (set after detection)
	rightContentH int // measured height of the rendered info table (set after detection)
	leftColW      int // left column content width; lipgloss wraps anything wider
	stableTop     int // vertical offset of the box, fixed per window size so box grows downward
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
	h.Styles.FullKey = lipgloss.NewStyle().Foreground(white)
	h.Styles.FullDesc = lipgloss.NewStyle().Foreground(muted)
	h.Styles.FullSeparator = lipgloss.NewStyle().Foreground(muted)

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
				m.activeTab = 0
				m.tabSubPage = 0
				m.categoryPageCursor = 0
			}

		case "left", "h":
			if m.stage == stageCategoryReview && m.activeTab > 0 {
				m.activeTab--
				m.tabSubPage = 0
				m.categoryPageCursor = 0
			}

		case "right", "l":
			if m.stage == stageCategoryReview && m.activeTab < len(m.categoryPages)-1 {
				m.activeTab++
				m.tabSubPage = 0
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
				} else if m.tabSubPage > 0 {
					// Wrap back to the last option on the previous sub-page.
					m.tabSubPage--
					curPage := m.categoryPages[m.activeTab]
					prevStart := m.tabSubPage * maxOptionsPerPage
					prevEnd := min(prevStart+maxOptionsPerPage, len(curPage.Options))
					m.categoryPageCursor = prevEnd - prevStart - 1
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
				curPage := m.categoryPages[m.activeTab]
				nSub := subPageCount(len(curPage.Options))
				startIdx := m.tabSubPage * maxOptionsPerPage
				endIdx := min(startIdx+maxOptionsPerPage, len(curPage.Options))
				subPageLen := endIdx - startIdx
				isLastTab := m.activeTab == len(m.categoryPages)-1
				isLastSub := m.tabSubPage == nSub-1
				// Confirm → counts as one extra cursor position on the last tab+subpage.
				maxCursor := subPageLen - 1
				if isLastTab && isLastSub {
					maxCursor++
				}
				if m.categoryPageCursor < maxCursor {
					m.categoryPageCursor++
				} else if !isLastSub {
					// Auto-advance to the next sub-page.
					m.tabSubPage++
					m.categoryPageCursor = 0
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
					m.activeTab = 0
					m.tabSubPage = 0
					m.categoryPageCursor = 0
					m.applyOrReviewCursor = 0
					m.stage = stageApplyOrReview
				}
			case stageApplyOrReview:
				switch m.applyOrReviewCursor {
				case 0:
					// Apply recommended — Stage 5: run processes (not yet built)
				case 1:
					m.activeTab = 0
					m.tabSubPage = 0
					m.categoryPageCursor = 0
					m.stage = stageCategoryReview
				}
			case stageCategoryReview:
				curPage := m.categoryPages[m.activeTab]
				startIdx := m.tabSubPage * maxOptionsPerPage
				endIdx := min(startIdx+maxOptionsPerPage, len(curPage.Options))
				subPageLen := endIdx - startIdx
				if m.categoryPageCursor < subPageLen {
					absIdx := startIdx + m.categoryPageCursor
					m.categoryPages[m.activeTab].Options[absIdx].Checked =
						!m.categoryPages[m.activeTab].Options[absIdx].Checked
				} else {
					// Confirm → Stage 5 not yet built
				}
			}
		}
	}

	return m, nil
}

// computeLayout sets leftColW and stableTop whenever both the terminal size
// and the right-column measurement are available.
//
// leftColW is content-driven: capped at maxLeftW so the box is no wider than
// the content needs, with a screen-driven floor so it never overflows narrow
// terminals.
//
// stableTop is the number of blank lines above the box. It is computed once
// from the terminal height and a worst-case box height (tallestBoxH) so the
// box always starts high enough that even the longest tab page fits without
// pushing the bottom edge past the terminal, which would look like movement.
func computeLayout(m Model) Model {
	if m.width == 0 || m.rightContentW == 0 {
		return m
	}
	// Content-driven left column width.
	// Binding constraints (both hit 35 chars):
	//   "● Dev machine / Test VM (Suggested)"  → 2+21+1+11 = 35
	//   "  ○ Disable password authentication"   → 2+2+31   = 35
	// Tab bar peaks at 29 chars ("SSH hardening" active + 3 ghost tabs × 4).
	const maxLeftW = 35
	screenLeftW := m.width - 4 - m.rightContentW - 8
	m.leftColW = max(min(maxLeftW, screenLeftW), 28)

	// Stable vertical anchor.
	// tallestBoxH covers the Packages tab (15 content lines) + topPad (1) +
	// hints (3) + border (2) = 21, rounded up to 23 for the wider tab bar.
	if m.rightContentH > 0 {
		const tallestBoxH = 23
		refBoxH := max(m.rightContentH+6, tallestBoxH)
		m.stableTop = max(0, (m.height-refBoxH)/2)
	}
	return m
}

// View builds the two-column layout centered in the terminal.
//
// When the left column content is taller than the info table, the lines that
// would sit beside blank right-column space instead flow below the table,
// spanning the full inner width of the box so no space is wasted.
func (m Model) View() string {
	leftMain := m.viewLeft()

	leftW := m.leftColW
	if leftW == 0 {
		leftW = lipgloss.Width(leftMain)
	}

	rightContent := m.viewRight()

	const topPad = 1
	paddedLeft := strings.Repeat("\n", topPad) + leftMain

	// leftCol outer = leftW + PaddingLeft(2) + PaddingRight(1) = leftW + 3
	// rightCol outer = rightContentW + PaddingLeft(1) + PaddingRight(1) = rightContentW + 2
	// total inner (inside box border) = leftW + rightContentW + 5
	// bottom content width = total inner - PaddingLeft(2) - PaddingRight(1) = leftW + rightContentW + 2
	bottomContentW := leftW + m.rightContentW + 2

	leftH := lipgloss.Height(paddedLeft)
	rightH := m.rightContentH // 0 until detection completes

	var box string
	switch {
	case m.stage == stageCategoryReview && m.rightContentW > 0:
		// Category-review layout: title + tab bar sit beside the info table.
		// A purple │ runs down the right edge of the left column from the top
		// box border to the connector line, terminated by ╮ on both ends.
		tabBar := m.viewTabBar()
		tabBarLines := strings.Split(tabBar, "\n")
		tabBarBottomLine := tabBarLines[len(tabBarLines)-1]
		tabBarBottomW := lipgloss.Width(tabBarBottomLine)
		tabBarTopPart := strings.Join(tabBarLines[:len(tabBarLines)-1], "\n")

		// Regular block: PaddingLeft(2) + Width(leftW), no PaddingRight.
		// A purple │ is appended to every line instead, forming the vertical
		// separator. Total line width = leftW+3 (same as with PaddingRight(1)).
		aboveSepTop := strings.Repeat("\n", topPad) +
			titleStyle.Render("boltx") + "\n" +
			subtitleStyle.Render("Review settings") + "\n\n" +
			tabBarTopPart
		regularBlock := lipgloss.NewStyle().PaddingLeft(2).Render(
			lipgloss.NewStyle().Width(leftW).Render(aboveSepTop))
		purpleBar := lipgloss.NewStyle().Foreground(purple).Render("│")
		regularLines := strings.Split(regularBlock, "\n")
		for i, l := range regularLines {
			regularLines[i] = l + purpleBar
		}
		regularBlock = strings.Join(regularLines, "\n")

		// Connector line: "──" + tab bar bottom chars + "─" fill + "╮".
		// Width = 2 + tabBarBottomW + remaining + 1 = leftW+3, matching every
		// regularBlock line so JoinHorizontal sees a uniform-width left column.
		// The ╮ closes the vertical separator against the connector.
		remaining := leftW - tabBarBottomW
		bareBottom := ansiEscape.ReplaceAllString(tabBarBottomLine, "")
		bare := "──" + bareBottom
		if remaining > 0 {
			bare += strings.Repeat("─", remaining)
		}
		bare += "╯"
		connectorLine := lipgloss.NewStyle().Foreground(purple).Render(bare)

		leftAboveCol := regularBlock + "\n" + connectorLine
		rightAboveCol := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1).Render(rightContent)
		aboveRow := lipgloss.JoinHorizontal(lipgloss.Top, leftAboveCol, rightAboveCol)

		body := m.viewCategoryReviewBody(bottomContentW)
		hints := m.viewHints(bottomContentW)
		bodyBlock := lipgloss.NewStyle().
			Width(bottomContentW).
			PaddingLeft(2).PaddingRight(1).
			Render(body + hints)

		box = boxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, aboveRow, bodyBlock))

		// Insert ╮ into the top border directly above the │ separator column.
		// The separator is at inner-content column leftW+2; top-border rune
		// index = (leftW+2) + 1 = leftW+3 (offset by 1 for the leading ╭).
		// leftW adapts to rightContentW so this stays correct if the table grows.
		boxLines := strings.Split(box, "\n")
		if len(boxLines) > 0 {
			stripped := ansiEscape.ReplaceAllString(boxLines[0], "")
			runes := []rune(stripped)
			if pos := leftW + 3; pos < len(runes) {
				runes[pos] = '┬'
				boxLines[0] = lipgloss.NewStyle().Foreground(purple).Render(string(runes))
			}
			box = strings.Join(boxLines, "\n")
		}

	case m.rightContentW > 0 && leftH > rightH:
		// Overflow layout: first rightH lines sit beside the table; the rest
		// flow below it at full inner width.
		leftLines := strings.Split(paddedLeft, "\n")
		topLeft := strings.Join(leftLines[:rightH], "\n")
		bottomLeft := strings.Join(leftLines[rightH:], "\n")

		topLeftCol := lipgloss.NewStyle().PaddingLeft(2).PaddingRight(1).
			Render(lipgloss.NewStyle().Width(leftW).Render(topLeft))
		topRightCol := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1).
			Render(rightContent)
		topRow := lipgloss.JoinHorizontal(lipgloss.Top, topLeftCol, topRightCol)

		hints := m.viewHints(bottomContentW)
		bottomBlock := lipgloss.NewStyle().
			Width(bottomContentW).
			PaddingLeft(2).PaddingRight(1).
			Render(bottomLeft + hints)

		box = boxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, topRow, bottomBlock))

	case m.rightContentW > 0:
		// Left content fits beside the table (leftH <= rightH).
		// Check whether hints would extend below the table's bottom edge; if so,
		// render them full-width in a bottom block instead of in the narrow left column.
		hints := m.viewHints(bottomContentW)
		if leftH+lipgloss.Height(hints) > rightH {
			topLeftBlock := lipgloss.NewStyle().Width(leftW).Render(paddedLeft)
			topLeftCol := lipgloss.NewStyle().PaddingLeft(2).PaddingRight(1).Render(topLeftBlock)
			topRightCol := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1).Render(rightContent)
			topRow := lipgloss.JoinHorizontal(lipgloss.Top, topLeftCol, topRightCol)
			bottomBlock := lipgloss.NewStyle().
				Width(bottomContentW).
				PaddingLeft(2).PaddingRight(1).
				Render(hints)
			box = boxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, topRow, bottomBlock))
		} else {
			hints = m.viewHints(leftW)
			leftMainBlock := lipgloss.NewStyle().Width(leftW).Render(paddedLeft)
			leftBlock := lipgloss.JoinVertical(lipgloss.Left, leftMainBlock, hints)
			leftCol := lipgloss.NewStyle().PaddingLeft(2).PaddingRight(1).Render(leftBlock)
			rightCol := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1).Render(rightContent)
			box = boxStyle.Render(lipgloss.JoinHorizontal(lipgloss.Top, leftCol, rightCol))
		}

	default:
		// Detecting or no right content yet: single-column layout.
		hints := m.viewHints(leftW)
		leftMainBlock := lipgloss.NewStyle().Width(leftW).Render(paddedLeft)
		leftBlock := lipgloss.JoinVertical(lipgloss.Left, leftMainBlock, hints)
		leftCol := lipgloss.NewStyle().PaddingLeft(2).PaddingRight(1).Render(leftBlock)
		rightCol := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1).Render(rightContent)
		box = boxStyle.Render(lipgloss.JoinHorizontal(lipgloss.Top, leftCol, rightCol))
	}

	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Top,
			strings.Repeat("\n", m.stableTop)+box)
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

// viewHints renders the help line constrained to maxWidth.
func (m Model) viewHints(maxWidth int) string {
	h := m.help
	h.Width = maxWidth

	if m.helpExpanded {
		if m.stage == stageCategoryReview {
			// Three columns: navigation | actions | meta — max 2 rows total.
			groups := [][]key.Binding{
				{keyNav, keyTabNav},
				{keySelect, keyBack},
				{keyHelpLess},
			}
			return "\n\n" + h.FullHelpView(groups)
		}
		// All other stages: single line using the full box width so it doesn't truncate.
		h.Width = m.leftColW + m.rightContentW + 2
		var bindings []key.Binding
		switch m.stage {
		case stageUseCase, stageApplyOrReview:
			bindings = []key.Binding{keyNav, keyConfirm, keyBack, keyHelpLess}
		default: // stageMenu
			bindings = []key.Binding{keyNav, keyConfirm, keyQuit, keyHelpLess}
		}
		return "\n\n" + h.ShortHelpView(bindings)
	}

	var bindings []key.Binding
	switch m.stage {
	case stageUseCase, stageApplyOrReview:
		bindings = []key.Binding{keyBack, keyHelpMore}
	case stageCategoryReview:
		bindings = []key.Binding{keyBack, keyHelpMore}
	default:
		bindings = []key.Binding{keyQuit, keyHelpMore}
	}
	return "\n\n" + h.ShortHelpView(bindings)
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
		Header:   lipgloss.NewStyle().Bold(true).Foreground(muted).Padding(0, 1),
		Cell:     lipgloss.NewStyle().Foreground(white).Padding(0, 1),
		Selected: lipgloss.NewStyle().Foreground(white),
	}

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
	b.WriteString(subtitleStyle.Render("Easy first setup... and FAST!") + "\n\n")

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

	suggested := m.env.SuggestedUseCase()
	colW := m.leftColW
	if colW == 0 {
		colW = 40
	}
	for i, uc := range useCaseOptions {
		radio := radioOff
		style := normalStyle
		if m.useCaseCursor == i {
			radio = radioOn
			style = selectedStyle
		}
		line := style.Render(radio + uc.String())
		if detect.UseCase(i) == suggested {
			line += " " + greenStyle.Render("(Suggested)")
		}
		b.WriteString(line + "\n")
		b.WriteString(renderOptionLine("  ", "", useCaseDescs[i], mutedStyle, colW))
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

// viewCategoryReviewAboveSep returns the content that sits above the
// full-width separator: title, subtitle, and the tab bar.
func (m Model) viewCategoryReviewAboveSep() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("boltx") + "\n")
	b.WriteString(subtitleStyle.Render("Review settings") + "\n\n")
	b.WriteString(m.viewTabBar())
	return b.String()
}

// viewCategoryReviewBody returns the options list and optional confirm button
// that sit below the full-width separator. maxWidth is the available text
// width for word-wrap; callers should pass bottomContentW when the body
// spans the full inner width, or leftColW for the narrow fallback.
func (m Model) viewCategoryReviewBody(maxWidth int) string {
	var b strings.Builder
	colW := maxWidth
	if colW == 0 {
		colW = m.leftColW
		if colW == 0 {
			colW = 40
		}
	}

	page := m.categoryPages[m.activeTab]
	nSub := subPageCount(len(page.Options))
	startIdx := m.tabSubPage * maxOptionsPerPage
	endIdx := min(startIdx+maxOptionsPerPage, len(page.Options))
	subOpts := page.Options[startIdx:endIdx]

	for i, opt := range subOpts {
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
		b.WriteString(renderOptionLine(cursor, radio, opt.Label, itemStyle, colW) + "\n")
	}

	isLastTab := m.activeTab == len(m.categoryPages)-1
	isLastSub := m.tabSubPage == nSub-1
	if isLastTab && isLastSub {
		confirmIdx := len(subOpts)
		cursor := noCursorStr
		cStyle := normalStyle
		if m.categoryPageCursor == confirmIdx {
			cursor = cursorStr
			cStyle = selectedStyle
		}
		b.WriteString("\n" + cursor + cStyle.Render("Confirm →"))
	}

	return b.String()
}

// viewCategoryReview is the fallback used before detection completes.
func (m Model) viewCategoryReview() string {
	return m.viewCategoryReviewAboveSep() + "\n\n" + m.viewCategoryReviewBody(m.leftColW)
}

// viewTabBar renders the horizontal tab strip for the category review stage.
// The active tab is 3 lines tall (border top, label, open-bottom). Ghost tabs
// are 2 lines tall (border top + content, no bottom border). Bottom-aligning
// them makes the active tab visually rise above the background tabs, giving
// the strip depth. The open bottoms all connect to the separator below.
func (m Model) viewTabBar() string {
	parts := make([]string, len(m.categoryPages))
	for i, page := range m.categoryPages {
		dist := m.activeTab - i
		if dist < 0 {
			dist = -dist
		}
		if dist == 0 {
			label := page.Name
			n := subPageCount(len(page.Options))
			if n > 1 {
				label = fmt.Sprintf("%s %d/%d", page.Name, m.tabSubPage+1, n)
			}
			parts[i] = activeTabStyle.Render(label)
		} else {
			// Ghost tabs are 1 line shorter than the active tab (2 vs 3 lines):
			// top border on line 1, closed connecting bottom on line 2, no
			// content row. Built manually so we can skip the middle content line
			// that lipgloss.Border always adds.
			inner := strings.Repeat("─", 2)
			top := mutedStyle.Render("╭" + inner + "╮")
			bot := mutedStyle.Render("┴" + inner + "┴")
			parts[i] = top + "\n" + bot
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Bottom, parts...)
}

// renderOptionLine renders one option row with word-wrap when the label is
// too wide for maxWidth. Continuation lines are indented to align with the
// first character of the label text.
func renderOptionLine(cursor, radio, label string, style lipgloss.Style, maxWidth int) string {
	prefix := cursor + radio          // e.g. "  ○ " — display width 4, but byte length may differ
	prefixW := lipgloss.Width(prefix) // use cell width, not byte length (○ ● › are multibyte)
	if prefixW+len(label) <= maxWidth {
		return prefix + style.Render(label)
	}
	availW := maxWidth - prefixW
	indent := strings.Repeat(" ", prefixW)
	words := strings.Fields(label)
	var lines []string
	cur := ""
	for _, w := range words {
		switch {
		case cur == "":
			cur = w
		case len(cur)+1+len(w) <= availW:
			cur += " " + w
		default:
			lines = append(lines, cur)
			cur = w
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	var sb strings.Builder
	for i, line := range lines {
		if i > 0 {
			sb.WriteByte('\n')
			sb.WriteString(indent)
		} else {
			sb.WriteString(prefix)
		}
		sb.WriteString(style.Render(line))
	}
	return sb.String()
}
