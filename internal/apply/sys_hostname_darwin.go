//go:build darwin

package apply

import (
	"fmt"
	"os/exec"
	"strings"
)

// Hostname sets the system hostname persistently via scutil.
// Sets HostName, LocalHostName, and ComputerName — all three are needed
// so the name survives reboots and network changes on macOS.
// Requires root privileges.
func Hostname(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("hostname cannot be empty")
	}
	if err := validateHostname(name); err != nil {
		return err
	}

	for _, key := range []string{"HostName", "LocalHostName", "ComputerName"} {
		out, err := exec.Command("scutil", "--set", key, name).CombinedOutput()
		if err != nil {
			return fmt.Errorf("scutil --set %s: %w\n%s", key, err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}
