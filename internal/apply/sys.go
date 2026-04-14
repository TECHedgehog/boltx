package apply

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Hostname sets the system hostname.
// Tries hostnamectl first (systemd systems); falls back to writing /etc/hostname
// and running hostname(1) directly (Alpine, minimal containers).
// Requires root privileges — returns a descriptive error if the call fails.
func Hostname(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("hostname cannot be empty")
	}

	// systemd path — available on Ubuntu, Fedora, Arch, Debian, etc.
	if _, err := exec.LookPath("hostnamectl"); err == nil {
		out, err := exec.Command("hostnamectl", "set-hostname", name).CombinedOutput()
		if err != nil {
			return fmt.Errorf("hostnamectl set-hostname: %w\n%s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}

	// Fallback: write /etc/hostname and call hostname(1) to apply immediately.
	if err := os.WriteFile("/etc/hostname", []byte(name+"\n"), 0644); err != nil {
		return fmt.Errorf("write /etc/hostname: %w", err)
	}
	out, err := exec.Command("hostname", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("hostname %s: %w\n%s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}
