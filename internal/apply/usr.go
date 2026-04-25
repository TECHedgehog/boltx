package apply

import (
	"bufio"
	"fmt"
	"os"
	"os/user"
	"regexp"
	"strings"
)

// HumanUser holds basic info about an existing system user.
type HumanUser struct {
	Name string
	Sudo bool
}

// loadGroupMembers returns a set of usernames in the sudo, wheel, or admin group.
func loadGroupMembers() map[string]bool {
	members := map[string]bool{}
	f, err := os.Open("/etc/group")
	if err != nil {
		return members
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Split(line, ":")
		if len(fields) < 4 {
			continue
		}
		groupName := fields[0]
		if groupName != "sudo" && groupName != "wheel" && groupName != "admin" {
			continue
		}
		for _, m := range strings.Split(fields[3], ",") {
			m = strings.TrimSpace(m)
			if m != "" {
				members[m] = true
			}
		}
	}
	return members
}

// sudoGroup returns the first of sudo/wheel/admin that exists on this system.
func sudoGroup() (string, error) {
	for _, g := range []string{"sudo", "wheel", "admin"} {
		if _, err := user.LookupGroup(g); err == nil {
			return g, nil
		}
	}
	return "", fmt.Errorf("no sudo/wheel/admin group found on this system")
}

var usernameRe = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

func ValidateUsername(name string) error {
	if len(name) > 32 {
		return fmt.Errorf("username too long (%d chars, max 32)", len(name))
	}
	if !usernameRe.MatchString(name) {
		return fmt.Errorf("invalid username: must start with a-z or _, contain only a-z 0-9 _ -")
	}
	return nil
}

// UserExists reports whether a user with the given name already exists on the system.
func UserExists(name string) bool {
	_, err := user.Lookup(name)
	return err == nil
}
