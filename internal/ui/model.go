package ui

import (
	"fmt"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"boltx/internal/apply"
	"boltx/internal/detect"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	bubblesTable "github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ansiEscape matches ANSI SGR escape sequences so they can be stripped from
// rendered strings before re-applying a different color.
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// Key bindings shared across stages.
var (
	keyNav          = key.NewBinding(key.WithKeys("up", "down", "k", "j"), key.WithHelp("↑↓/jk", "navigate"))
	keyTabNav       = key.NewBinding(key.WithKeys("left", "right", "h", "l"), key.WithHelp("←→/hl", "tab"))
	keySelect       = key.NewBinding(key.WithKeys("enter", " "), key.WithHelp("enter/space", "toggle"))
	keyConfirm      = key.NewBinding(key.WithKeys("enter", " "), key.WithHelp("enter", "confirm"))
	keyBack         = key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q/esc", "back"))
	keyQuit         = key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q/esc", "quit"))
	keyHelpMore     = key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "more"))
	keyHelpLess     = key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "less"))
	keyTheme        = key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme"))
	keyResetOption  = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reset option"))
	keyRemoveSSHKey = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "remove key"))
	keyResetTab     = key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "reset tab"))
)

// maxOptionsPerPage is the maximum number of options shown per tab sub-page.
// When a category has more options than this, it gains extra sub-pages.
const maxOptionsPerPage = 8

type page int

const (
	pageSplash page = iota
	pageWelcome
	pageEnvironment
	pageQuickSetup
	pageReview
)

// OptionKind describes how an option is rendered and interacted with.
// Each kind maps to a specific bubbles component that owns input while active.
type OptionKind int

const (
	KindToggle    OptionKind = iota // checkbox on/off — no extra component
	KindTextInput                   // single-line text, backed by bubbles/textinput
	KindSelect                      // pick from a list; items stored in SelectItems
	KindCycle                       // cycles through SelectItems on space/enter; Value holds current
	KindPortList // expandable port rule list (NET tab firewall)
)

// portRuleKey identifies a firewall rule by its values, ignoring the Existing flag.
type portRuleKey struct{ From, To, Protocol string }

func portRuleKeySet(rules []apply.PortRule) map[portRuleKey]bool {
	set := make(map[portRuleKey]bool, len(rules))
	for _, r := range rules {
		set[portRuleKey{r.From, r.To, r.Protocol}] = true
	}
	return set
}

// netPreset describes a named group of port rules available in "Add presets".
type netPreset struct {
	Label string
	Rules []apply.PortRule
}

var netPresets = []netPreset{
	{"Web (80, 443)", []apply.PortRule{
		{From: "80", Protocol: "tcp"},
		{From: "443", Protocol: "tcp"},
	}},
	{"Minecraft (25565)", []apply.PortRule{
		{From: "25565", Protocol: "tcp"},
		{From: "25565", Protocol: "udp"},
	}},
	{"SSH (22)", []apply.PortRule{
		{From: "22", Protocol: "tcp"},
	}},
}

// UserEntry holds per-user configuration for the USR tab.
type UserEntry struct {
	Name            string
	OriginalName    string // system name at load time; empty for new users
	Password        string // used for new user creation only
	OldPassword     string // current password, required on macOS to re-encrypt Secure Token
	NewPassword     string // used for existing user password change (deferred)
	Sudo            bool
	OriginalSudo    bool     // sudo state at load time, for delta apply
	SSHKeys         []string // current key list (new + existing minus removed)
	OriginalSSHKeys []string // loaded from authorized_keys at sync time; used to compute delta at GO!
	Existing        bool     // loaded from the system, not pending creation
	ActiveSession   bool     // user has running processes; username cannot be changed
	PendingDelete   bool     // existing user marked for deletion at apply time
}

// Per-user option cursor positions within the USR tab body.
const (
	usrOptUsername = 0
	usrOptPassword = 1
	usrOptSudo     = 2
	usrOptSSHKey   = 3
	usrOptDelete   = 4
	usrOptCount    = 5
)

// Apply priority tiers — lower value executes first in GO!
const (
	PrioPackageInstall = 10 // PKG tab: install packages
	PrioConfigWrite    = 20 // config file writes (SYS, SEC, NET)
	PrioServiceRestart = 30 // service enable/restart (sshd, fail2ban)
	PrioFirewallRule   = 40 // UFW rule changes
)

// CategoryOption is a single setting within a category page.
type CategoryOption struct {
	Label            string
	Kind             OptionKind
	Checked          bool               // will this option be applied?
	Default          string             // detected current value (shown as placeholder)
	Value            string             // user-supplied value; empty → use Default on apply
	ApplyFn          func(string) error // deferred to GO! tab; called when Checked=true
	UndoFn           func(string) error // deferred to GO! tab; called when Checked=false (reverts the option)
	OriginalChecked  bool               // Checked state at sync time; delta apply skips if Checked==OriginalChecked
	NeedsRoot        bool               // if true, hidden when not running as root
	SelectItems      []string           // valid choices for KindSelect; populated at build time
	ClearOnEdit      bool               // KindTextInput: open with empty field (placeholder = Default) instead of pre-filling Value
	ValidateFn       func(string) error // optional: called on confirm; blocks save if non-nil error
	Priority         int                // GO! execution order; lower = earlier; 0 treated as PrioConfigWrite
	PortRules         []apply.PortRule // KindPortList: current list of port rules (existing + new)
	DetectedPortRules []apply.PortRule // KindPortList: rules detected from live system at sync time
}

// CategoryPage groups related options under a category name.
type CategoryPage struct {
	Name        string
	Icon        string
	Options     []CategoryOption
	UserEntries []UserEntry // USR tab only — users to be created
	Synced      bool        // true after first syncOnTabEnter; prevents overwriting user changes
}

// subPageCount returns the number of sub-pages needed for nOptions.
func subPageCount(nOptions int) int {
	if nOptions == 0 {
		return 1
	}
	return (nOptions + maxOptionsPerPage - 1) / maxOptionsPerPage
}

// buildCategoryPages returns all category pages with defaults pre-filled for the given use case.
// Options marked NeedsRoot are omitted when osInfo.IsRoot is false.
func buildCategoryPages(uc detect.UseCase, osInfo detect.OSInfo) []CategoryPage {
	filter := func(opts []CategoryOption) []CategoryOption {
		if osInfo.IsRoot {
			return opts
		}
		out := opts[:0:0]
		for _, o := range opts {
			if !o.NeedsRoot {
				out = append(out, o)
			}
		}
		return out
	}
	return []CategoryPage{
		{
			Name: "SYS",
			Options: filter([]CategoryOption{
				{
					Label:     "Hostname",
					Kind:      KindTextInput,
					Default:   osInfo.Hostname,
					NeedsRoot: true,
					Priority:  PrioConfigWrite,
					ApplyFn:   func(v string) error { return apply.Hostname(v) },
				},
				{
					Label:       "Locale",
					Kind:        KindSelect,
					Default:     osInfo.Locale,
					NeedsRoot:   true,
					Priority:    PrioConfigWrite,
					SelectItems: detect.DetectLocales(),
					ApplyFn:     func(v string) error { return apply.Locale(v) },
				},
				{
					Label:       "Timezone",
					Kind:        KindSelect,
					Default:     osInfo.Timezone,
					NeedsRoot:   true,
					Priority:    PrioConfigWrite,
					SelectItems: detect.DetectTimezones(),
					ApplyFn:     func(v string) error { return apply.Timezone(v) },
				},
			}),
		},
		{
			Name:        "USR",
			Options:     []CategoryOption{},
			UserEntries: []UserEntry{},
		},
		{
			Name: "SEC",
			Options: filter([]CategoryOption{
				{
					Label:       "SSH: Permit root login",
					Kind:        KindCycle,
					Checked:     true,
					NeedsRoot:   true,
					SelectItems: []string{"no", "prohibit-password", "forced-commands-only", "yes"},
					Value:       "no",
					Priority:    PrioConfigWrite,
					ApplyFn:     func(v string) error { return apply.ApplySSHOption("PermitRootLogin", v) },
				},
				{
					Label:     "SSH: Require key auth only",
					Kind:      KindToggle,
					Checked:   uc == detect.UseCaseVPS,
					NeedsRoot: true,
					Priority:  PrioConfigWrite,
					ApplyFn:   func(_ string) error { return apply.DisablePasswordAuth() },
					UndoFn:    func(_ string) error { return apply.EnablePasswordAuth() },
				},
				{
					Label:       "SSH: Port",
					Kind:        KindTextInput,
					Checked:     false,
					NeedsRoot:   true,
					Default:     "22",
					ClearOnEdit: true,
					Priority:    PrioConfigWrite,
					ValidateFn:  func(v string) error { return apply.ValidatePort(v) },
					ApplyFn:     func(v string) error { return apply.ApplySSHPort(v) },
				},
				{
					Label:     "Enable UFW firewall",
					Kind:      KindToggle,
					Checked:   uc == detect.UseCaseVPS,
					NeedsRoot: true,
					Priority:  PrioFirewallRule,
					ApplyFn:   func(_ string) error { return apply.EnableFirewall() },
					UndoFn:    func(_ string) error { return apply.DisableFirewall() },
				},
			}),
		},
		{
			Name: "NET",
			Options: filter([]CategoryOption{
				{
					Label:     "FW: Open ports",
					Kind:      KindPortList,
					NeedsRoot: true,
					Priority:  PrioFirewallRule,
				},
				{
					Label:       "Proxy",
					Kind:        KindCycle,
					SelectItems: []string{"none", "traefik", "nginx"},
					Default:     "none",
					Value:       "none",
					Priority:    PrioConfigWrite,
					ApplyFn:     func(v string) error { return apply.SetProxyManager(v) },
				},
				{
					Label:     "Enable fail2ban",
					Kind:      KindToggle,
					NeedsRoot: true,
					Priority:  PrioServiceRestart,
					ApplyFn:   func(_ string) error { return apply.EnableFail2ban() },
					UndoFn:    func(_ string) error { return apply.DisableFail2ban() },
				},
				{
					Label:       "fail2ban: Preset",
					Kind:        KindCycle,
					SelectItems: []string{"ssh", "web", "all"},
					Default:     "ssh",
					Value:       "ssh",
					Priority:    PrioConfigWrite,
					ApplyFn:     func(v string) error { return apply.WriteFail2banConfig(v) },
				},
			}),
		},
		{
			Name: "PKG",
			Options: []CategoryOption{
				{Label: "Placeholder A", Kind: KindToggle, Checked: true, ApplyFn: func(_ string) error { return nil }},
				{Label: "Placeholder B", Kind: KindToggle, Checked: true, ApplyFn: func(_ string) error { return nil }},
				{Label: "Placeholder C", Kind: KindToggle, Checked: true, ApplyFn: func(_ string) error { return nil }},
			},
		},
		{
			Name: "RUN",
			Options: []CategoryOption{
				{Label: "Placeholder A", Kind: KindToggle, Checked: true, ApplyFn: func(_ string) error { return nil }},
			},
		},
		{
			Name:    "GO!",
			Options: []CategoryOption{},
		},
	}
}

// detectDoneMsg carries the result of the async environment detection.
type detectDoneMsg struct {
	env    detect.Environment
	osInfo detect.OSInfo
}

// applyState tracks the lifecycle of the GO! apply pass.
type applyState int

const (
	applyIdle    applyState = iota // waiting for user to confirm
	applyRunning                   // background apply in progress
	applyDone                      // apply finished; results available
)

// applyResult holds the outcome of one option's ApplyFn call.
type applyResult struct {
	label string
	err   error
}

// applyDoneMsg is returned by doApplyAll when every ApplyFn has been called.
type applyDoneMsg struct {
	results []applyResult
}

