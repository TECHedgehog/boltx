//go:build darwin

package apply

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// LoadHumanUsers returns human users via dscl (macOS OpenDirectory).
// Includes users with UID ≥ 501, skipping underscore-prefixed system accounts.
func LoadHumanUsers() []HumanUser {
	sudoMembers := loadGroupMembers()

	out, err := exec.Command("dscl", ".", "-list", "/Users", "UniqueID").Output()
	if err != nil {
		return nil
	}

	var users []HumanUser
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		name := fields[0]
		uid, err := strconv.Atoi(fields[1])
		if err != nil || uid < 501 {
			continue
		}
		if strings.HasPrefix(name, "_") {
			continue
		}
		users = append(users, HumanUser{Name: name, Sudo: sudoMembers[name]})
	}
	return users
}

// HasActiveProcesses reports whether any running process is owned by name.
func HasActiveProcesses(name string) bool {
	out, err := exec.Command("ps", "-u", name, "-o", "pid=").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

// nextAvailableUID finds the highest UID in use and returns the next one.
func nextAvailableUID() (int, error) {
	out, err := exec.Command("dscl", ".", "-list", "/Users", "UniqueID").Output()
	if err != nil {
		return 0, err
	}
	maxUID := 500
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		uid, err := strconv.Atoi(fields[1])
		if err != nil || uid >= 60000 {
			continue
		}
		if uid > maxUID {
			maxUID = uid
		}
	}
	return maxUID + 1, nil
}

// CreateUser creates a new macOS user via dscl and sets its password.
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

	uid, err := nextAvailableUID()
	if err != nil {
		return fmt.Errorf("find next UID: %w", err)
	}

	steps := [][]string{
		{"dscl", ".", "-create", "/Users/" + name},
		{"dscl", ".", "-create", "/Users/" + name, "UserShell", "/bin/bash"},
		{"dscl", ".", "-create", "/Users/" + name, "RealName", name},
		{"dscl", ".", "-create", "/Users/" + name, "UniqueID", strconv.Itoa(uid)},
		{"dscl", ".", "-create", "/Users/" + name, "PrimaryGroupID", "20"},
		{"dscl", ".", "-create", "/Users/" + name, "NFSHomeDirectory", "/Users/" + name},
		{"createhomedir", "-c", "-u", name},
	}
	for _, args := range steps {
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %w\n%s", args[0], err, strings.TrimSpace(string(out)))
		}
	}

	// New users have no Secure Token yet, so dscl -passwd works directly.
	if out, err := exec.Command("dscl", ".", "-passwd", "/Users/"+name, password).CombinedOutput(); err != nil {
		return fmt.Errorf("dscl -passwd %s: %w\n%s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ChangePassword sets a new password for an existing macOS user.
// oldPassword is required on macOS to re-encrypt the Secure Token.
// Requires root privileges.
func ChangePassword(name, oldPassword, newPassword string) error {
	if name == "" {
		return fmt.Errorf("username cannot be empty")
	}
	if newPassword == "" {
		return fmt.Errorf("password cannot be empty")
	}
	out, err := exec.Command("/usr/sbin/sysadminctl",
		"-resetPasswordFor", name,
		"-oldPassword", oldPassword,
		"-newPassword", newPassword).CombinedOutput()
	outStr := strings.TrimSpace(string(out))
	if err != nil {
		return fmt.Errorf("sysadminctl -resetPasswordFor %s: %w\n%s", name, err, outStr)
	}
	lower := strings.ToLower(outStr)
	if strings.Contains(lower, "error") || strings.Contains(lower, "failed") || strings.Contains(lower, "permission") {
		return fmt.Errorf("sysadminctl %s: %s", name, outStr)
	}
	return nil
}

// RenameUser changes a user's login name via dscl.
// Requires root privileges.
func RenameUser(oldName, newName string) error {
	if oldName == "" || newName == "" {
		return fmt.Errorf("username cannot be empty")
	}
	out, err := exec.Command("dscl", ".", "-change", "/Users/"+oldName, "RecordName", oldName, newName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("dscl rename %s→%s: %w\n%s", oldName, newName, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// DeleteUser removes a macOS user record via dscl.
// Requires root privileges.
func DeleteUser(name string) error {
	out, err := exec.Command("dscl", ".", "-delete", "/Users/"+name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("dscl -delete %s: %w\n%s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// AddSudo adds an existing user to the admin group via dseditgroup.
// Requires root privileges.
func AddSudo(name string) error {
	group, err := sudoGroup()
	if err != nil {
		return err
	}
	out, err := exec.Command("dseditgroup", "-o", "edit", "-a", name, "-t", "user", group).CombinedOutput()
	if err != nil {
		return fmt.Errorf("dseditgroup add %s to %s: %w\n%s", name, group, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// RemoveSudo removes a user from the admin group via dseditgroup.
// Requires root privileges.
func RemoveSudo(name string) error {
	group, err := sudoGroup()
	if err != nil {
		return err
	}
	out, err := exec.Command("dseditgroup", "-o", "edit", "-d", name, "-t", "user", group).CombinedOutput()
	if err != nil {
		return fmt.Errorf("dseditgroup remove %s from %s: %w\n%s", name, group, err, strings.TrimSpace(string(out)))
	}
	return nil
}
