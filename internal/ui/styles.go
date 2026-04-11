package ui

import "github.com/charmbracelet/lipgloss"

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
	helpStyle     = lipgloss.NewStyle().Foreground(muted)

	cursorStr   = "› "
	noCursorStr = "  "
)