// Model holds all TUI state.
type Model struct {
	width  int
	height int

	page       page
	menuCursor int

	detecting     bool
	spinner       spinner.Model
	env           detect.Environment
	osInfo        detect.OSInfo
	useCaseCursor int
	infoTable     bubblesTable.Model
	help          help.Model
	helpExpanded  bool

	// pageQuickSetup
	applyOrReviewCursor int

	// pageReview
	categoryPages      []CategoryPage
	activeTab          int // which category tab is visible
	tabSubPage         int // sub-page within the active tab (for overflow)
	categoryPageCursor int // cursor position within the current sub-page

	// Option editing — active while a KindTextInput is being edited.
	// Only one option can be edited at a time.
	editingOption bool
	textInput     textinput.Model
	// Multi-step input — used by the USR tab's username/password flows.
	// Steps: 0=main value, 1=password, 2=confirm.
	inputSubStep int
	subValues    [3]string
	inputError   string

	// USR tab state — user sub-tabs and per-user editing.
	usrSubTab            int    // index into UserEntries; len(UserEntries) = "+ New User" tab
	usrTabOffset         int    // first visible entry index in the sub-tab bar
	usrEditingField      int    // which per-user field is being edited (usrOpt* constants)
	usrEditingSSHList    bool   // SSH key list sub-mode is open for current user
	usrSSHListCursor     int    // cursor in SSH list: 0..len(items)-1 = key; len(items) = "Add new key"
	usrSSHEditingOrigKey string // non-empty when editing an existing key (holds the original key being replaced)

	// NET tab state — firewall port list editing.
	netPortListOpen      bool   // port rule sub-list is open
	netPortListCursor    int    // cursor: 0..N-1=rules, N="Add port", N+1="Add presets"
	netPortEditing       bool   // port edit submenu is open
	netPortEditIdx       int    // index of rule being edited; -1 = new rule
	netPortSubMenuCursor int    // cursor within port-edit submenu
	netPortSubMenuType   string // "single" or "range"
	netPortPresetsOpen   bool   // preset sublist is expanded inside port list
	netPortPresetCursor  int    // cursor within preset sublist (0..len(netPresets))
	netPortPresetSel     []bool // which presets are toggled (len = len(netPresets))

	// Option selecting — active while a KindSelect picker is open.
	selectingOption bool
	selectItems     []string // items shown in the picker
	selectCursor    int      // index of highlighted item
	selectViewport  int      // index of first visible item

	// GO! apply pass.
	applyState   applyState
	applyResults []applyResult

	// Theme cycling — index into Themes slice, advanced by 't'.
	themeIdx int

	ringBell   bool // cleared after one render cycle
	visualBell bool // flashes blocked row for 150ms

	// Splash animation state
	splashGen        int
	splashFrame      int
	splashPhase      int
	splashHoldCnt    int
	splashColorIdx   int
	splashColorTick  int
	splashScatter    [][]int
	splashMaxReveal  int

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
	"Firewall, SSH hardening",
	"Dev tools and common packages",
}

var applyOrReviewOptions = []string{
	"Recommended setup",
	"Custom setup",
}

// NewModel returns the initial model.
func NewModel() Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(Themes[0].Accent)

	h := help.New()
	h.Styles.ShortKey = lipgloss.NewStyle().Foreground(Themes[0].Text)
	h.Styles.ShortDesc = lipgloss.NewStyle().Foreground(Themes[0].Muted)
	h.Styles.ShortSeparator = lipgloss.NewStyle().Foreground(Themes[0].Muted)
	h.Styles.Ellipsis = lipgloss.NewStyle().Foreground(Themes[0].Muted)
	h.Styles.FullKey = lipgloss.NewStyle().Foreground(Themes[0].Text)
	h.Styles.FullDesc = lipgloss.NewStyle().Foreground(Themes[0].Muted)
	h.Styles.FullSeparator = lipgloss.NewStyle().Foreground(Themes[0].Muted)

	m := Model{
		detecting: true,
		spinner:   s,
		help:      h,
	}
	m.initSplash()
	return m
}

// Init fires environment and OS detection as soon as the program starts.
func (m Model) Init() tea.Cmd {
	return tea.Batch(doDetect, m.spinner.Tick, splashTickCmd(m.splashGen))
}

// doDetect runs detection in the background and returns the result as a message.
func doDetect() tea.Msg {
	return detectDoneMsg{
		env:    detect.Detect(),
		osInfo: detect.DetectOS(),
	}
}

// newPasswordInput returns a focused password textinput with the given prompt.
func newPasswordInput(prompt string) textinput.Model {
	ti := textinput.New()
	ti.Prompt = prompt
	ti.EchoMode = textinput.EchoPassword
	ti.Focus()
	return ti
}

// applyAction is a single deferred operation collected by doApplyAll.
// fn returns zero or more results (user ops may produce several steps).
type applyAction struct {
	priority int
	fn       func() []applyResult
}

// effectivePriority returns the priority to use, treating 0 as PrioConfigWrite.
func effectivePriority(p int) int {
	if p == 0 {
		return PrioConfigWrite
	}
	return p
}

// doApplyAll collects every queued action across all tabs, sorts by priority,
// and executes them in order, returning all results as an applyDoneMsg.
func doApplyAll(pages []CategoryPage) tea.Cmd {
	return func() tea.Msg {
		var actions []applyAction

		for _, pg := range pages {
			// USR tab: user operations run as a sequential group at PrioConfigWrite.
			// Internal ordering (delete → create → rename → password/sudo/SSH) is preserved.
			if len(pg.UserEntries) > 0 {
				entries := pg.UserEntries
				actions = append(actions, applyAction{
					priority: PrioConfigWrite,
					fn: func() []applyResult {
						return applyUserEntries(entries)
					},
				})
			}

			// Generic options sorted individually by their Priority field.
			for _, opt := range pg.Options {
				opt := opt
				v := opt.Value
				if v == "" {
					v = opt.Default
				}
				p := effectivePriority(opt.Priority)
				switch opt.Kind {
				case KindPortList:
					// Compute delta: add new rules, delete removed existing rules.
					var toAdd []apply.PortRule
					for _, r := range opt.PortRules {
						if !r.Existing {
							toAdd = append(toAdd, r)
						}
					}
					current := portRuleKeySet(opt.PortRules)
					var toRemove []apply.PortRule
					for _, r := range opt.DetectedPortRules {
						if !current[portRuleKey{r.From, r.To, r.Protocol}] {
							toRemove = append(toRemove, r)
						}
					}
					if len(toAdd) > 0 || len(toRemove) > 0 {
						add, remove, lbl := toAdd, toRemove, opt.Label
						actions = append(actions, applyAction{
							priority: p,
							fn: func() []applyResult {
								var res []applyResult
								if err := apply.ApplyFirewallRules(add); err != nil {
									res = append(res, applyResult{label: lbl + " (add)", err: err})
								}
								if err := apply.DeleteFirewallRules(remove); err != nil {
									res = append(res, applyResult{label: lbl + " (remove)", err: err})
								}
								if len(res) == 0 {
									res = append(res, applyResult{label: lbl})
								}
								return res
							},
						})
					}
				case KindCycle:
					if opt.ApplyFn != nil && opt.Value != opt.Default {
						actions = append(actions, applyAction{
							priority: p,
							fn: func() []applyResult {
								return []applyResult{{label: opt.Label, err: opt.ApplyFn(v)}}
							},
						})
					}
				default:
					if opt.UndoFn != nil {
						if opt.Checked && !opt.OriginalChecked && opt.ApplyFn != nil {
							actions = append(actions, applyAction{
								priority: p,
								fn: func() []applyResult {
									return []applyResult{{label: opt.Label, err: opt.ApplyFn(v)}}
								},
							})
						} else if !opt.Checked && opt.OriginalChecked {
							actions = append(actions, applyAction{
								priority: p,
								fn: func() []applyResult {
									return []applyResult{{label: opt.Label + " (revert)", err: opt.UndoFn(v)}}
								},
							})
						}
					} else if opt.Checked && opt.ApplyFn != nil {
						actions = append(actions, applyAction{
							priority: p,
							fn: func() []applyResult {
								return []applyResult{{label: opt.Label, err: opt.ApplyFn(v)}}
							},
						})
					}
				}
			}
		}

		sort.SliceStable(actions, func(i, j int) bool {
			return actions[i].priority < actions[j].priority
		})

		var results []applyResult
		for _, a := range actions {
			results = append(results, a.fn()...)
		}
		return applyDoneMsg{results: results}
	}
}

