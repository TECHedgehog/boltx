//go:build !darwin

package apply

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Locale sets the system locale (LANG variable).
// Tries localectl set-locale first (systemd); falls back to writing /etc/default/locale.
// Requires root privileges.
func Locale(locale string) error {
	locale = strings.TrimSpace(locale)
	if locale == "" {
		return fmt.Errorf("locale cannot be empty")
	}
	if err := validateLocale(locale); err != nil {
		return err
	}

	if _, err := exec.LookPath("localectl"); err == nil {
		out, err := exec.Command("localectl", "set-locale", "LANG="+locale).CombinedOutput()
		if err != nil {
			return fmt.Errorf("localectl set-locale: %w\n%s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}

	if err := os.MkdirAll("/etc/default", 0755); err != nil {
		return fmt.Errorf("mkdir /etc/default: %w", err)
	}
	content := "LANG=" + locale + "\n"
	if err := os.WriteFile("/etc/default/locale", []byte(content), 0644); err != nil {
		return fmt.Errorf("write /etc/default/locale: %w", err)
	}
	return nil
}
