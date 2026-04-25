//go:build !darwin

package apply

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Hostname sets the system hostname.
// Tries hostnamectl first (systemd); falls back to writing /etc/hostname and hostname(1).
// Requires root privileges.
func Hostname(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("hostname cannot be empty")
	}
	if err := validateHostname(name); err != nil {
		return err
	}

	if _, err := exec.LookPath("hostnamectl"); err == nil {
		out, err := exec.Command("hostnamectl", "set-hostname", name).CombinedOutput()
		if err != nil {
			return fmt.Errorf("hostnamectl set-hostname: %w\n%s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}

	if err := os.WriteFile("/etc/hostname", []byte(name+"\n"), 0644); err != nil {
		return fmt.Errorf("write /etc/hostname: %w", err)
	}
	out, err := exec.Command("hostname", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("hostname %s: %w\n%s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}