// applyUserEntries applies all queued user operations in the correct sequence.
// Internal ordering is preserved: delete → create → existing-user delta.
func applyUserEntries(entries []UserEntry) []applyResult {
	var results []applyResult
	for _, u := range entries {
		if u.Existing && u.PendingDelete {
			err := apply.DeleteUser(u.Name)
			results = append(results, applyResult{label: "Delete user: " + u.Name, err: err})
			continue
		}
		if !u.Existing {
			err := apply.CreateUser(u.Name, u.Password)
			results = append(results, applyResult{label: "Create user: " + u.Name, err: err})
			if err != nil {
				continue
			}
			if u.Sudo {
				err = apply.AddSudo(u.Name)
				results = append(results, applyResult{label: "Add sudo: " + u.Name, err: err})
			}
			for _, k := range u.SSHKeys {
				err = apply.AddSSHKey(u.Name, k)
				results = append(results, applyResult{label: "SSH key (" + apply.SSHKeyComment(k) + "): " + u.Name, err: err})
			}
			continue
		}
		// Existing user: rename first, then password/sudo/SSH delta.
		if u.OriginalName != "" && u.Name != u.OriginalName {
			err := apply.RenameUser(u.OriginalName, u.Name)
			results = append(results, applyResult{label: "Rename user: " + u.OriginalName + "→" + u.Name, err: err})
			if err != nil {
				continue
			}
		}
		if u.NewPassword != "" {
			err := apply.ChangePassword(u.Name, u.OldPassword, u.NewPassword)
			results = append(results, applyResult{label: "Change password: " + u.Name, err: err})
		}
		if u.Sudo && !u.OriginalSudo {
			err := apply.AddSudo(u.Name)
			results = append(results, applyResult{label: "Add sudo: " + u.Name, err: err})
		} else if !u.Sudo && u.OriginalSudo {
			err := apply.RemoveSudo(u.Name)
			results = append(results, applyResult{label: "Remove sudo: " + u.Name, err: err})
		}
		origSet := map[string]bool{}
		for _, k := range u.OriginalSSHKeys {
			origSet[k] = true
		}
		newSet := map[string]bool{}
		for _, k := range u.SSHKeys {
			newSet[k] = true
		}
		for _, k := range u.SSHKeys {
			if !origSet[k] {
				err := apply.AddSSHKey(u.Name, k)
				results = append(results, applyResult{label: "SSH key add (" + apply.SSHKeyComment(k) + "): " + u.Name, err: err})
			}
		}
		for _, k := range u.OriginalSSHKeys {
			if !newSet[k] {
				err := apply.RemoveSSHKey(u.Name, k)
				results = append(results, applyResult{label: "SSH key remove (" + apply.SSHKeyComment(k) + "): " + u.Name, err: err})
			}
		}
	}
	return results
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m = computeLayout(m)

	case splashTickMsg:
		if msg.gen != m.splashGen {
			return m, nil
		}
		if done := m.advanceSplash(); done {
			m.page = pageWelcome
			return m, nil
		}
		return m, splashTickCmd(m.splashGen)

	case spinner.TickMsg:
		if m.detecting || m.applyState == applyRunning {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case bellClearedMsg:
		m.ringBell = false
		m.visualBell = false
		return m, nil

	case applyDoneMsg:
		m.applyState = applyDone
		m.applyResults = msg.results
		return m, nil

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
		// Any key press skips the splash.
		if m.page == pageSplash {
			m.page = pageWelcome
			m.themeIdx = 0
			applyTheme(Themes[0])
			m.splashGen++ // stop the running splash tick stream
			return m, nil
		}
		// While a KindSelect picker is open, intercept navigation keys.
		if m.selectingOption {
			switch msg.String() {
			case "up", "k":
				if m.selectCursor > 0 {
					m.selectCursor--
					if m.selectCursor < m.selectViewport {
						m.selectViewport = m.selectCursor
					}
				}
			case "down", "j":
				if m.selectCursor < len(m.selectItems)-1 {
					m.selectCursor++
					if m.selectCursor >= m.selectViewport+visibleItems {
						m.selectViewport = m.selectCursor - visibleItems + 1
					}
				}
			case "enter", " ":
				absIdx := m.tabSubPage*maxOptionsPerPage + m.categoryPageCursor
				opt := &m.categoryPages[m.activeTab].Options[absIdx]
				opt.Value = m.selectItems[m.selectCursor]
				opt.Checked = true
				m.selectingOption = false
			case "esc", "q":
				m.selectingOption = false
			default:
				// Letter key: jump to first item whose prefix matches (e.g. "e" → Europe/...).
				if k := msg.String(); len(k) == 1 {
					kl := strings.ToLower(k)
					for i, item := range m.selectItems {
						if strings.HasPrefix(strings.ToLower(item), kl) {
							m.selectCursor = i
							m.selectViewport = max(0, m.selectCursor-visibleItems/2)
							break
						}
					}
				}
			}
			return m, nil
		}

		// SSH key list sub-mode: navigate/add/remove keys before entering text input.
		if m.activeTab == tabIndexUSR && m.usrEditingSSHList && !m.editingOption {
			nUsers := len(m.categoryPages[tabIndexUSR].UserEntries)
			if m.usrSubTab < nUsers {
				user := &m.categoryPages[tabIndexUSR].UserEntries[m.usrSubTab]
				items := sshKeyDisplayItems(*user)
				addIdx := len(items)
				switch msg.String() {
				case "up", "k":
					if m.usrSSHListCursor > 0 {
						m.usrSSHListCursor--
					}
				case "down", "j":
					if m.usrSSHListCursor < addIdx {
						m.usrSSHListCursor++
					}
				case "r":
					if m.usrSSHListCursor < addIdx {
						item := items[m.usrSSHListCursor]
						if item.pendingRemove {
							// undo: restore key to active set
							user.SSHKeys = append(user.SSHKeys, item.key)
						} else if item.existing {
							// flag for removal: drop from SSHKeys (stays visible as pending)
							for i, k := range user.SSHKeys {
								if k == item.key {
									user.SSHKeys = append(user.SSHKeys[:i], user.SSHKeys[i+1:]...)
									break
								}
							}
						} else {
							// newly added key: discard entirely, clamp cursor
							for i, k := range user.SSHKeys {
								if k == item.key {
									user.SSHKeys = append(user.SSHKeys[:i], user.SSHKeys[i+1:]...)
									break
								}
							}
							newLen := len(sshKeyDisplayItems(*user))
							if m.usrSSHListCursor > newLen {
								m.usrSSHListCursor = newLen
							}
						}
					}
				case "enter", " ":
					ti := textinput.New()
					ti.Focus()
					m.usrEditingField = usrOptSSHKey
					m.inputError = ""
					if m.usrSSHListCursor < addIdx && !items[m.usrSSHListCursor].pendingRemove {
						item := items[m.usrSSHListCursor]
						ti.Prompt = "Edit key:  "
						ti.SetValue(item.key)
						m.usrSSHEditingOrigKey = item.key
					} else if m.usrSSHListCursor == addIdx {
						ti.Prompt = "Paste key: "
						m.usrSSHEditingOrigKey = ""
					} else {
						break // pending-remove row: enter does nothing
					}
					m.textInput = ti
					m.editingOption = true
				case "esc", "q":
					m.usrEditingSSHList = false
					m.inputError = ""
				case "?":
					m.helpExpanded = !m.helpExpanded
				}
			}
			return m, nil
		}

		// NET tab: preset sublist navigation (expanded inside port list).
		if m.activeTab == tabIndexNET && m.netPortListOpen && m.netPortPresetsOpen {
			confirmIdx := len(netPresets)
			switch msg.String() {
			case "up", "k":
				if m.netPortPresetCursor > 0 {
					m.netPortPresetCursor--
				}
			case "down", "j":
				if m.netPortPresetCursor < confirmIdx {
					m.netPortPresetCursor++
				}
			case " ", "enter":
				if m.netPortPresetCursor < confirmIdx {
					m.netPortPresetSel[m.netPortPresetCursor] = !m.netPortPresetSel[m.netPortPresetCursor]
				} else {
					// Confirm: add selected preset rules (skip duplicates).
					opt := &m.categoryPages[tabIndexNET].Options[0]
					current := portRuleKeySet(opt.PortRules)
					for i, p := range netPresets {
						if !m.netPortPresetSel[i] {
							continue
						}
						for _, r := range p.Rules {
							k := portRuleKey{r.From, r.To, r.Protocol}
							if !current[k] {
								opt.PortRules = append(opt.PortRules, r)
								current[k] = true
							}
						}
					}
					m.netPortPresetsOpen = false
					m.netPortPresetSel = nil
					m.netPortListCursor = len(opt.PortRules) - 1
					if m.netPortListCursor < 0 {
						m.netPortListCursor = 0
					}
				}
			case "esc":
				m.netPortPresetsOpen = false
				m.netPortPresetSel = nil
			case "?":
				m.helpExpanded = !m.helpExpanded
			}
			return m, nil
		}

		// NET tab: port list navigation (open, not in submenu or preset sublist).
		if m.activeTab == tabIndexNET && m.netPortListOpen && !m.netPortEditing {
			opt := &m.categoryPages[tabIndexNET].Options[0]
			// cursor 0..N-1 = rules; N = "Add port"; N+1 = "Add presets"
			addPortIdx := len(opt.PortRules)
			addPresetsIdx := len(opt.PortRules) + 1
			switch msg.String() {
			case "up", "k":
				if m.netPortListCursor > 0 {
					m.netPortListCursor--
				}
			case "down", "j":
				if m.netPortListCursor < addPresetsIdx {
					m.netPortListCursor++
				}
			case "r":
				if m.netPortListCursor < addPortIdx {
					opt.PortRules = append(opt.PortRules[:m.netPortListCursor], opt.PortRules[m.netPortListCursor+1:]...)
					if m.netPortListCursor > len(opt.PortRules) {
						m.netPortListCursor = len(opt.PortRules)
					}
				}
			case "e", "enter", " ":
				switch m.netPortListCursor {
				case addPortIdx:
					m = m.startPortEdit(-1)
				case addPresetsIdx:
					m.netPortPresetsOpen = true
					m.netPortPresetCursor = 0
					m.netPortPresetSel = make([]bool, len(netPresets))
				default:
					m = m.startPortEdit(m.netPortListCursor)
				}
			case "esc":
				m.netPortListOpen = false
				m.inputError = ""
			case "?":
				m.helpExpanded = !m.helpExpanded
			}
			return m, nil
		}

		// NET tab: port edit submenu navigation (submenu open, no text field focused).
		if m.activeTab == tabIndexNET && m.netPortEditing && !m.editingOption {
			switch msg.String() {
			case "up", "k":
				if m.netPortSubMenuCursor > 0 {
					m.netPortSubMenuCursor--
				}
			case "down", "j":
				if m.netPortSubMenuCursor < m.portSubMenuConfirmIdx() {
					m.netPortSubMenuCursor++
				}
			case "enter", " ":
				protoIdx := m.portSubMenuProtoIdx()
				confirmIdx := m.portSubMenuConfirmIdx()
				switch m.netPortSubMenuCursor {
				case 0: // Type cycle
					if m.netPortSubMenuType == "single" {
						m.netPortSubMenuType = "range"
					} else {
						m.netPortSubMenuType = "single"
						m.subValues[1] = ""
						if m.netPortSubMenuCursor > m.portSubMenuConfirmIdx() {
							m.netPortSubMenuCursor = m.portSubMenuConfirmIdx()
						}
					}
				case 1: // Port/From text field
					ti := textinput.New()
					ti.Prompt = ""
					ti.Width = 6
					ti.SetValue(m.subValues[0])
					ti.Focus()
					m.textInput = ti
					m.editingOption = true
					m.inputError = ""
				case 2: // To text field (range mode only)
					if m.netPortSubMenuType == "range" {
						ti := textinput.New()
						ti.Prompt = ""
						ti.Width = 6
						ti.SetValue(m.subValues[1])
						ti.Focus()
						m.textInput = ti
						m.editingOption = true
						m.inputError = ""
					}
				default:
					if m.netPortSubMenuCursor == protoIdx {
						protos := []string{"tcp", "udp", "both"}
						cur := m.subValues[2]
						if cur == "" {
							cur = "tcp"
						}
						for i, p := range protos {
							if p == cur {
								m.subValues[2] = protos[(i+1)%len(protos)]
								break
							}
						}
					} else if m.netPortSubMenuCursor == confirmIdx {
						m = m.confirmPortEdit()
					}
				}
			case "esc":
				m = m.cancelPortEdit()
			case "?":
				m.helpExpanded = !m.helpExpanded
			}
			return m, nil
		}

		// While a text input is open, route to the right context.
		// USR tab has its own multi-step flows; other tabs use the standard KindTextInput path.
		// Only Enter and Esc are handled specially here.
		if m.editingOption {
			switch msg.String() {
			case "enter":
				val := m.textInput.Value()
				m.inputError = ""
				if m.activeTab == tabIndexUSR {
					nUsers := len(m.categoryPages[tabIndexUSR].UserEntries)
					if m.usrSubTab == nUsers {
						// New User flow: step 0=username, 1=password, 2=confirm.
						switch m.inputSubStep {
						case 0:
							if val == "" {
								m.inputError = "username cannot be empty"
								return m, nil
							}
							if err := apply.ValidateUsername(val); err != nil {
								m.inputError = err.Error()
								return m, nil
							}
							for _, u := range m.categoryPages[tabIndexUSR].UserEntries {
								if u.Name == val {
									m.inputError = "user \"" + val + "\" already in list"
									return m, nil
								}
							}
							if apply.UserExists(val) {
								m.inputError = "user \"" + val + "\" already exists on system"
								return m, nil
							}
							m.subValues[0] = val
							m.inputSubStep = 1
							m.textInput = newPasswordInput("Password:  ")
						case 1:
							if val == "" {
								m.inputError = "password cannot be empty"
								return m, nil
							}
							m.subValues[1] = val
							m.inputSubStep = 2
							m.textInput = newPasswordInput("Confirm:   ")
						case 2:
							if val != m.subValues[1] {
								m.inputError = "passwords do not match"
								m.inputSubStep = 1
								m.textInput = newPasswordInput("Password:  ")
								return m, nil
							}
							m.categoryPages[tabIndexUSR].UserEntries = append(
								m.categoryPages[tabIndexUSR].UserEntries,
								UserEntry{Name: m.subValues[0], Password: m.subValues[1]},
							)
							m.usrSubTab = len(m.categoryPages[tabIndexUSR].UserEntries) - 1
							m.editingOption = false
							m.inputSubStep = 0
							m.subValues = [3]string{}
							m.categoryPageCursor = 0
						}
					} else {
						// Editing an existing user's field.
						user := &m.categoryPages[tabIndexUSR].UserEntries[m.usrSubTab]
						switch m.usrEditingField {
						case usrOptUsername:
							if val == "" {
								m.inputError = "username cannot be empty"
								return m, nil
							}
							if err := apply.ValidateUsername(val); err != nil {
								m.inputError = err.Error()
								return m, nil
							}
							for i, u := range m.categoryPages[tabIndexUSR].UserEntries {
								if u.Name == val && i != m.usrSubTab {
									m.inputError = "user \"" + val + "\" already in list"
									return m, nil
								}
							}
							if apply.UserExists(val) {
								m.inputError = "user \"" + val + "\" already exists on system"
								return m, nil
							}
							user.Name = val
							m.editingOption = false
						case usrOptPassword:
							if !user.Existing {
								// New user: step 1=password, step 2=confirm.
								switch m.inputSubStep {
								case 1:
									if val == "" {
										m.inputError = "password cannot be empty"
										return m, nil
									}
									m.subValues[1] = val
									m.inputSubStep = 2
									m.textInput = newPasswordInput("Confirm:   ")
								case 2:
									if val != m.subValues[1] {
										m.inputError = "passwords do not match"
										m.inputSubStep = 1
										m.textInput = newPasswordInput("New password: ")
										return m, nil
									}
									user.Password = m.subValues[1]
									m.editingOption = false
									m.inputSubStep = 0
									m.subValues = [3]string{}
								}
							} else {
								// Existing user: step 0=old (macOS only), 1=new, 2=confirm.
								switch m.inputSubStep {
								case 0:
									m.subValues[0] = val
									m.inputSubStep = 1
									m.textInput = newPasswordInput("New password: ")
								case 1:
									if val == "" {
										m.inputError = "password cannot be empty"
										return m, nil
									}
									m.subValues[1] = val
									m.inputSubStep = 2
									m.textInput = newPasswordInput("Confirm:   ")
								case 2:
									if val != m.subValues[1] {
										m.inputError = "passwords do not match"
										m.inputSubStep = 1
										m.textInput = newPasswordInput("New password: ")
										return m, nil
									}
									user.OldPassword = m.subValues[0]
									user.NewPassword = m.subValues[1]
									m.editingOption = false
									m.inputSubStep = 0
									m.subValues = [3]string{}
								}
							}
						case usrOptSSHKey:
							if err := apply.ValidateSSHKey(val); err != nil {
								m.inputError = err.Error()
								return m, nil
							}
							// dup check: ignore the key being replaced
							for _, k := range user.SSHKeys {
								if k == val && k != m.usrSSHEditingOrigKey {
									m.textInput.SetValue("")
									m.inputError = "already added — paste a different key"
									return m, nil
								}
							}
							if m.usrSSHEditingOrigKey != "" {
								// edit: replace old key with new
								for i, k := range user.SSHKeys {
									if k == m.usrSSHEditingOrigKey {
										user.SSHKeys[i] = val
										break
									}
								}
								m.usrSSHEditingOrigKey = ""
							} else {
								user.SSHKeys = append(user.SSHKeys, val)
							}
							m.usrSSHListCursor = len(sshKeyDisplayItems(*user)) // point to "Add new key"
							m.editingOption = false
							// stay in usrEditingSSHList mode
						}
					}
				} else if m.activeTab == tabIndexNET && m.netPortEditing {
					// NET tab: validate and save text field in port-edit submenu.
					val := m.textInput.Value()
					switch m.netPortSubMenuCursor {
					case 1: // Port/From
						if err := apply.ValidateNetPort(val); err != nil {
							m.inputError = err.Error()
							return m, nil
						}
						m.subValues[0] = val
						m.editingOption = false
						m.inputError = ""
					case 2: // To (range mode)
						if val != "" {
							if err := apply.ValidatePortRange(m.subValues[0], val); err != nil {
								m.inputError = err.Error()
								return m, nil
							}
						}
						m.subValues[1] = val
						m.editingOption = false
						m.inputError = ""
					}
				} else {
					absIdx := m.tabSubPage*maxOptionsPerPage + m.categoryPageCursor
					opt := &m.categoryPages[m.activeTab].Options[absIdx]
					val := m.textInput.Value()
					if opt.ValidateFn != nil {
						if err := opt.ValidateFn(val); err != nil {
							m.inputError = err.Error()
							break
						}
					}
					opt.Value = val
					opt.Checked = true
					m.editingOption = false
				}
			case "esc":
				m.editingOption = false
				m.inputError = ""
				m.usrSSHEditingOrigKey = ""
				// For port submenu, esc just unfocuses the field; keep subValues intact.
				if !m.netPortEditing {
					m.inputSubStep = 0
					m.subValues = [3]string{}
				}
				// If we were adding/editing a key inside the SSH list, stay in list mode.
				// (usrEditingSSHList remains true; the list handler takes over next key.)
			default:
				var cmd tea.Cmd
				m.textInput, cmd = m.textInput.Update(msg)
				return m, cmd
			}
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit

		case "?":
			m.helpExpanded = !m.helpExpanded

		case "t":
			m.themeIdx = (m.themeIdx + 1) % len(Themes)
			applyTheme(Themes[m.themeIdx])

		case "r":
			if m.page == pageReview && !m.editingOption {
				if m.activeTab == tabIndexUSR {
					nUsers := len(m.categoryPages[tabIndexUSR].UserEntries)
					if m.usrSubTab < nUsers {
						user := &m.categoryPages[tabIndexUSR].UserEntries[m.usrSubTab]
						switch m.categoryPageCursor {
						case usrOptUsername:
							if user.Existing {
								user.Name = user.OriginalName
							} else {
								user.Name = ""
							}
						case usrOptPassword:
							if user.Existing {
								user.OldPassword = ""
								user.NewPassword = ""
							} else {
								user.Password = ""
							}
						case usrOptSudo:
							if user.Existing {
								user.Sudo = user.OriginalSudo
							} else {
								user.Sudo = false
							}
						case usrOptSSHKey:
							if user.Existing {
								user.SSHKeys = append([]string(nil), user.OriginalSSHKeys...)
							} else {
								user.SSHKeys = nil
							}
						}
					}
				} else {
					absIdx := m.tabSubPage*maxOptionsPerPage + m.categoryPageCursor
					page := m.categoryPages[m.activeTab]
					if absIdx < len(page.Options) {
						resetOption(&m.categoryPages[m.activeTab].Options[absIdx])
					}
				}
			}

		case "R":
			if m.page == pageReview && !m.editingOption {
				if m.activeTab == tabIndexUSR {
					nUsers := len(m.categoryPages[tabIndexUSR].UserEntries)
					if m.usrSubTab < nUsers {
						u := &m.categoryPages[tabIndexUSR].UserEntries[m.usrSubTab]
						if u.Existing {
							u.Name = u.OriginalName
							u.OldPassword = ""
							u.NewPassword = ""
							u.Sudo = u.OriginalSudo
							u.SSHKeys = append([]string(nil), u.OriginalSSHKeys...)
						} else {
							u.Name = ""
							u.Password = ""
							u.Sudo = false
							u.SSHKeys = nil
						}
					}
				} else {
					for i := range m.categoryPages[m.activeTab].Options {
						resetOption(&m.categoryPages[m.activeTab].Options[i])
					}
				}
			}

		case "q", "esc":
			switch m.page {
			case pageWelcome:
				return m, tea.Quit
			case pageEnvironment:
				m.page = pageWelcome
			case pageQuickSetup:
				m.page = pageEnvironment
			case pageReview:
				if m.netPortListOpen {
					m.netPortListOpen = false
					m.netPortEditing = false
					m.inputError = ""
					return m, nil
				}
				m.page = pageQuickSetup
				m.activeTab = 0
				m.tabSubPage = 0
				m.categoryPageCursor = 0
			}

		case "left", "h":
			if m.page == pageReview {
				if m.activeTab == tabIndexUSR && m.osInfo.IsRoot && m.usrSubTab > 0 {
					m.usrSubTab--
					m.categoryPageCursor = 0
					labels := usrTabLabels(m.categoryPages[tabIndexUSR].UserEntries)
					m.usrTabOffset = clampUsrTabOffset(labels, m.usrSubTab, m.usrTabOffset, m.leftColW+m.rightContentW+2)
				} else if m.activeTab > 0 {
					m.activeTab--
					m.tabSubPage = 0
					m.categoryPageCursor = 0
					m.editingOption = false
					m.selectingOption = false
					m.usrEditingSSHList = false
					m.netPortListOpen = false
					m.netPortEditing = false
					m.inputError = ""
					m.categoryPages = syncOnTabEnter(m.activeTab, m.categoryPages)
				}
			}

		case "right", "l":
			if m.page == pageReview {
				nUserTabs := len(m.categoryPages[tabIndexUSR].UserEntries) + 1
				if m.activeTab == tabIndexUSR && m.osInfo.IsRoot && m.usrSubTab < nUserTabs-1 {
					m.usrSubTab++
					m.categoryPageCursor = 0
					labels := usrTabLabels(m.categoryPages[tabIndexUSR].UserEntries)
					m.usrTabOffset = clampUsrTabOffset(labels, m.usrSubTab, m.usrTabOffset, m.leftColW+m.rightContentW+2)
				} else if m.activeTab < len(m.categoryPages)-1 {
					m.activeTab++
					m.tabSubPage = 0
					m.categoryPageCursor = 0
					m.editingOption = false
					m.selectingOption = false
					m.usrEditingSSHList = false
					m.netPortListOpen = false
					m.netPortEditing = false
					m.inputError = ""
					m.categoryPages = syncOnTabEnter(m.activeTab, m.categoryPages)
				}
			}

		case "up", "k":
			switch m.page {
			case pageWelcome:
				if m.menuCursor > 0 {
					m.menuCursor--
				}
			case pageEnvironment:
				if !m.detecting && m.useCaseCursor > 0 {
					m.useCaseCursor--
				}
			case pageQuickSetup:
				if m.applyOrReviewCursor > 0 {
					m.applyOrReviewCursor--
				}
			case pageReview:
				if m.activeTab == tabIndexUSR && m.osInfo.IsRoot {
					if m.categoryPageCursor > 0 {
						m.categoryPageCursor--
					}
				} else if m.categoryPageCursor > 0 {
					m.categoryPageCursor--
				} else if m.tabSubPage > 0 {
					m.tabSubPage--
					curPage := m.categoryPages[m.activeTab]
					prevStart := m.tabSubPage * maxOptionsPerPage
					prevEnd := min(prevStart+maxOptionsPerPage, len(curPage.Options))
					m.categoryPageCursor = prevEnd - prevStart - 1
				}
			}

		case "down", "j":
			switch m.page {
			case pageWelcome:
				if m.menuCursor < len(menuItems)-1 {
					m.menuCursor++
				}
			case pageEnvironment:
				if !m.detecting && m.useCaseCursor < len(useCaseOptions)-1 {
					m.useCaseCursor++
				}
			case pageQuickSetup:
				if m.applyOrReviewCursor < len(applyOrReviewOptions)-1 {
					m.applyOrReviewCursor++
				}
			case pageReview:
				if m.activeTab == tabIndexUSR && m.osInfo.IsRoot {
					nUsers := len(m.categoryPages[tabIndexUSR].UserEntries)
					maxCursor := usrOptCount - 1
					if m.usrSubTab == nUsers {
						maxCursor = 0 // New User tab has no options to navigate
					}
					if m.categoryPageCursor < maxCursor {
						m.categoryPageCursor++
					}
				} else {
					curPage := m.categoryPages[m.activeTab]
					nSub := subPageCount(len(curPage.Options))
					isLastSub := m.tabSubPage == nSub-1
					startIdx := m.tabSubPage * maxOptionsPerPage
					endIdx := min(startIdx+maxOptionsPerPage, len(curPage.Options))
					subPageLen := endIdx - startIdx
					maxCursor := subPageLen - 1
					if m.categoryPageCursor < maxCursor {
						m.categoryPageCursor++
					} else if !isLastSub {
						m.tabSubPage++
						m.categoryPageCursor = 0
					}
				}
			}

		case "enter", " ":
			switch m.page {
			case pageWelcome:
				switch m.menuCursor {
				case 0:
					m.page = pageEnvironment
				case 1:
					return m, tea.Quit
				}
			case pageEnvironment:
				if !m.detecting {
					// Build category pages here so changes survive a back/forward trip
					// between pageQuickSetup and pageReview.
					m.categoryPages = buildCategoryPages(useCaseOptions[m.useCaseCursor], m.osInfo)
					m.activeTab = 0
					m.tabSubPage = 0
					m.categoryPageCursor = 0
					m.applyOrReviewCursor = 0
					m.page = pageQuickSetup
				}
			case pageQuickSetup:
				switch m.applyOrReviewCursor {
				case 0:
					// Apply recommended — Stage 5: run processes (not yet built)
				case 1:
					m.activeTab = 0
					m.tabSubPage = 0
					m.categoryPageCursor = 0
					m.page = pageReview
				}
			case pageReview:
				if m.activeTab == tabIndexUSR && m.osInfo.IsRoot {
					nUsers := len(m.categoryPages[tabIndexUSR].UserEntries)
					if m.usrSubTab == nUsers {
						// New User tab — start add flow.
						ti := textinput.New()
						ti.Prompt = "Username:  "
						ti.Focus()
						m.textInput = ti
						m.inputSubStep = 0
						m.subValues = [3]string{}
						m.inputError = ""
						m.editingOption = true
					} else {
						// Existing user tab — act on current cursor row.
						user := &m.categoryPages[tabIndexUSR].UserEntries[m.usrSubTab]
						switch m.categoryPageCursor {
						case usrOptUsername:
							if user.ActiveSession {
								m.ringBell = true
								m.visualBell = true
								return m, bellCmd()
							}
							ti := textinput.New()
							ti.Prompt = "Username:  "
							ti.SetValue(user.Name)
							ti.Focus()
							m.textInput = ti
							m.usrEditingField = usrOptUsername
							m.inputError = ""
							m.editingOption = true
						case usrOptPassword:
							m.usrEditingField = usrOptPassword
							m.subValues = [3]string{}
							m.inputError = ""
							if user.Existing && runtime.GOOS == "darwin" {
								m.textInput = newPasswordInput("Current pwd: ")
								m.inputSubStep = 0 // 0=old, 1=new, 2=confirm
							} else {
								m.textInput = newPasswordInput("New password: ")
								m.inputSubStep = 1 // 1=new, 2=confirm (Linux root skips old)
							}
							m.editingOption = true
						case usrOptSudo:
							user.Sudo = !user.Sudo
						case usrOptSSHKey:
							m.usrEditingSSHList = true
							m.usrSSHListCursor = len(sshKeyDisplayItems(*user)) // default to "+ Add new key"
							m.inputError = ""
						case usrOptDelete:
							u := &m.categoryPages[tabIndexUSR].UserEntries[m.usrSubTab]
							if u.Existing {
								u.PendingDelete = !u.PendingDelete
							} else {
								m.categoryPages[tabIndexUSR].UserEntries = append(
									m.categoryPages[tabIndexUSR].UserEntries[:m.usrSubTab],
									m.categoryPages[tabIndexUSR].UserEntries[m.usrSubTab+1:]...,
								)
								if m.usrSubTab >= len(m.categoryPages[tabIndexUSR].UserEntries) && m.usrSubTab > 0 {
									m.usrSubTab--
								}
								m.categoryPageCursor = 0
							}
						}
					}
				} else {
					curPage := m.categoryPages[m.activeTab]
					startIdx := m.tabSubPage * maxOptionsPerPage
					endIdx := min(startIdx+maxOptionsPerPage, len(curPage.Options))
					subPageLen := endIdx - startIdx
					if m.categoryPageCursor < subPageLen {
						absIdx := startIdx + m.categoryPageCursor
						opt := &m.categoryPages[m.activeTab].Options[absIdx]
						switch opt.Kind {
						case KindTextInput:
							ti := textinput.New()
							placeholder := opt.Default
							if opt.ClearOnEdit && opt.Value != "" {
								placeholder = opt.Value
							}
							ti.Placeholder = placeholder
							ti.Width = 40
							if opt.ClearOnEdit {
								ti.SetValue("")
							} else {
								editVal := opt.Value
								if editVal == "" {
									editVal = opt.Default
								}
								ti.SetValue(editVal)
							}
							ti.Focus()
							m.textInput = ti
							m.editingOption = true
						case KindSelect:
							m.selectItems = opt.SelectItems
							current := opt.Value
							if current == "" {
								current = opt.Default
							}
							m.selectCursor = 0
							for i, item := range m.selectItems {
								if item == current {
									m.selectCursor = i
									break
								}
							}
							m.selectViewport = max(0, m.selectCursor-visibleItems/2)
							m.selectingOption = true
						case KindPortList:
							// Enter/space: open the port list at cursor 0 (preset row).
							m.netPortListOpen = true
							m.netPortListCursor = 0
							m.inputError = ""
						case KindCycle:
							items := opt.SelectItems
							if len(items) > 0 {
								next := 0
								for i, v := range items {
									if v == opt.Value {
										next = (i + 1) % len(items)
										break
									}
								}
								opt.Value = items[next]
							}
						default: // KindToggle
							opt.Checked = !opt.Checked
						}
					} else if m.activeTab == tabIndexGO {
						switch m.applyState {
						case applyIdle:
							m.applyState = applyRunning
							return m, tea.Batch(doApplyAll(m.categoryPages), m.spinner.Tick)
						case applyDone:
							return m, tea.Quit
						}
					}
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
	// blank line below separator (1) + hints (3) + border (2) = 22,
	// rounded up to 24 for the wider tab bar.
	if m.rightContentH > 0 {
		const tallestBoxH = 24
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
	if m.page == pageSplash {
		return m.viewSplashPage()
	}
	leftMain := m.viewLeft()

	leftW := m.leftColW
	if leftW == 0 {
		leftW = lipgloss.Width(leftMain)
	}

	rightContent := m.viewRight()

	const topPad = 1
	paddedLeft := strings.Repeat("\n", topPad) + leftMain
	if m.page != pageReview {
		paddedLeft += "\n"
	}

	// leftCol outer = leftW + PaddingLeft(2) + PaddingRight(1) = leftW + 3
	// rightCol outer = rightContentW + PaddingLeft(1) + PaddingRight(1) = rightContentW + 2
	// total inner (inside box border) = leftW + rightContentW + 5
	// bottom content width = total inner - PaddingLeft(2) - PaddingRight(1) = leftW + rightContentW + 2
	bottomContentW := leftW + m.rightContentW + 2

	leftH := lipgloss.Height(paddedLeft)
	rightH := m.rightContentH // 0 until detection completes

	var box string
	switch {
	case m.page == pageReview && m.rightContentW > 0:
		// Category-review layout: title + tab bar sit beside the info table.
		// A purple │ runs down the right edge of the left column from the top
		// box border to the connector line, terminated by ╮ on both ends.
		tabBar := m.viewTabBar()
		tabBarLines := strings.Split(tabBar, "\n")
		tabBarBottomLine := tabBarLines[len(tabBarLines)-1]
		tabBarBottomW := lipgloss.Width(tabBarBottomLine)
		// Indent the visible tab lines one extra space to the right.
		topLines := tabBarLines[:len(tabBarLines)-1]
		for i, l := range topLines {
			topLines[i] = " " + l
		}
		tabBarTopPart := strings.Join(topLines, "\n")

		// Regular block: PaddingLeft(2) + Width(leftW), no PaddingRight.
		// A purple │ is appended to every line instead, forming the vertical
		// separator. Total line width = leftW+3 (same as with PaddingRight(1)).
		aboveSepTop := strings.Repeat("\n", topPad) +
			m.viewTitle() + "\n" +
			subtitleStyle.Render("Review settings") + "\n\n" +
			tabBarTopPart
		regularBlock := lipgloss.NewStyle().PaddingLeft(2).Render(
			lipgloss.NewStyle().Width(leftW).Render(aboveSepTop))
		purpleBar := lipgloss.NewStyle().Foreground(Themes[m.themeIdx].Accent).Render("│")
		regularLines := strings.Split(regularBlock, "\n")
		for i, l := range regularLines {
			regularLines[i] = l + purpleBar
		}
		regularBlock = strings.Join(regularLines, "\n")

		// Connector line: "───" + tab bar bottom chars + "─" fill + "╯".
		// The tabs are indented 1 extra space (see above), so we use 3 leading
		// dashes instead of 2 and subtract 1 from remaining to keep the total
		// width at leftW+3 (matching every regularBlock line).
		// The ╯ closes the vertical separator against the connector.
		remaining := leftW - tabBarBottomW - 1
		bareBottom := ansiEscape.ReplaceAllString(tabBarBottomLine, "")
		bare := "───" + bareBottom
		if remaining > 0 {
			bare += strings.Repeat("─", remaining)
		}
		bare += "╯"
		connectorLine := lipgloss.NewStyle().Foreground(Themes[m.themeIdx].Accent).Render(bare)

		leftAboveCol := regularBlock + "\n" + connectorLine
		rightAboveCol := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1).Render(rightContent)
		aboveRow := lipgloss.JoinHorizontal(lipgloss.Top, leftAboveCol, rightAboveCol)

		body := m.viewCategoryReviewBody(bottomContentW)
		hints := m.viewHints(bottomContentW)
		innerW := bottomContentW + 3
		sepLine := lipgloss.NewStyle().Foreground(Themes[m.themeIdx].Accent).Render(strings.Repeat("─", innerW))
		bodyPrefix := "\n"
		if m.activeTab == tabIndexUSR {
			bodyPrefix = ""
		}
		bodyBlock := lipgloss.NewStyle().
			Width(bottomContentW).
			PaddingLeft(2).PaddingRight(1).
			Render(bodyPrefix + body)
		hintsBlock := lipgloss.NewStyle().
			Width(bottomContentW).
			PaddingLeft(2).PaddingRight(1).
			Render(strings.TrimLeft(hints, "\n"))

		box = boxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, aboveRow, bodyBlock, sepLine, hintsBlock))

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
				boxLines[0] = lipgloss.NewStyle().Foreground(Themes[m.themeIdx].Accent).Render(string(runes))
			}
			// Replace │ with ├ on the left border at the connector-line row.
			// Use raw string replacement to preserve existing ANSI colors in the line.
			connectorRowIdx := lipgloss.Height(leftAboveCol)
			if connectorRowIdx < len(boxLines) {
				raw := boxLines[connectorRowIdx]
				if idx := strings.Index(raw, "│"); idx >= 0 {
					boxLines[connectorRowIdx] = raw[:idx] + "├" + raw[idx+len("│"):]
				}
			}
			boxLines = injectHintsSep(boxLines)
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
		innerW := bottomContentW + 3
		sepLine := lipgloss.NewStyle().Foreground(Themes[m.themeIdx].Accent).Render(strings.Repeat("─", innerW))
		bottomBlock := lipgloss.NewStyle().
			Width(bottomContentW).
			PaddingLeft(2).PaddingRight(1).
			Render(bottomLeft)
		hintsBlock := lipgloss.NewStyle().
			Width(bottomContentW).
			PaddingLeft(2).PaddingRight(1).
			Render(strings.TrimLeft(hints, "\n"))

		box = boxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, topRow, bottomBlock, sepLine, hintsBlock))
		boxLines := strings.Split(box, "\n")
		boxLines = injectHintsSep(boxLines)
		box = strings.Join(boxLines, "\n")

	case m.rightContentW > 0:
		// Left content fits beside the table (leftH <= rightH).
		// Always render hints full-width below the table with a separator.
		topLeftBlock := lipgloss.NewStyle().Width(leftW).Render(paddedLeft)
		topLeftCol := lipgloss.NewStyle().PaddingLeft(2).PaddingRight(1).Render(topLeftBlock)
		topRightCol := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1).Render(rightContent)
		topRow := lipgloss.JoinHorizontal(lipgloss.Top, topLeftCol, topRightCol)

		hints := m.viewHints(bottomContentW)
		innerW := bottomContentW + 3
		sepLine := lipgloss.NewStyle().Foreground(Themes[m.themeIdx].Accent).Render(strings.Repeat("─", innerW))
		hintsBlock := lipgloss.NewStyle().
			Width(bottomContentW).
			PaddingLeft(2).PaddingRight(1).
			Render(strings.TrimLeft(hints, "\n"))

		box = boxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, topRow, sepLine, hintsBlock))
		boxLines := strings.Split(box, "\n")
		boxLines = injectHintsSep(boxLines)
		box = strings.Join(boxLines, "\n")

	default:
		// Detecting or no right content yet: single-column layout.
		leftMainBlock := lipgloss.NewStyle().Width(leftW).Render(paddedLeft)
		leftCol := lipgloss.NewStyle().PaddingLeft(2).PaddingRight(1).Render(leftMainBlock)
		rightCol := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1).Render(rightContent)
		topRow := lipgloss.JoinHorizontal(lipgloss.Top, leftCol, rightCol)

		innerW := lipgloss.Width(topRow)
		sepLine := lipgloss.NewStyle().Foreground(Themes[m.themeIdx].Accent).Render(strings.Repeat("─", innerW))
		hintsW := innerW - 3
		hints := m.viewHints(hintsW)
		hintsBlock := lipgloss.NewStyle().
			Width(hintsW).
			PaddingLeft(2).PaddingRight(1).
			Render(strings.TrimLeft(hints, "\n"))

		box = boxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, topRow, sepLine, hintsBlock))
		boxLines := strings.Split(box, "\n")
		boxLines = injectHintsSep(boxLines)
		box = strings.Join(boxLines, "\n")
	}

	bell := ""
	if m.ringBell {
		bell = "\a"
	}
	if m.width > 0 && m.height > 0 {
		return bell + lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Top,
			strings.Repeat("\n", m.stableTop)+box)
	}
	return bell + box
}

