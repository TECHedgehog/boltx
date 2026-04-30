package ui

import (
	"math/rand"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── config ───────────────────────────────────────────────────────────────────

const (
	// ── timing (easy to tune) ────────────────────────────────────────────────
	// Reveal/dismiss speed: lower = faster.
	// Total reveal time ≈ (splashTargetFrames + splashNoiseWin + 2) × splashTickDur.
	splashTargetFrames = 25              // frames across full reveal/dismiss  (was 40)
	splashTickDur      = 30 * time.Millisecond // ms per tick               (was 40ms)

	// Hold duration ≈ splashHoldTotal × splashTickDur. Must be divisible by len(Themes).
	splashHoldTotal = 60 // hold ticks (60 × 30ms = ~1.8s, 20 ticks per theme)

	splashNoiseWin = 5  // noise-glitch window in frames (keep as-is)

	// ── layout ───────────────────────────────────────────────────────────────
	splashInnerW = 60 // content width inside the box
	splashVPad   = 3  // blank lines above and below logo

	// ── animation feel ───────────────────────────────────────────────────────
	// Max random horizontal offset per column (frames). Lower = more vertical wave.
	// 0 = perfectly vertical; splashTargetFrames/2 = very diagonal.
	splashColJitter = 3
)

var splashNoiseChars = []rune("01234567│╎|⁞")

// ── logo ─────────────────────────────────────────────────────────────────────

// Kban logo — embedded so the main binary has no runtime file dependency.
const splashLogoRaw = `'||              '||    .
 || ...    ...    ||  .||.  ... ...
 ||'  || .|  '|.  ||   ||    '|..'
 ||    | ||   ||  ||   ||     .|.
 '|...'   '|..|' .||.  '|.' .|  ||.`

var (
	splashLogoRows [][]rune
	splashLogoW    int
	splashLogoH    int
)

func init() {
	lines := strings.Split(splashLogoRaw, "\n")
	maxW := 0
	rLines := make([][]rune, len(lines))
	for i, l := range lines {
		rLines[i] = []rune(l)
		if len(rLines[i]) > maxW {
			maxW = len(rLines[i])
		}
	}
	splashLogoH = len(lines)
	splashLogoW = maxW
	splashLogoRows = make([][]rune, len(lines))
	for i, r := range rLines {
		row := make([]rune, maxW)
		copy(row, r)
		for j := len(r); j < maxW; j++ {
			row[j] = ' '
		}
		splashLogoRows[i] = row
	}
}

// ── message ───────────────────────────────────────────────────────────────────

type splashTickMsg struct{ gen int }

func splashTickCmd(gen int) tea.Cmd {
	return tea.Tick(splashTickDur, func(time.Time) tea.Msg {
		return splashTickMsg{gen: gen}
	})
}

// ── phases ────────────────────────────────────────────────────────────────────

const (
	splashReveal  = 0
	splashHold    = 1
	splashDismiss = 2
)

// ── init ──────────────────────────────────────────────────────────────────────

// initSplash precomputes column-rain scatter frames and resets all splash fields.
func (m *Model) initSplash() {
	h, w := splashLogoH, splashLogoW
	half := splashTargetFrames / 2
	rowStep := 1
	if h > 1 {
		rowStep = half / (h - 1)
	}
	colStart := make([]int, w)
	for c := range colStart {
		colStart[c] = rand.Intn(splashColJitter + 1)
	}
	scatter := make([][]int, h)
	rawMax := 0
	for r := 0; r < h; r++ {
		scatter[r] = make([]int, w)
		for c := 0; c < w; c++ {
			v := colStart[c] + r*rowStep
			scatter[r][c] = v
			if v > rawMax {
				rawMax = v
			}
		}
	}
	if rawMax > 0 {
		for r := range scatter {
			for c := range scatter[r] {
				scatter[r][c] = scatter[r][c] * splashTargetFrames / rawMax
			}
		}
	}
	m.splashScatter = scatter
	m.splashMaxReveal = splashTargetFrames
	m.splashFrame = 0
	m.splashPhase = splashReveal
	m.splashHoldCnt = 0
	m.splashColorIdx = 0
	m.splashColorTick = 0
}

// ── tick ──────────────────────────────────────────────────────────────────────

// advanceSplash steps the animation by one tick.
// Returns true when the splash is fully done and the main menu should appear.
func (m *Model) advanceSplash() (done bool) {
	threshold := m.splashMaxReveal + splashNoiseWin + 2

	switch m.splashPhase {
	case splashReveal:
		m.splashFrame++
		if m.splashFrame > threshold {
			m.splashPhase = splashHold
			m.splashFrame = 0
			m.splashHoldCnt = 0
			m.splashColorIdx = 0
			m.splashColorTick = 0
		}

	case splashHold:
		m.splashHoldCnt++
		ticksPerTheme := splashHoldTotal / len(Themes)
		m.splashColorTick++
		if m.splashColorTick >= ticksPerTheme {
			m.splashColorTick = 0
			m.splashColorIdx = (m.splashColorIdx + 1) % len(Themes)
			// shift the whole app theme so the box border cycles too
			m.themeIdx = m.splashColorIdx
			applyTheme(Themes[m.themeIdx])
		}
		if m.splashHoldCnt >= splashHoldTotal {
			// restore default theme before dissolve
			m.themeIdx = 0
			applyTheme(Themes[0])
			m.splashPhase = splashDismiss
			m.splashFrame = 0
		}

	case splashDismiss:
		m.splashFrame++
		if m.splashFrame > threshold {
			return true
		}
	}
	return false
}

// ── view ──────────────────────────────────────────────────────────────────────

// viewSplashPage renders the full-screen splash: centered box containing the
// animated Kban logo. Called instead of the normal View layout when page == pageSplash.
func (m Model) viewSplashPage() string {
	theme := Themes[m.themeIdx]
	ss := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	ns := lipgloss.NewStyle().Foreground(theme.Dim)

	// Render each logo line
	lines := make([]string, splashLogoH)
	for row := 0; row < splashLogoH; row++ {
		var sb strings.Builder
		for col := 0; col < splashLogoW; col++ {
			sb.WriteString(splashCell(m.splashPhase, m.splashFrame, m.splashMaxReveal,
				m.splashScatter, row, col, splashLogoRows[row][col], ss, ns))
		}
		lines[row] = sb.String()
	}
	logoBlock := strings.Join(lines, "\n")

	// Center logo within the inner box width
	centeredLogo := lipgloss.NewStyle().
		Width(splashInnerW).
		Align(lipgloss.Center).
		Render(logoBlock)

	// Add vertical padding
	vpad := strings.Repeat("\n", splashVPad)
	innerContent := lipgloss.NewStyle().
		PaddingLeft(2).PaddingRight(1).
		Render(vpad + centeredLogo + vpad)

	box := boxStyle.Render(innerContent)

	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Top,
			strings.Repeat("\n", m.stableTop)+box)
	}
	return box
}

// splashCell renders one character at (row, col) for the current animation state.
func splashCell(phase, frame, maxReveal int, scatter [][]int,
	row, col int, ch rune, ss, ns lipgloss.Style) string {

	if ch == ' ' {
		return " "
	}
	startR := scatter[row][col]
	settleR := startR + splashNoiseWin

	// Dissolve top-to-bottom: same order as reveal.
	startD := startR
	settleD := startD + splashNoiseWin

	switch phase {
	case splashReveal:
		if frame < startR {
			return " "
		}
		if frame < settleR {
			return ns.Render(string(splashNoise()))
		}
		return ss.Render(string(ch))
	case splashHold:
		return ss.Render(string(ch))
	case splashDismiss:
		if frame < startD {
			return ss.Render(string(ch))
		}
		if frame < settleD {
			return ns.Render(string(splashNoise()))
		}
		return " "
	}
	return " "
}

func splashNoise() rune {
	return splashNoiseChars[rand.Intn(len(splashNoiseChars))]
}

