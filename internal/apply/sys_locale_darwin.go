//go:build darwin

package apply

import (
	"fmt"
	"os/exec"
	"strings"
)

// Locale sets LANG for the current launchd session via launchctl setenv.
// New Terminal.app windows will pick it up. Does not modify System Settings.
// Requires root privileges.
func Locale(locale string) error {
	locale = strings.TrimSpace(locale)
	if locale == "" {
		return fmt.Errorf("locale cannot be empty")
	}
	if err := validateLocale(locale); err != nil {
		return err
	}
	out, err := exec.Command("launchctl", "setenv", "LANG", locale).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl setenv LANG: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