// viewTitle returns the "boltx" title with a privilege indicator suffix.
// Not root: amber "· no sudo". Root: green "· ● sudo".
func (m Model) viewTitle() string {
	title := titleStyle.Render("boltx")
	if !m.osInfo.IsRoot {
		warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B"))
		return title + warnStyle.Render(" ● running without sudo")
	}
	return title + greenStyle.Render(" ● running with sudo")
}

// viewLeft returns the main interactive content for the left column (no hints).
func (m Model) viewLeft() string {
	switch m.page {
	case pageEnvironment:
		return m.viewUseCase()
	case pageQuickSetup:
		return m.viewApplyOrReview()
	case pageReview:
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
		if m.page == pageReview {
			resetBinding := keyResetOption
			if m.activeTab == tabIndexUSR && m.usrEditingSSHList {
				entries := m.categoryPages[tabIndexUSR].UserEntries
				if m.usrSubTab < len(entries) {
					if m.usrSSHListCursor < len(sshKeyDisplayItems(entries[m.usrSubTab])) {
						resetBinding = keyRemoveSSHKey
					}
				}
			}
			groups := [][]key.Binding{
				{keyNav, keyTabNav},
				{keySelect, keyBack, resetBinding, keyResetTab},
				{keyTheme, keyHelpLess},
			}
			return "\n\n" + h.FullHelpView(groups)
		}
		// Two rows: nav/confirm stacked in the first column, rest alongside.
		// This ensures all bindings are always visible regardless of box width.
		h.Width = m.leftColW + m.rightContentW + 2
		var groups [][]key.Binding
		switch m.page {
		case pageEnvironment, pageQuickSetup:
			groups = [][]key.Binding{
				{keyNav, keyConfirm},
				{keyBack, keyTheme},
				{keyHelpLess},
			}
		default: // pageWelcome
			groups = [][]key.Binding{
				{keyNav, keyConfirm},
				{keyQuit, keyTheme},
				{keyHelpLess},
			}
		}
		return "\n\n" + h.FullHelpView(groups)
	}

	var bindings []key.Binding
	switch m.page {
	case pageEnvironment, pageQuickSetup:
		bindings = []key.Binding{keyBack, keyHelpMore}
	case pageReview:
		bindings = []key.Binding{keyBack, keyHelpMore}
	default:
		bindings = []key.Binding{keyQuit, keyHelpMore}
	}
	themeName := mutedStyle.Render("  [" + Themes[m.themeIdx].Name + "]")
	return "\n\n" + h.ShortHelpView(bindings) + themeName
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

	osName := osInfo.PrettyName
	if osName == "" {
		osName = "—"
	}
	data = append(data, kv{"OS", osName})
	data = append(data, kv{"Virt", env.Virt.String()})
	pkgName := osInfo.Pkg.String()
	if osInfo.Pkg == detect.PkgUnknown {
		pkgName = "—"
	}
	data = append(data, kv{"Pkg", pkgName})
	sshVal := "no"
	if env.ViaSSH {
		sshVal = "connected"
	}
	data = append(data, kv{"SSH", sshVal})
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
		Header:   mutedStyle.Padding(0, 1),
		Cell:     normalStyle.Padding(0, 1),
		Selected: normalStyle,
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

	b.WriteString(m.viewTitle() + "\n")
	b.WriteString(subtitleStyle.Render("Easy first setup... and FAST!") + "\n\n\n")

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

	b.WriteString(m.viewTitle() + "\n")
	b.WriteString(subtitleStyle.Render("Environment") + "\n\n")

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

	b.WriteString(m.viewTitle() + "\n")
	b.WriteString(subtitleStyle.Render("Quick setup") + "\n\n")

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
	b.WriteString(m.viewTitle() + "\n")
	b.WriteString(subtitleStyle.Render("Review settings") + "\n\n")
	b.WriteString(m.viewTabBar())
	return b.String()
}

