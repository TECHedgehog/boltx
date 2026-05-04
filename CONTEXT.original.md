# boltx — project context

> **Session startup (MANDATORY):** Invoke `/caveman ultra` — NOT `/caveman` (default=full, wrong). Use **ultra** level every session start.

## What this is
TUI tool (Go + Bubbletea + Lipgloss) for setting up Linux systems.
Target users: Linux newcomers and experienced lazy users.

## Use cases
- VPS (remote server)
- Test VM / Dev machine
(auto-detected via systemd-detect-virt, user confirms)

## Stack
- Go 1.22
- github.com/charmbracelet/bubbletea (TUI engine)
- github.com/charmbracelet/lipgloss (styling)

## Easy-to-tune constants (styles.go)
- `leftColMaxW=35`, `leftColMinW=28` — left column width bounds
- `tallestBoxH=24` — worst-case box height for vertical anchor
- `optionInputWidth=40` — KindTextInput field width + col fallback
- `bellClearDur=150ms` — bell flash duration
- `splashTargetFrames`, `splashTickDur`, `splashHoldTotal`, `splashColJitter` in splash.go

## Structure
```
boltx/
├── main.go
└── internal/
    ├── ui/
    │   ├── model.go    ← main TUI model, menu navigation
    │   └── styles.go   ← all lipgloss styles
    ├── detect/         ← OS/virt/hostname detection (viaSSH: env vars + /proc parent-chain walk)
    └── apply/          ← apply functions (one file per tab)
        ├── sys.go
        └── usr.go
```

## Current stage
SYS tab complete (Hostname, Locale, Timezone — all with apply functions and validation).
USR tab complete: multi-user TUI, per-user sub-tabs, username rename, password set/change, sudo toggle, SSH key management, active-session detection, delete user. All with apply functions and validation.
SEC tab complete: SSH hardening (PermitRootLogin cycle, key-only auth, port), UFW firewall toggle; drop-in config; cross-distro sshd restart.
NET tab complete: FW preset+port list (`KindPortList` — cycle presets, add/edit/remove individual rules), proxy manager cycle (none/traefik/nginx), fail2ban toggle + preset cycle; priority-ordered apply; `syncNetTab` detects live fail2ban state.
Next: RUN tab (runtime config; proxy service config for traefik/nginx selected in NET; Docker container lifecycle).

## Architecture

### Option system
- `CategoryOption` has `Kind`, `Default`, `Value`, `ApplyFn`, `Priority` fields
- `OptionKind`: `KindToggle`, `KindTextInput`, `KindSelect`, `KindCycle`, `KindPortList` (NET tab firewall — preset cycle + expandable port rule list)
- `editingOption bool` + `textInput textinput.Model` on `Model` for inline editing
- `buildCategoryPages` takes `(uc detect.UseCase, osInfo detect.OSInfo)`

### Priority system (GO! apply order)
- `CategoryOption.Priority int` — GO! execution order; lower = earlier; 0 treated as `PrioConfigWrite`
- Constants: `PrioPackageInstall=10`, `PrioConfigWrite=20`, `PrioServiceRestart=30`, `PrioFirewallRule=40`
- `doApplyAll` collects all actions into `[]applyAction{priority, fn}`, stable-sorts by priority, then executes
- USR `UserEntry` ops collected as a single group at `PrioConfigWrite`; internal sequence preserved via `applyUserEntries()`

### Queued indicators
- `isOptionQueued(opt CategoryOption) bool` — KindCycle: `Value != Default`; KindTextInput/Select: `Value != "" && Value != Default`; KindPortList: `Value != Default || len(PortRules) > 0`; KindToggle w/ UndoFn: `Checked != OriginalChecked`; KindToggle w/o UndoFn: `Checked`
- `queuedStyle` — `t.Queued` color per theme; Purple=`#F59E0B` amber, Teal=`#F97316` orange, Amber=`#FBBF24` yellow; chosen to not repeat Accent/Success/Muted/Text in each theme
- `Theme.Dim` — darkened accent for splash noise chars; Purple=`#3B1A7A`, Teal=`#134E4A`, Amber=`#78350F`; lives in Theme struct, eliminates `splashDimColor` switch
- `pendingRemoveStyle` — errorStyle + strikethrough; used for SSH keys pending removal
- In `viewCategoryReviewBody`: queued text/select/cycle → value in `queuedStyle`; queued toggle → trailing `queuedStyle.Render(" ·")`
- `CategoryOption.ValidateFn func(string) error` — optional; called on confirm before saving Value; blocks save + sets `inputError` on failure
- `apply.ValidatePort(s string) error` — numeric, 1–65535; wired as `ValidateFn` on SSH Port option
- USR tab: per-row queued flags (username rename, password set, sudo change, SSH delta); value text in `queuedStyle`; sudo toggle gets ` ·`; SSH Keys row shows `(n) +a -b` with delta in `queuedStyle`
- `sshKeyDelta(u UserEntry) (added, removed int)` — counts keys added/removed vs `OriginalSSHKeys`

