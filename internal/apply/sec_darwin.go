//go:build darwin

package apply

import (
	"fmt"
	"os/exec"
	"strings"
)

// DisablePasswordAuth writes PasswordAuthentication no to 00-boltx.conf and restarts sshd.
func DisablePasswordAuth() error {
	if err := editBoltxConf("PasswordAuthentication", "no"); err != nil {
		return err
	}
	return restartSSHD()
}

// EnablePasswordAuth writes PasswordAuthentication yes to 00-boltx.conf and restarts sshd.
func EnablePasswordAuth() error {
	if err := editBoltxConf("PasswordAuthentication", "yes"); err != nil {
		return err
	}
	return restartSSHD()
}

// EnableFirewall is not supported on macOS (UFW is Linux-only).
func EnableFirewall() error {
	return fmt.Errorf("UFW not available on macOS")
}

// DisableFirewall is not supported on macOS.
func DisableFirewall() error {
	return fmt.Errorf("UFW not available on macOS")
}

func restartSSHD() error {
	if _, err := exec.Command("launchctl", "list", "com.openssh.sshd").CombinedOutput(); err != nil {
		return fmt.Errorf("SSH not enabled — enable Remote Login in System Settings > General > Sharing first")
	}
	cmds := [][]string{
		{"launchctl", "stop", "com.openssh.sshd"},
		{"launchctl", "start", "com.openssh.sshd"},
	}
	for _, args := range cmds {
		out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func updateFirewallForPort(_, _ string) error { return nil }

func firewallActive() bool { return false }