// viewUSRBody renders the USR tab body: secondary user sub-tab row + per-user options.
func (m Model) viewUSRBody(maxWidth int) string {
	var b strings.Builder
	users := m.categoryPages[tabIndexUSR].UserEntries

	if !m.osInfo.IsRoot {
		b.WriteString(mutedStyle.Render("User management requires root.") + "\n")
		b.WriteString(normalStyle.Render("  sudo ./boltx") + "\n")
		return b.String()
	}

	// Secondary user sub-tab row — sliding window, always one line.
	usrTabActiveStyle := lipgloss.NewStyle().Foreground(Themes[m.themeIdx].Accent).Bold(true)
	newUserIdx := len(users)
	allLabels := usrTabLabels(users)

	b.WriteString(usrTabSlidingWindow(allLabels, m.usrSubTab, m.usrTabOffset, maxWidth, usrTabActiveStyle) + "\n\n")

	// New User tab.
	if m.usrSubTab == newUserIdx {
		if m.editingOption {
			b.WriteString(m.textInput.View() + "\n")
			if m.inputError != "" {
				b.WriteString(errorStyle.Render("✗ "+m.inputError) + "\n")
			}
		} else {
			b.WriteString(mutedStyle.Render("Press enter to add a new user.") + "\n")
		}
		return b.String()
	}

	// Existing user options.
	user := users[m.usrSubTab]
	type usrRow struct {
		label string
		value string
		idx   int
	}
	sshAdded, sshRemoved := sshKeyDelta(user)
	var sshVal string
	if len(user.SSHKeys) == 0 {
		sshVal = "(0)"
	} else {
		sshVal = fmt.Sprintf("(%d)", len(user.SSHKeys))
	}
	pwVal := "••••••"
	if user.Existing {
		pwVal = "(unchanged)"
		if user.NewPassword != "" {
			pwVal = "(set)"
		}
	}
	usernameVal := user.Name
	if user.ActiveSession {
		usernameVal = user.Name + " (active — log out to rename)"
	}

	// Queued flags per row.
	usernameQueued := !user.Existing || (!user.ActiveSession && user.OriginalName != "" && user.Name != user.OriginalName)
	passwordQueued := user.Password != "" || user.NewPassword != ""
	sudoQueued := (!user.Existing && user.Sudo) || (user.Existing && user.Sudo != user.OriginalSudo)
	sshQueued := sshAdded > 0 || sshRemoved > 0

	rows := []usrRow{
		{label: "Username", value: usernameVal, idx: usrOptUsername},
		{label: "Password", value: pwVal, idx: usrOptPassword},
		{label: "Sudo", idx: usrOptSudo},
		{label: "SSH keys", value: sshVal, idx: usrOptSSHKey},
	}
	for _, row := range rows {
		isCursor := m.categoryPageCursor == row.idx
		cur := noCursorStr
		style := normalStyle
		if isCursor {
			cur = cursorStr
			style = selectedStyle
		}
		if row.idx == usrOptUsername && m.visualBell {
			style = errorStyle
		}

		// Per-row queued state.
		var rowQueued bool
		switch row.idx {
		case usrOptUsername:
			rowQueued = usernameQueued
		case usrOptPassword:
			rowQueued = passwordQueued
		case usrOptSudo:
			rowQueued = sudoQueued
		case usrOptSSHKey:
			rowQueued = sshQueued
		}
		valStyle := mutedStyle
		if rowQueued {
			valStyle = queuedStyle
		}

		if row.idx == usrOptSudo {
			marker := radioOff
			if user.Sudo {
				marker = radioOn
			}
			line := renderOptionLine(cur, marker, "Sudo", style, maxWidth)
			if sudoQueued {
				line += queuedStyle.Render(" ·")
			}
			b.WriteString(line + "\n")
		} else {
			listMarker := kindTextInputMarker
			if row.idx == usrOptSSHKey {
				if m.usrEditingSSHList {
					listMarker = kindListExpanded
				} else {
					listMarker = kindListCollapsed
				}
			}
			b.WriteString(renderOptionLine(cur, listMarker, row.label+": ", style, maxWidth))
			if row.idx == usrOptSSHKey {
				b.WriteString(mutedStyle.Render(sshVal))
				if sshAdded > 0 || sshRemoved > 0 {
					b.WriteString(" ")
					if sshAdded > 0 {
						b.WriteString(queuedStyle.Render(fmt.Sprintf("+%d", sshAdded)))
					}
					if sshRemoved > 0 {
						if sshAdded > 0 {
							b.WriteString(" ")
						}
						b.WriteString(queuedStyle.Render(fmt.Sprintf("-%d", sshRemoved)))
					}
				}
				b.WriteString("\n")
			} else {
				b.WriteString(valStyle.Render(row.value) + "\n")
			}
			if isCursor && m.usrEditingSSHList {
				indent := strings.Repeat(" ", lipgloss.Width(cur+kindTextInputMarker))
				pendingStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Strikethrough(true)
				items := sshKeyDisplayItems(user)
				addIdx := len(items)
				for i, item := range items {
					liCur := "  "
					isCursorItem := m.usrSSHListCursor == i
					if item.pendingRemove {
						hint := mutedStyle.Render("  [r: undo]")
						label := pendingStyle.Render(apply.SSHKeyComment(item.key))
						if isCursorItem {
							liCur = cursorStr
						}
						b.WriteString(indent + liCur + label + hint + "\n")
					} else {
						liStyle := normalStyle
						if isCursorItem {
							liCur = cursorStr
							liStyle = selectedStyle
						}
						b.WriteString(indent + liCur + liStyle.Render(apply.SSHKeyComment(item.key)) + mutedStyle.Render("  [enter: edit  r: remove]") + "\n")
					}
				}
				addCur := "  "
				addStyle := mutedStyle
				if m.usrSSHListCursor == addIdx {
					addCur = cursorStr
					addStyle = selectedStyle
				}
				b.WriteString(indent + addCur + addStyle.Render("+ Add new key") + "\n")
				if m.editingOption {
					b.WriteString(indent + "  " + m.textInput.View() + "\n")
				}
				if m.inputError != "" {
					b.WriteString(indent + "  " + errorStyle.Render("✗ "+m.inputError) + "\n")
				}
			} else if isCursor && m.editingOption {
				indent := strings.Repeat(" ", lipgloss.Width(cur+kindTextInputMarker))
				b.WriteString(indent + m.textInput.View() + "\n")
				if m.inputError != "" {
					b.WriteString(indent + errorStyle.Render("✗ "+m.inputError) + "\n")
				}
			}
		}
	}

	// Delete user row — danger color, always last.
	isCursorDel := m.categoryPageCursor == usrOptDelete
	curDel := noCursorStr
	if isCursorDel {
		curDel = cursorStr
	}
	deleteLabel := "Delete user"
	if user.PendingDelete {
		deleteLabel = "Undo delete"
	}
	b.WriteString("\n" + curDel + errorStyle.Render(deleteLabel) + "\n")

	return b.String()
}