### Tab layout (7 tabs)
`SYS → USR → SEC → NET → PKG → RUN → GO!`
Constants: `tabIndexSYS=0` … `tabIndexGO=6`
`syncOnTabEnter` dispatches to `syncPkgTab` / `syncRunTab` stubs on navigation.

| Tab | Icon | Description |
|-----|------|-------------|
| SYS | ⚙️ | Base system state (updates, hostname, locale) |
| USR | 👤 | Users, sudo access, and SSH keys |
| SEC | 🛡️ | Access control and SSH hardening (root, login rules, firewall) |
| NET | 🌐 | Network exposure and service traffic (ports, proxies, fail2ban) |
| PKG | 📦 | Install system packages, tools, and services |
| RUN | 🚀 | Runtime setup, shell, automation, and system lifecycle tools |
| GO! | — | Apply all selected options |

### SYS tab (complete)
- **Hostname**: `KindTextInput`, pre-filled via `detect.OSInfo.Hostname` (`os.Hostname()`)
  - `apply.Hostname(name)`: tries `hostnamectl set-hostname` first, falls back to `/etc/hostname` + `hostname(1)`; requires root
- **Locale**: `KindSelect`, list from `locale -a`; `apply.Locale(locale)`: writes `/etc/default/locale`, calls `localectl set-locale`
- **Timezone**: `KindSelect`, list from `detect.DetectTimezones()` (walks `/usr/share/zoneinfo`, resolves symlink for macOS); pre-selected to `osInfo.Timezone` (detected from `/etc/localtime` symlink on macOS); `apply.Timezone(zone)`: calls `timedatectl set-timezone`
- All three have input validation and tests in `sys_test.go`

### USR tab (in progress)
- `apply/usr.go`: common types/helpers — `HumanUser`, `loadGroupMembers`, `sudoGroup` (checks sudo/wheel/admin), `ValidateUsername`, `UserExists`
- `apply/usr_linux.go`: Linux impls — `LoadHumanUsers` (UID≥1000, /etc/passwd), `HasActiveProcesses` (/proc), `CreateUser` (useradd + openssl + usermod), `ChangePassword`, `RenameUser`, `DeleteUser`, `AddSudo` (usermod -aG), `RemoveSudo` (gpasswd)
- `apply/usr_darwin.go`: macOS impls — `LoadHumanUsers` (dscl, UID≥501), `HasActiveProcesses` (ps -u), `CreateUser` (dscl + createhomedir), `ChangePassword`, `RenameUser`, `DeleteUser`, `AddSudo`/`RemoveSudo` (dseditgroup)
- `UserEntry.OriginalName` — set at load time; GO! renames first if `Name != OriginalName`, then applies password/sudo with new name
- `UserEntry.ActiveSession` — set at `syncUsrTab`; username row shows `(active)` and blocks editing when true
- `apply/usr_test.go`: validation tests
- Multi-user TUI: per-user sub-tabs, sliding window nav, "+ New User" tab
- Per-user options (all users): Username (`▶`), Password (`▶`), Sudo (`●`/`○`), SSH key (`▶`), Delete
  - Existing users: Password shows `(unchanged)`/`(set)`; edit stores to `UserEntry.NewPassword`; applied via `ChangePassword` at GO!
  - New users: Password stores to `UserEntry.Password`; applied via `CreateUser` at GO!
- Kind markers match SYS tab style throughout

### Apply strategy
- **Deferred + priority-ordered**: GO! collects all queued `applyAction{priority, fn}` across all tabs, stable-sorts by `Priority`, executes in order
- **Cross-tab**: NET fail2ban toggle → `syncPkgTab` auto-checks fail2ban in PKG (stub until PKG implemented); NET proxy selection likewise
- **RUN tab**: dynamic — rebuilt by `syncRunTab` on navigation; shows runtime config for checked options

