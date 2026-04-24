# boltx

![boltx](assets/gifs/boltx.gif)

A terminal tool for setting up Linux systems. Whether you're spinning up a VPS or a dev machine, boltx walks you through the usual setup steps — hostname, users, security, packages — and applies them all at once when you're ready.

Built for people who know what they want but don't want to type the same commands every time.

## What it configures

| Tab | What you can set |
|-----|-----------------|
| SYS | Hostname, locale, timezone |
| USR | Create users, set passwords, sudo access, SSH keys |
| SEC | Firewall, fail2ban, SSH hardening |
| NET | Web servers, reverse proxies |
| PKG | System updates, essential packages |
| GO! | Review and apply everything |

Pick your options across tabs, then hit GO! to apply them all.

## Status

Early development. SYS tab is fully working. USR tab (user creation and password management) is in progress. Other tabs are placeholders.

## Keybindings

| Key | Action |
|-----|--------|
| `↑↓` / `jk` | Navigate |
| `←→` / `hl` | Switch tabs |
| `enter` / `space` | Select / toggle option |
| `r` | Reset current option to default |
| `R` | Reset all options in current tab |
| `t` | Cycle theme (Purple → Teal → Amber) |
| `?` | Toggle help |
| `q` / `esc` | Back / quit |

## Built with

- [Bubbletea](https://github.com/charmbracelet/bubbletea) — TUI framework
- [Lipgloss](https://github.com/charmbracelet/lipgloss) — terminal styling
- [Bubbles](https://github.com/charmbracelet/bubbles) — UI components

## Install & run

**Requirements:** Go 1.24+

```bash
# Run directly
go run .

# Build a binary
go build -o boltx .
sudo ./boltx

# Install to $GOPATH/bin
go install .
```

Most apply steps require root (`sudo`).

## Roadmap

- [x] Main menu with use-case detection (VPS vs dev machine)
- [x] Per-category options with tab navigation
- [x] Theme switching
- [x] SYS tab: hostname, locale, timezone
- [x] Exit prompt after apply
- [ ] USR tab: user creation, passwords, sudo, SSH keys (in progress)
- [ ] SEC, NET, PKG tab options

## Support

If you find this useful, consider buying me a coffee: [ko-fi.com/ericllaca](https://ko-fi.com/ericllaca)
