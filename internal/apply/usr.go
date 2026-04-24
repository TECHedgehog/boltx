package apply

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"regexp"
	"strconv"
	"strings"
)

// HumanUser holds basic info about an existing system user.
type HumanUser struct {
	Name string
	Sudo bool
}

// LoadHumanUsers returns all users with UID ≥ 1000 from /etc/passwd,
// with sudo group membership detected from /etc/group.
func LoadHumanUsers() []HumanUser {
	sudoMembers := loadGroupMembers()

	f, err := os.Open("/etc/passwd")
	if err != nil {
		return nil
	}
	defer f.Close()

	var users []HumanUser
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || line[0] == '#' {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) < 7 {
			continue
		}
		uid, err := strconv.Atoi(fields[2])
		if err != nil || uid < 1000 {
			continue
		}
		name := fields[0]
		shell := fields[6]
		if shell == "/sbin/nologin" || shell == "/usr/sbin/nologin" || shell == "/bin/false" {
			continue
		}
		users = append(users, HumanUser{Name: name, Sudo: sudoMembers[name]})
	}
	return users
}

// loadGroupMembers returns a set of usernames that belong to sudo or wheel group.
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
		if groupName != "sudo" && groupName != "wheel" {
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

// sudoGroup returns "sudo" (Debian/Ubuntu) or "wheel" (RHEL/Arch/Fedora),
// whichever exists on this system.
func sudoGroup() (string, error) {
	for _, g := range []string{"sudo", "wheel"} {
		if _, err := user.LookupGroup(g); err == nil {
			return g, nil
		}
	}
	return "", fmt.Errorf("neither 'sudo' nor 'wheel' group found on this system")
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

// HasActiveProcesses reports whether any running process is owned by name.
func HasActiveProcesses(name string) bool {
	u, err := user.Lookup(name)
	if err != nil {
		return false
	}
	dir, err := os.Open("/proc")
	if err != nil {
		return false
	}
	defer dir.Close()
	entries, err := dir.Readdirnames(-1)
	if err != nil {
		return false
	}
	for _, e := range entries {
		status, err := os.ReadFile("/proc/" + e + "/status")
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(status), "\n") {
			if strings.HasPrefix(line, "Uid:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 && fields[1] == u.Uid {
					return true
				}
				break
			}
		}
	}
	return false
}

// hashPassword returns a SHA-512 crypt hash via /usr/bin/openssl.
// Uses usermod -p instead of chpasswd to bypass PAM quality checks.
func hashPassword(password string) (string, error) {
	cmd := exec.Command("/usr/bin/openssl", "passwd", "-6", "-stdin")
	cmd.Stdin = strings.NewReader(password)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("openssl passwd: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// setPassword sets a pre-hashed password for name via usermod -p.
func setPassword(name, password string) error {
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	if _, err := exec.LookPath("usermod"); err != nil {
		return fmt.Errorf("usermod not found: %w", err)
	}
	if out, err := exec.Command("usermod", "-p", hash, name).CombinedOutput(); err != nil {
		return fmt.Errorf("usermod -p %s: %w\n%s", name, err, strings.TrimSpace(string(out)))
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
	if err := ValidateUsername(name); err != nil {
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
	return setPassword(name, password)
}

// ChangePassword sets a new password for an existing user.
// Requires root privileges.
func ChangePassword(name, password string) error {
	if name == "" {
		return fmt.Errorf("username cannot be empty")
	}
	if password == "" {
		return fmt.Errorf("password cannot be empty")
	}
	return setPassword(name, password)
}

// RenameUser changes a user's login name and moves their home directory.
// Requires root privileges.
func RenameUser(oldName, newName string) error {
	if oldName == "" || newName == "" {
		return fmt.Errorf("username cannot be empty")
	}
	if _, err := exec.LookPath("usermod"); err != nil {
		return fmt.Errorf("usermod not found: %w", err)
	}
	out, err := exec.Command("usermod", "-l", newName, "-d", "/home/"+newName, "-m", oldName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("usermod rename %s→%s: %w\n%s", oldName, newName, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// DeleteUser removes a user and their home directory from the system.
// Requires root privileges.
func DeleteUser(name string) error {
	if _, err := exec.LookPath("userdel"); err != nil {
		return fmt.Errorf("userdel not found: %w", err)
	}
	out, err := exec.Command("userdel", "-r", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("userdel -r %s: %w\n%s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// RemoveSudo removes a user from the sudo/wheel group.
// Requires root privileges.
func RemoveSudo(name string) error {
	group, err := sudoGroup()
	if err != nil {
		return err
	}
	if _, err := exec.LookPath("gpasswd"); err != nil {
		return fmt.Errorf("gpasswd not found: %w", err)
	}
	out, err := exec.Command("gpasswd", "-d", name, group).CombinedOutput()
	if err != nil {
		return fmt.Errorf("gpasswd -d %s %s: %w\n%s", name, group, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// AddSudo adds an existing user to the sudo/wheel group.
// Requires root privileges.
func AddSudo(name string) error {
	group, err := sudoGroup()
	if err != nil {
		return err
	}
	if _, err := exec.LookPath("usermod"); err != nil {
		return fmt.Errorf("usermod not found: %w", err)
	}
	out, err := exec.Command("usermod", "-aG", group, name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("usermod -aG %s %s: %w\n%s", group, name, err, strings.TrimSpace(string(out)))
	}
	return nil
}
