package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"boltx/internal/ui"
)

func main() {
	p := tea.NewProgram(ui.NewModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