// sshKeyDelta returns the count of keys added and removed relative to OriginalSSHKeys.
func sshKeyDelta(u UserEntry) (added, removed int) {
	origSet := map[string]bool{}
	for _, k := range u.OriginalSSHKeys {
		origSet[k] = true
	}
	activeSet := map[string]bool{}
	for _, k := range u.SSHKeys {
		activeSet[k] = true
	}
	for _, k := range u.SSHKeys {
		if !origSet[k] {
			added++
		}
	}
	for _, k := range u.OriginalSSHKeys {
		if !activeSet[k] {
			removed++
		}
	}
	return
}

// sshKeyItem is one row in the SSH key list sub-UI.
type sshKeyItem struct {
	key           string
	existing      bool // loaded from authorized_keys (OriginalSSHKeys)
	pendingRemove bool // existing key the user flagged for removal (not in SSHKeys)
}

// sshKeyDisplayItems builds the ordered display list for the SSH key list sub-UI:
// original keys first (pending-remove if absent from SSHKeys), then newly added keys.
func sshKeyDisplayItems(u UserEntry) []sshKeyItem {
	origSet := map[string]bool{}
	for _, k := range u.OriginalSSHKeys {
		origSet[k] = true
	}
	activeSet := map[string]bool{}
	for _, k := range u.SSHKeys {
		activeSet[k] = true
	}
	var items []sshKeyItem
	for _, k := range u.OriginalSSHKeys {
		items = append(items, sshKeyItem{key: k, existing: true, pendingRemove: !activeSet[k]})
	}
	for _, k := range u.SSHKeys {
		if !origSet[k] {
			items = append(items, sshKeyItem{key: k, existing: false})
		}
	}
	return items
}

// usrTabLabels builds the display labels for all user sub-tabs including "+ New User".
func usrTabLabels(entries []UserEntry) []string {
	labels := make([]string, len(entries)+1)
	for i, u := range entries {
		label := u.Name
		if u.PendingDelete {
			label = "✗" + u.Name
		} else if u.Existing {
			label = "·" + u.Name
		}
		labels[i] = label
	}
	labels[len(entries)] = "+ New User"
	return labels
}

// usrTabVisibleEnd returns the exclusive end index of entries visible from offset within maxWidth.
func usrTabVisibleEnd(labels []string, offset, maxWidth int) int {
	const sepW = 3
	budget := maxWidth
	if offset > 0 {
		budget -= 1 + sepW // "‹" + sep
	}
	hi := offset
	for hi < len(labels) {
		w := len([]rune(labels[hi]))
		if hi > offset {
			w += sepW
		}
		// If more entries remain after this one, reserve space for "›".
		if hi+1 < len(labels) && budget-w < 1+sepW {
			break
		}
		budget -= w
		hi++
	}
	return hi
}

// clampUsrTabOffset returns the smallest offset ≥ current that keeps activeIdx visible.
func clampUsrTabOffset(labels []string, activeIdx, offset, maxWidth int) int {
	if activeIdx < offset {
		return activeIdx
	}
	for {
		if activeIdx < usrTabVisibleEnd(labels, offset, maxWidth) {
			return offset
		}
		offset++
		if offset > activeIdx {
			return activeIdx
		}
	}
}

// usrTabSlidingWindow renders the user sub-tab row as a single line starting at offset.
// Only scrolls when activeIdx goes off-screen (lazy scroll).
func usrTabSlidingWindow(labels []string, activeIdx, offset, maxWidth int, activeStyle lipgloss.Style) string {
	const sep = "   "
	hi := usrTabVisibleEnd(labels, offset, maxWidth)

	var parts []string
	if offset > 0 {
		parts = append(parts, mutedStyle.Render("‹"))
	}
	for i := offset; i < hi; i++ {
		if i == activeIdx {
			parts = append(parts, activeStyle.Render(labels[i]))
		} else {
			parts = append(parts, mutedStyle.Render(labels[i]))
		}
	}
	if hi < len(labels) {
		parts = append(parts, mutedStyle.Render("›"))
	}
	return strings.Join(parts, sep)
}

