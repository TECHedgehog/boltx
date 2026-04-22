package apply

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

var usernameRe = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

func validateUsername(name string) error {
	if len(name) > 32 {
		return fmt.Errorf("username too long (%d chars, max 32)", len(name))
	}
	if !usernameRe.MatchString(name) {
		return fmt.Errorf("invalid username %q (must start with a-z or _, contain only a-z, 0-9, _, -; max 32 chars)", name)
	}
	return nil
}

// CreateUser creates a new Linux user with a home directory and sets its password.
// Requires root privileges.
func CreateUser(name, password string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("username cannot be empty")
	}
	if err := validateUsername(name); err != nil {
		return err
	}
	if password == "" {
		return fmt.Errorf("password cannot be empty")
	}
	if _, err := exec.LookPath("useradd"); err != nil {
		return fmt.Errorf("useradd not found: %w", err)
	}
	out, err := exec.Command("useradd", "-m", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("useradd -m %s: %w\n%s", name, err, strings.TrimSpace(string(out)))
	}
	if _, err := exec.LookPath("chpasswd"); err != nil {
		return fmt.Errorf("chpasswd not found: %w", err)
	}
	cmd := exec.Command("chpasswd")
	cmd.Stdin = strings.NewReader(name + ":" + password + "\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("chpasswd: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