### apply/ package
One file per tab: `sys.go`, `usr.go`, `sec.go`, `net.go`…
- `sys.go`: `Hostname`, `Locale`, `Timezone` — all `func(string) error`
- `usr.go`: common types/helpers; `usr_linux.go` / `usr_darwin.go`: platform impls
- `sec.go`: `editSSHDConfigAt`, `detectSSHDConfigAt`, `DetectSSHDConfig` (boltx.conf-first), `FirewallActive`, `ensureSSHDDropIn`, `editBoltxConf`, `ApplySSHOption`
  - All SSH key writes go to `/etc/ssh/sshd_config.d/99-boltx.conf`; `ensureSSHDDropIn` creates dir + injects `Include` line into `sshd_config` once if absent (root-guarded)
- `sec_linux.go`: `DisableRootLogin`, `EnableRootLogin`, `DisablePasswordAuth`, `EnablePasswordAuth`, `EnableFirewall`, `DisableFirewall`, `restartSSHD`, `firewallActive`
- `sec_darwin.go`: same signatures; firewall fns return error stub, `firewallActive` returns false

## Key decisions
- Single compiled binary, no runtime dependencies
- Use case sets profile (defaults/recommendations), nothing forced
- Rewriting bash logic natively in Go — no `exec.Command` wrappers
- Each screen is a Stage — build one at a time
- **Finish one option end-to-end** (including apply) before starting the next
- Run `sudo ./boltx` to test apply functions

### SEC tab (complete)
- **SSH: Permit root login** — `KindCycle`, NeedsRoot; `SelectItems`: `["no","prohibit-password","forced-commands-only","yes"]`; `ApplyFn`: `ApplySSHOption("PermitRootLogin", v)`; default `"no"`; `syncSecTab` sets `Value` from detected system state; marker `↻`
- **SSH: Require key auth only** — `KindToggle`, NeedsRoot; `ApplyFn`: `DisablePasswordAuth()` (`PasswordAuthentication no`); `UndoFn`: `EnablePasswordAuth()` (`PasswordAuthentication yes`)
- **SSH: Port** — `KindTextInput`, NeedsRoot, `ClearOnEdit: true`; `Default` set from detected Port at sync (not `Value`); opens empty field with current port as placeholder; `ApplyFn`: `ApplySSHPort(v)`; updates UFW rules if firewall active
- **Enable UFW firewall** — `KindToggle`, NeedsRoot; `ApplyFn`: `EnableFirewall()`; `UndoFn`: `DisableFirewall()`
- `syncSecTab()` runs once (guarded by `CategoryPage.Synced`); detects live sshd_config + ufw state and sets `Checked` bidirectionally; subsequent tab entries preserve user-toggled state
- `CategoryOption.UndoFn`: called at GO! when `Checked=false`; allows options to revert as well as apply
- `CategoryOption.ClearOnEdit bool`: KindTextInput opens empty (placeholder=Default) instead of pre-filling Value
- r/R reset: `resetOption(*CategoryOption)` — KindCycle → `Value=Default`; KindTextInput → `Value=""` + `Checked=false`
- Tests in `apply/sec_test.go` cover key-present, key-commented, key-absent, and file-missing cases

### apply/sec.go — cross-distro notes
- `editBoltxConf`: routes by OpenSSH version — ≥ 7.3 uses drop-in `00-boltx.conf`; < 7.3 edits `sshd_config` directly
- `opensshSupportsInclude()`: parses `sshd -V`; major > 7 || (major == 7 && minor >= 3)
- `restartSSHD` (Linux, sec_linux.go): tries systemd → OpenRC (`rc-service sshd restart`) → SysV (`service`) before failing
- `restartSSHD` (macOS, sec_darwin.go): checks `launchctl list com.openssh.sshd` first; fails with clear message if Remote Login disabled
- `updateFirewallForPort(old, new)`: Linux — `ufw allow <new>/tcp`, deletes old port + OpenSSH rules; macOS — no-op
- UFW: Linux-only; `EnableFirewall` and `DisableFirewall` on macOS return error stub; `FirewallActive` on macOS always false