// viewPortSubMenu renders the port add/edit submenu rows into the given indent.
func (m Model) viewPortSubMenu(indent string) string {
	var b strings.Builder
	proto := m.subValues[2]
	if proto == "" {
		proto = "tcp"
	}
	isRange := m.netPortSubMenuType == "range"
	protoIdx := m.portSubMenuProtoIdx()
	confirmIdx := m.portSubMenuConfirmIdx()

	row := func(rowIdx int, label, val string) {
		cur := noCursorStr
		s := normalStyle
		if m.netPortSubMenuCursor == rowIdx {
			cur = cursorStr
			s = selectedStyle
		}
		b.WriteString(indent + cur + s.Render(label+val) + "\n")
	}

	// Row 0: Type cycle
	row(0, "Type: ", m.netPortSubMenuType)

	// Row 1: Port/From text field
	fromLabel := "Port:  "
	if isRange {
		fromLabel = "From:  "
	}
	if m.netPortSubMenuCursor == 1 && m.editingOption {
		b.WriteString(indent + cursorStr + selectedStyle.Render(fromLabel) + m.textInput.View() + "\n")
		if m.inputError != "" {
			b.WriteString(indent + noCursorStr + errorStyle.Render("✗ "+m.inputError) + "\n")
		}
	} else {
		v := m.subValues[0]
		if v == "" {
			v = mutedStyle.Render("1 - 65535")
		}
		cur := noCursorStr
		s := normalStyle
		if m.netPortSubMenuCursor == 1 {
			cur = cursorStr
			s = selectedStyle
		}
		b.WriteString(indent + cur + s.Render(fromLabel+v) + "\n")
	}

	// Row 2: To text field (range mode only)
	if isRange {
		if m.netPortSubMenuCursor == 2 && m.editingOption {
			b.WriteString(indent + cursorStr + selectedStyle.Render("To:    ") + m.textInput.View() + "\n")
			if m.inputError != "" {
				b.WriteString(indent + noCursorStr + errorStyle.Render("✗ "+m.inputError) + "\n")
			}
		} else {
			v := m.subValues[1]
			if v == "" {
				v = mutedStyle.Render("1 - 65535")
			}
			cur := noCursorStr
			s := normalStyle
			if m.netPortSubMenuCursor == 2 {
				cur = cursorStr
				s = selectedStyle
			}
			b.WriteString(indent + cur + s.Render("To:    "+v) + "\n")
		}
	}

	// Proto row
	row(protoIdx, "Proto: ", proto+"\n")

	// Confirm row
	row(confirmIdx, "Confirm", "")

	return b.String()
}

// viewCategoryReviewBody returns the options list and optional confirm button
// that sit below the full-width separator. maxWidth is the available text
// width for word-wrap; callers should pass bottomContentW when the body
// spans the full inner width, or leftColW for the narrow fallback.
func (m Model) viewCategoryReviewBody(maxWidth int) string {
	if m.activeTab == tabIndexGO {
		return m.viewGOBody(maxWidth)
	}
	if m.activeTab == tabIndexUSR {
		return m.viewUSRBody(maxWidth)
	}

	var b strings.Builder
	colW := maxWidth
	if colW == 0 {
		colW = m.leftColW
		if colW == 0 {
			colW = 40
		}
	}

	page := m.categoryPages[m.activeTab]

	startIdx := m.tabSubPage * maxOptionsPerPage
	endIdx := min(startIdx+maxOptionsPerPage, len(page.Options))
	subOpts := page.Options[startIdx:endIdx]

	for i, opt := range subOpts {
		cursor := noCursorStr
		itemStyle := normalStyle
		isCursor := m.categoryPageCursor == i
		if isCursor {
			cursor = cursorStr
			itemStyle = selectedStyle
		}

		dirty := isOptionQueued(opt)
		valStyle := mutedStyle
		if dirty {
			valStyle = queuedStyle
		}

		switch opt.Kind {
		case KindTextInput:
			editing := isCursor && m.editingOption
			var displayVal string
			if !editing {
				displayVal = opt.Value
				if displayVal == "" {
					displayVal = opt.Default
				}
			}

			b.WriteString(renderOptionLine(cursor, kindTextInputMarker, opt.Label+": ", itemStyle, colW))
			b.WriteString(valStyle.Render(displayVal) + "\n")
			// When this option is being edited, render the inline text input below it.
			if editing {
				indent := strings.Repeat(" ", lipgloss.Width(cursor+kindTextInputMarker))
				b.WriteString(indent + m.textInput.View() + "\n")
				if m.inputError != "" {
					b.WriteString(indent + errorStyle.Render("✗ "+m.inputError) + "\n")
				}
			}
		case KindSelect:
			displayVal := opt.Value
			if displayVal == "" {
				displayVal = opt.Default
			}
			b.WriteString(renderOptionLine(cursor, kindSelectMarker, opt.Label+": ", itemStyle, colW))
			b.WriteString(valStyle.Render(displayVal) + "\n")
			// When this option is being selected, render the inline picker below it.
			if isCursor && m.selectingOption {
				indent := strings.Repeat(" ", lipgloss.Width(cursor+kindSelectMarker))
				end := min(m.selectViewport+visibleItems, len(m.selectItems))
				for idx := m.selectViewport; idx < end; idx++ {
					item := m.selectItems[idx]
					if idx == m.selectCursor {
						b.WriteString(indent + selectedStyle.Render("› "+item) + "\n")
					} else {
						b.WriteString(indent + mutedStyle.Render("  "+item) + "\n")
					}
				}
			}
		case KindPortList:
			listOpen := isCursor && m.netPortListOpen
			marker := kindListCollapsed
			if listOpen {
				marker = kindListExpanded
			}
			// Header row: label + rule count.
			count := len(opt.PortRules)
			countSuffix := ""
			if count > 0 {
				countSuffix = fmt.Sprintf(" (%d)", count)
			}
			b.WriteString(renderOptionLine(cursor, marker, opt.Label, itemStyle, colW))
			b.WriteString(valStyle.Render(countSuffix))
			if dirty {
				b.WriteString(queuedStyle.Render(" ·"))
			}
			b.WriteString("\n")

			if listOpen {
				indent := strings.Repeat(" ", lipgloss.Width(cursor+marker))

				if m.netPortEditing {
					b.WriteString(m.viewPortSubMenu(indent))
				} else {
					// Rule rows (cursor 0..N-1).
					for ri, rule := range opt.PortRules {
						liCur := noCursorStr
						liStyle := normalStyle
						if m.netPortListCursor == ri {
							liCur = cursorStr
							liStyle = selectedStyle
						}
						label := rule.String()
						if rule.Existing {
							label += mutedStyle.Render(" (active)")
						}
						b.WriteString(indent + liCur + liStyle.Render(label) + "\n")
					}
					// "Add port" row (cursor N).
					addPortIdx := len(opt.PortRules)
					addPortCur := noCursorStr
					addPortStyle := mutedStyle
					if m.netPortListCursor == addPortIdx {
						addPortCur = cursorStr
						addPortStyle = selectedStyle
					}
					b.WriteString(indent + addPortCur + addPortStyle.Render("+ Add port") + "\n")
					// "Add presets" row (cursor N+1) — expands inline when open.
					addPresetsIdx := addPortIdx + 1
					addPresetsCur := noCursorStr
					addPresetsStyle := mutedStyle
					if m.netPortListCursor == addPresetsIdx {
						addPresetsCur = cursorStr
						addPresetsStyle = selectedStyle
					}
					b.WriteString(indent + addPresetsCur + addPresetsStyle.Render("+ Add presets") + "\n")
					if m.netPortPresetsOpen {
						presetIndent := indent + "  "
						for pi, p := range netPresets {
							pCur := noCursorStr
							pStyle := normalStyle
							if m.netPortPresetCursor == pi {
								pCur = cursorStr
								pStyle = selectedStyle
							}
							check := radioOff
							if m.netPortPresetSel[pi] {
								check = radioOn
							}
							b.WriteString(presetIndent + pCur + pStyle.Render(check+p.Label) + "\n")
						}
						confirmIdx := len(netPresets)
						confCur := noCursorStr
						confStyle := mutedStyle
						if m.netPortPresetCursor == confirmIdx {
							confCur = cursorStr
							confStyle = selectedStyle
						}
						b.WriteString(presetIndent + confCur + confStyle.Render("[Confirm]") + "\n")
					}
				}
			}

		case KindCycle:
			b.WriteString(renderOptionLine(cursor, kindCycleMarker, opt.Label+": ", itemStyle, colW))
			b.WriteString(valStyle.Render(opt.Value) + "\n")
		default: // KindToggle
			radio := radioOff
			if opt.Checked {
				radio = radioOn
			}
			line := renderOptionLine(cursor, radio, opt.Label, itemStyle, colW)
			if dirty {
				line += queuedStyle.Render(" ·")
			}
			b.WriteString(line + "\n")
		}
	}

	if len(page.Options) == 0 {
		b.WriteString(mutedStyle.Render("Nothing to configure here yet."))
	}

	return b.String()
}

// viewGOBody renders the GO! tab body across its three lifecycle states.
func (m Model) viewGOBody(maxWidth int) string {
	var b strings.Builder

	switch m.applyState {
	case applyRunning:
		b.WriteString(m.spinner.View() + " Applying settings…\n")

	case applyDone:
		if len(m.applyResults) == 0 {
			b.WriteString(mutedStyle.Render("Nothing was applied.") + "\n")
		} else {
			for _, r := range m.applyResults {
				if r.err == nil {
					b.WriteString(greenStyle.Render("✓ "+r.label) + "\n")
				} else {
					b.WriteString(errorStyle.Render("✗ "+r.label+": "+r.err.Error()) + "\n")
				}
			}
		}
		b.WriteString("\n" + mutedStyle.Render("Press Enter or q to exit.") + "\n")

	default: // applyIdle
		b.WriteString(m.viewGOSummaryTable(maxWidth))
	}

	return b.String()
}

// viewGOSummaryTable renders a row of individual rounded mini-tables, one per
// category, with 1-space gaps between them so they feel like floating cards.
// Borders are accent-colored when the category has pending changes, muted otherwise.
//
//	╭─────╮ ╭─────╮ ╭─────╮ ╭─────╮ ╭─────╮ ╭─────╮
//	│ SYS │ │ USR │ │ SEC │ │ NET │ │ PKG │ │ RUN │
//	│  0  │ │  2  │ │  0  │ │  0  │ │  3  │ │  1  │
//	╰─────╯ ╰─────╯ ╰─────╯ ╰─────╯ ╰─────╯ ╰─────╯
//	Total: 6             ↵  Apply
func (m Model) viewGOSummaryTable(maxWidth int) string {
	var pages []CategoryPage
	for _, p := range m.categoryPages {
		if p.Name != "GO!" {
			pages = append(pages, p)
		}
	}
	if len(pages) == 0 {
		return mutedStyle.Render("No categories.") + "\n"
	}

	const colW = 5 // inner cell width (must fit category name + padding)

	// center pads s to exactly colW chars (ASCII-only content assumed).
	center := func(s string) string {
		pad := colW - len(s)
		if pad <= 0 {
			return s[:colW]
		}
		l := pad / 2
		return strings.Repeat(" ", l) + s + strings.Repeat(" ", pad-l)
	}

	const boxLines = 4 // top + name + count + bottom
	rows := make([][]string, len(pages))
	total := 0

	for i, p := range pages {
		count := checkedCount(p)
		total += count

		var bst lipgloss.Style
		if count > 0 {
			bst = lipgloss.NewStyle().Foreground(Themes[m.themeIdx].Accent)
		} else {
			bst = mutedStyle
		}
		pipe := bst.Render("│")

		s := fmt.Sprintf("%d", count)
		var countCell string
		if count > 0 {
			countCell = greenStyle.Render(center(s))
		} else {
			countCell = mutedStyle.Render(center(s))
		}

		rows[i] = []string{
			bst.Render("╭─────╮"),
			pipe + normalStyle.Render(center(p.Name)) + pipe,
			pipe + countCell + pipe,
			bst.Render("╰─────╯"),
		}
	}

	// Join all boxes horizontally line by line with 1-space gap.
	lines := make([]string, boxLines)
	for i, box := range rows {
		for l := 0; l < boxLines; l++ {
			if i > 0 {
				lines[l] += " "
			}
			lines[l] += box[l]
		}
	}

	// Center the card row within maxWidth.
	tableW := lipgloss.Width(lines[0])
	tablePad := max(0, (maxWidth-tableW)/2)
	tablePrefix := strings.Repeat(" ", tablePad)

	// Apply button: "↵  Apply N changes" centered within maxWidth.
	var applyLabel string
	if total == 1 {
		applyLabel = fmt.Sprintf("↵  Apply %d change", total)
	} else {
		applyLabel = fmt.Sprintf("↵  Apply %d changes", total)
	}
	applyBtn := selectedStyle.Render(applyLabel)
	btnPad := max(0, (maxWidth-lipgloss.Width(applyBtn))/2)
	btnLine := strings.Repeat(" ", btnPad) + applyBtn

	var b strings.Builder
	for _, l := range lines {
		b.WriteString(tablePrefix + l + "\n")
	}
	b.WriteString("\n")
	b.WriteString(btnLine)
	return b.String()
}

