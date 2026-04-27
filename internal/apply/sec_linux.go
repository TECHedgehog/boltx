//go:build !darwin

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

// EnableFirewall configures UFW with deny-incoming defaults and enables it.
func EnableFirewall() error {
	if _, err := exec.LookPath("ufw"); err != nil {
		return fmt.Errorf("ufw not found — install ufw first")
	}
	cmds := [][]string{
		{"ufw", "default", "deny", "incoming"},
		{"ufw", "default", "allow", "outgoing"},
		{"ufw", "allow", "OpenSSH"},
		{"ufw", "--force", "enable"},
	}
	for _, args := range cmds {
		out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func restartSSHD() error {
	// systemd
	for _, svc := range []string{"sshd", "ssh"} {
		if _, err := exec.Command("systemctl", "restart", svc).CombinedOutput(); err == nil {
			return nil
		}
	}
	// OpenRC (Alpine, Gentoo)
	if _, err := exec.Command("rc-service", "sshd", "restart").CombinedOutput(); err == nil {
		return nil
	}
	// SysV
	for _, svc := range []string{"sshd", "ssh"} {
		if _, err := exec.Command("service", svc, "restart").CombinedOutput(); err == nil {
			return nil
		}
	}
	return fmt.Errorf("could not restart sshd: no supported init system found (tried systemctl, rc-service, service)")
}

func updateFirewallForPort(oldPort, newPort string) error {
	if !firewallActive() {
		return nil
	}
	if out, err := exec.Command("ufw", "allow", newPort+"/tcp").CombinedOutput(); err != nil {
		return fmt.Errorf("ufw allow %s/tcp: %w\n%s", newPort, err, strings.TrimSpace(string(out)))
	}
	exec.Command("ufw", "delete", "allow", oldPort+"/tcp").Run()
	exec.Command("ufw", "delete", "allow", "OpenSSH").Run()
	return nil
}

// DisableFirewall disables UFW.
func DisableFirewall() error {
	if _, err := exec.LookPath("ufw"); err != nil {
		return fmt.Errorf("ufw not found")
	}
	out, err := exec.Command("ufw", "--force", "disable").CombinedOutput()
	if err != nil {
		return fmt.Errorf("ufw disable: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func firewallActive() bool {
	out, err := exec.Command("ufw", "status").CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "Status: active")
}