### NET tab (complete)
- **FW: Open ports** — `KindPortList`; disabled (muted, non-interactive, hint shown) when `ufwChecked()` is false (SEC tab "Enable UFW firewall" unchecked); `syncNetTab` detects live UFW rules via `DetectOpenPorts()`, populates `PortRules` (Existing=true) and `DetectedPortRules`; enter opens sub-list; cursor 0..N-1=rules, N="Add port", N+1="Add presets"; existing rules shown with `(active)` label; j/k navigate; r removes; e/enter on rule → port-edit submenu; port-edit submenu: Type (single/range), Port/From, To (range), Proto, Confirm; "Add presets" → inline toggle list (Web/Minecraft/SSH) + Confirm — adds selected preset rules, skips duplicates; esc closes layers; `isOptionQueued`: any non-Existing rule OR any DetectedPortRule removed; `doApplyAll` computes delta: `ApplyFirewallRules(toAdd)` + `DeleteFirewallRules(toRemove)`; `resetOption` restores DetectedPortRules
- **Proxy** — `KindCycle` (none/traefik/nginx); no-op at apply (RUN tab handles config); `PrioConfigWrite`
- **Enable fail2ban** — `KindToggle`, NeedsRoot; `ApplyFn=EnableFail2ban`, `UndoFn=DisableFail2ban`; `PrioServiceRestart`; `syncNetTab` detects live state
- **fail2ban: Preset** — `KindCycle` (ssh/web/all); writes `/etc/fail2ban/jail.local`; `PrioConfigWrite` (runs before service start)
- `apply/net.go`: `PortRule`, `ValidateNetPort`, `ValidatePortRange`, `PresetPortRules`, `ApplyFirewallRules`, `SetProxyManager`, `WriteFail2banConfig`, `EnableFail2ban`, `DisableFail2ban`, `DetectFail2banActive`
- `apply/net_linux.go`: UFW rule exec, jail.local write, systemctl enable/start/stop/disable/is-active
- `apply/net_darwin.go`: stubs returning errors
- Model state: `netPortListOpen`, `netPortListCursor` (0..N-1=rules,N=add,N+1=presets), `netPortEditing`, `netPortEditIdx`, `netPortSubMenuCursor`, `netPortSubMenuType` ("single"/"range"), `netPortPresetsOpen`, `netPortPresetCursor`, `netPortPresetSel []bool`
- Helpers: `startPortEdit(idx)`, `confirmPortEdit()`, `cancelPortEdit()`, `viewPortSubMenu(indent)`, `portSubMenuProtoIdx()`, `portSubMenuConfirmIdx()`, `portRuleKeySet()`, `portRuleKey{}`
- `internal/ui/ufw_presets.json` — UFW preset definitions (label + rules); embedded via `//go:embed`; loaded into `netPresets []netPreset` at init; edit + rebuild to add presets
- `PortRule` has json tags (`from`/`to`/`protocol`); `Existing` is `json:"-"` (runtime-only)
- `PortRule.Existing bool` — set by `DetectOpenPorts()`; `DetectedPortRules []PortRule` on `CategoryOption`

## Splash screen (`internal/ui/splash.go`)
- Kban logo (embedded), column-rain animation (top→down per column, random column offsets)
- `pageSplash` is the initial page (before `pageWelcome`)
- Phases: reveal (~1.9s) → hold (3s, theme cycles Purple→Teal→Amber→Purple) → dismiss (~1.9s, same direction as reveal = top-down)
- Any keypress skips to `pageWelcome`; `splashGen` int invalidates stale tick streams
- Theme changes during hold call `applyTheme()` + update `m.themeIdx` → box border cycles too
- Logo viewer tool: `cmd/splash/` — 9 anims, speed ↑↓, live reload, selection → `assets/selected_logo.txt`

### PKG tab (complete)
- 13 packages: git, curl, wget, htop, vim, tmux, unzip, rsync, jq, nginx, fail2ban, ufw, docker — all `KindToggle`, `NeedsRoot: true`, `Priority: PrioPackageInstall`
- `apply/pkg.go`: `InstallPackage(pm, label)`, `DetectPackageInstalled(pm, label)`
- `apply/pkg_linux.go`: PM dispatch (apt/dnf/pacman/apk/zypper); docker → `docker.io` on apt; detection via dpkg-query / rpm -q / pacman -Q / apk info -e
- `apply/pkg_darwin.go`: stubs returning errors
- `syncPkgTab(pages, pm)`: detects installed packages → sets `OriginalChecked`; cross-tab wiring (runs once, Synced guard): NET fail2ban on → fail2ban checked; NET proxy=nginx → nginx checked; NET proxy=traefik → docker checked
- `syncOnTabEnter` signature: `(tabIdx int, pages []CategoryPage, pm detect.PackageManager)` — both call sites pass `m.osInfo.Pkg`
- traefik runs as Docker container; RUN tab handles container lifecycle

## What comes next
1. RUN tab (runtime config; Docker container lifecycle for traefik/nginx; proxy service config)