// viewCategoryReview is the fallback used before detection completes.
func (m Model) viewCategoryReview() string {
	return m.viewCategoryReviewAboveSep() + "\n\n" + m.viewCategoryReviewBody(m.leftColW)
}

// viewTabBar renders the horizontal tab strip for the category review stage.
// The active tab is 3 lines tall (top border, label, open bottom). Ghost tabs
// are 2 lines tall (top border, bottom connector). Bottom-aligning them makes
// the active tab rise above the ghost tabs. All open bottoms connect to the
// separator below.
// userHasChanges reports whether a UserEntry will produce any work at GO! time.
func userHasChanges(u UserEntry) bool {
	if !u.Existing {
		return true // new user to create
	}
	if u.PendingDelete {
		return true
	}
	if u.OriginalName != "" && u.Name != u.OriginalName {
		return true
	}
	if u.NewPassword != "" {
		return true
	}
	if u.Sudo != u.OriginalSudo {
		return true
	}
	// SSH key delta: any add or remove?
	origSet := map[string]bool{}
	for _, k := range u.OriginalSSHKeys {
		origSet[k] = true
	}
	for _, k := range u.SSHKeys {
		if !origSet[k] {
			return true // key to add
		}
	}
	activeSet := map[string]bool{}
	for _, k := range u.SSHKeys {
		activeSet[k] = true
	}
	for _, k := range u.OriginalSSHKeys {
		if !activeSet[k] {
			return true // key to remove
		}
	}
	return false
}

// isOptionQueued reports whether an option differs from its session-start state.
func isOptionQueued(opt CategoryOption) bool {
	switch opt.Kind {
	case KindCycle:
		return opt.Value != opt.Default
	case KindTextInput, KindSelect:
		return opt.Value != "" && opt.Value != opt.Default
	case KindPortList:
		// Queued if any new (non-existing) rules were added.
		for _, r := range opt.PortRules {
			if !r.Existing {
				return true
			}
		}
		// Or if any detected rule was removed.
		current := portRuleKeySet(opt.PortRules)
		for _, r := range opt.DetectedPortRules {
			if !current[portRuleKey{r.From, r.To, r.Protocol}] {
				return true
			}
		}
		return false
	default: // KindToggle
		if opt.UndoFn != nil {
			return opt.Checked != opt.OriginalChecked
		}
		return opt.Checked
	}
}

// checkedCount returns the number of options that will produce work at GO! time:
// checked options with ApplyFn, or unchecked options with UndoFn.
func checkedCount(page CategoryPage) int {
	n := 0
	for _, u := range page.UserEntries {
		if userHasChanges(u) {
			n++
		}
	}
	for _, opt := range page.Options {
		switch opt.Kind {
		case KindPortList:
			if len(opt.PortRules) > 0 {
				n++
			}
		case KindCycle:
			if opt.ApplyFn != nil && opt.Value != opt.Default {
				n++
			}
		default:
			if opt.UndoFn != nil {
				if opt.Checked != opt.OriginalChecked {
					n++
				}
			} else if opt.Checked && opt.ApplyFn != nil {
				n++
			}
		}
	}
	return n
}

func (m Model) viewTabBar() string {
	parts := make([]string, len(m.categoryPages))
	for i, page := range m.categoryPages {
		if i == m.activeTab {
			label := page.Name
			n := subPageCount(len(page.Options))
			if n > 1 {
				label = fmt.Sprintf("%s %d/%d", page.Name, m.tabSubPage+1, n)
			}
			parts[i] = activeTabStyle.Render(label)
		} else {
			top := mutedStyle.Render("╭──╮")
			bot := mutedStyle.Render("┴──┴")
			parts[i] = top + "\n" + bot
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Bottom, parts...)
}

// syncOnTabEnter is called every time the active tab changes.
// It dispatches to tab-specific sync functions as they are implemented.
func syncOnTabEnter(tabIdx int, pages []CategoryPage) []CategoryPage {
	switch tabIdx {
	case tabIndexUSR:
		return syncUsrTab(pages)
	case tabIndexSEC:
		return syncSecTab(pages)
	case tabIndexNET:
		return syncNetTab(pages)
	case tabIndexPKG:
		return syncPkgTab(pages)
	case tabIndexRUN:
		return syncRunTab(pages)
	}
	return pages
}

// syncUsrTab merges system human users (UID ≥ 1000) into UserEntries.
// Existing entries (pending new users or already-merged system users) are kept as-is.
func syncUsrTab(pages []CategoryPage) []CategoryPage {
	existing := map[string]bool{}
	for _, u := range pages[tabIndexUSR].UserEntries {
		existing[u.Name] = true
	}
	for _, hu := range apply.LoadHumanUsers() {
		if existing[hu.Name] {
			continue
		}
		loadedKeys := apply.LoadSSHKeys(hu.Name)
		pages[tabIndexUSR].UserEntries = append(
			pages[tabIndexUSR].UserEntries,
			UserEntry{
				Name: hu.Name, OriginalName: hu.Name,
				Sudo: hu.Sudo, OriginalSudo: hu.Sudo,
				Existing: true, ActiveSession: apply.HasActiveProcesses(hu.Name),
				SSHKeys:         append([]string(nil), loadedKeys...),
				OriginalSSHKeys: loadedKeys,
			},
		)
	}
	return pages
}

// Tab index constants — must match the order in buildCategoryPages.
const (
	tabIndexSYS = 0
	tabIndexUSR = 1
	tabIndexSEC = 2
	tabIndexNET = 3
	tabIndexPKG = 4
	tabIndexRUN = 5
	tabIndexGO  = 6
)

// syncSecTab detects current SSH and firewall state and pre-checks options accordingly.
// Runs only on first tab entry; subsequent entries preserve user-toggled state.
func syncSecTab(pages []CategoryPage) []CategoryPage {
	if pages[tabIndexSEC].Synced {
		return pages
	}
	opts := pages[tabIndexSEC].Options
	permitRootLogin, _ := apply.DetectSSHDConfig("PermitRootLogin")
	pwAuth, _ := apply.DetectSSHDConfig("PasswordAuthentication")
	sshPort, _ := apply.DetectSSHDConfig("Port")
	fwActive := apply.FirewallActive()
	for i := range opts {
		switch opts[i].Label {
		case "SSH: Permit root login":
			if permitRootLogin != "" {
				opts[i].Value = permitRootLogin
				opts[i].Default = permitRootLogin
			}
		case "SSH: Require key auth only":
			opts[i].Checked = pwAuth == "no"
			opts[i].OriginalChecked = opts[i].Checked
		case "SSH: Port":
			if sshPort != "" {
				opts[i].Default = sshPort
			}
		case "Enable UFW firewall":
			opts[i].Checked = fwActive
			opts[i].OriginalChecked = opts[i].Checked
		}
	}
	pages[tabIndexSEC].Options = opts
	pages[tabIndexSEC].Synced = true
	return pages
}

// resetOption reverts a single CategoryOption to its detected/default state.
// KindCycle: restores Value to Default.
// KindTextInput: clears Value and unchecks.
// KindToggle: restores Checked to OriginalChecked (detected state).
// KindPortList: restores PortRules to DetectedPortRules.
func resetOption(opt *CategoryOption) {
	switch opt.Kind {
	case KindCycle:
		opt.Value = opt.Default
	case KindTextInput:
		opt.Value = ""
		opt.Checked = false
	case KindToggle:
		opt.Checked = opt.OriginalChecked
	case KindPortList:
		// Reset to the state detected from the live system.
		opt.PortRules = make([]apply.PortRule, len(opt.DetectedPortRules))
		copy(opt.PortRules, opt.DetectedPortRules)
	}
}

// portSubMenuProtoIdx returns the submenu row index of the protocol cycle row.
func (m Model) portSubMenuProtoIdx() int {
	if m.netPortSubMenuType == "range" {
		return 3
	}
	return 2
}

// portSubMenuConfirmIdx returns the submenu row index of the Confirm row.
func (m Model) portSubMenuConfirmIdx() int {
	if m.netPortSubMenuType == "range" {
		return 4
	}
	return 3
}

// startPortEdit opens the port-edit submenu.
// idx == -1 means adding a new rule; idx >= 0 means editing the rule at that index.
func (m Model) startPortEdit(idx int) Model {
	opt := &m.categoryPages[tabIndexNET].Options[0]
	m.netPortEditing = true
	m.netPortEditIdx = idx
	m.netPortSubMenuCursor = 1 // start on Port/From field
	m.netPortSubMenuType = "single"
	m.subValues = [3]string{"", "", "tcp"}
	if idx >= 0 && idx < len(opt.PortRules) {
		r := opt.PortRules[idx]
		m.subValues[0] = r.From
		m.subValues[1] = r.To
		m.subValues[2] = r.Protocol
		if r.To != "" && r.To != r.From {
			m.netPortSubMenuType = "range"
		}
	}
	m.editingOption = false
	m.inputError = ""
	return m
}

// confirmPortEdit saves the completed port rule and clears edit state.
func (m Model) confirmPortEdit() Model {
	opt := &m.categoryPages[tabIndexNET].Options[0]
	rule := apply.PortRule{
		From:     m.subValues[0],
		To:       m.subValues[1],
		Protocol: m.subValues[2],
	}
	if m.netPortEditIdx >= 0 && m.netPortEditIdx < len(opt.PortRules) {
		opt.PortRules[m.netPortEditIdx] = rule
		m.netPortListCursor = m.netPortEditIdx
	} else {
		opt.PortRules = append(opt.PortRules, rule)
		m.netPortListCursor = len(opt.PortRules) - 1
	}
	m.netPortEditing = false
	m.editingOption = false
	m.inputSubStep = 0
	m.subValues = [3]string{}
	m.inputError = ""
	return m
}

// cancelPortEdit aborts the port-edit submenu and returns to the port list.
func (m Model) cancelPortEdit() Model {
	m.netPortEditing = false
	m.editingOption = false
	m.netPortSubMenuCursor = 0
	m.netPortSubMenuType = ""
	m.inputSubStep = 0
	m.subValues = [3]string{}
	m.inputError = ""
	return m
}


// syncNetTab detects current system state for NET tab options (run once, guarded by Synced).
func syncNetTab(pages []CategoryPage) []CategoryPage {
	if pages[tabIndexNET].Synced {
		return pages
	}
	opts := pages[tabIndexNET].Options
	for i := range opts {
		switch opts[i].Label {
		case "FW: Open ports":
			detected := apply.DetectOpenPorts()
			snapshot := make([]apply.PortRule, len(detected))
			copy(snapshot, detected)
			opts[i].PortRules = detected
			opts[i].DetectedPortRules = snapshot
		case "Enable fail2ban":
			active := apply.DetectFail2banActive()
			opts[i].Checked = active
			opts[i].OriginalChecked = active
		}
	}
	pages[tabIndexNET].Options = opts
	pages[tabIndexNET].Synced = true
	return pages
}

// syncPkgTab auto-toggles packages required by other tabs' checked options.
// Stub — logic added when PKG options are implemented.
func syncPkgTab(pages []CategoryPage) []CategoryPage { return pages }

// syncRunTab rebuilds the RUN tab based on what was selected in other tabs.
// Stub — logic added when RUN options are implemented.
func syncRunTab(pages []CategoryPage) []CategoryPage { return pages }

// injectHintsSep scans the rendered box lines for a full-width all-dashes line
// (│─────────│) and replaces the border chars with ├ and ┤ to make it look
// like a proper cross-section connector.
func injectHintsSep(boxLines []string) []string {
	for i, line := range boxLines {
		stripped := ansiEscape.ReplaceAllString(line, "")
		runes := []rune(stripped)
		if len(runes) < 3 || runes[0] != '│' || runes[len(runes)-1] != '│' {
			continue
		}
		allDashes := true
		for _, r := range runes[1 : len(runes)-1] {
			if r != '─' {
				allDashes = false
				break
			}
		}
		if !allDashes {
			continue
		}
		// Replace first │ with ├ and last │ with ┤ in the raw (ANSI) string.
		first := strings.Index(line, "│")
		if first >= 0 {
			line = line[:first] + "├" + line[first+len("│"):]
		}
		last := strings.LastIndex(line, "│")
		if last >= 0 {
			line = line[:last] + "┤" + line[last+len("│"):]
		}
		boxLines[i] = line
	}
	return boxLines
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

type bellClearedMsg struct{}

func bellCmd() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg { return bellClearedMsg{} })
}
