package apply

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
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

var validKeyTypes = []string{
	"ssh-ed25519", "ssh-rsa",
	"ecdsa-sha2-nistp256", "ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521",
	"sk-ssh-ed25519@openssh.com", "sk-ecdsa-sha2-nistp256@openssh.com",
}

func ValidateSSHKey(pubkey string) error {
	pubkey = strings.TrimSpace(pubkey)
	if pubkey == "" {
		return fmt.Errorf("key is empty")
	}
	fields := strings.Fields(pubkey)
	if len(fields) < 2 {
		return fmt.Errorf("expected: <type> <base64-data> [comment]")
	}
	keyType := fields[0]
	known := false
	for _, t := range validKeyTypes {
		if keyType == t {
			known = true
			break
		}
	}
	if !known {
		return fmt.Errorf("unknown key type %q — expected ssh-ed25519, ssh-rsa, or ecdsa-*", keyType)
	}
	data := fields[1]
	// Pad to multiple of 4 so StdEncoding can decode.
	if rem := len(data) % 4; rem != 0 {
		data += strings.Repeat("=", 4-rem)
	}
	if _, err := base64.StdEncoding.DecodeString(data); err != nil {
		return fmt.Errorf("key data is not valid base64")
	}
	return nil
}

// SSHKeyComment extracts the comment field (3rd token) from a public key line,
// stripping any user@ prefix so only the device name is shown.
func SSHKeyComment(pubkey string) string {
	fields := strings.Fields(strings.TrimSpace(pubkey))
	var comment string
	if len(fields) >= 3 {
		comment = fields[2]
	} else if len(fields) >= 2 && len(fields[1]) > 12 {
		return fields[1][:8] + "…"
	} else {
		return pubkey
	}
	if i := strings.LastIndex(comment, "@"); i >= 0 {
		return comment[i+1:]
	}
	return comment
}

// LoadSSHKeys reads ~username/.ssh/authorized_keys and returns valid pubkey lines.
func LoadSSHKeys(username string) []string {
	u, err := user.Lookup(username)
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(u.HomeDir, ".ssh", "authorized_keys"))
	if err != nil {
		return nil
	}
	var keys []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") && ValidateSSHKey(line) == nil {
			keys = append(keys, line)
		}
	}
	return keys
}

// RemoveSSHKey rewrites authorized_keys without the matching pubkey line.
func RemoveSSHKey(username, pubkey string) error {
	pubkey = strings.TrimSpace(pubkey)
	u, err := user.Lookup(username)
	if err != nil {
		return fmt.Errorf("user %q not found: %w", username, err)
	}
	authKeys := filepath.Join(u.HomeDir, ".ssh", "authorized_keys")
	data, err := os.ReadFile(authKeys)
	if err != nil {
		return fmt.Errorf("read authorized_keys: %w", err)
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)

	var kept []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) != pubkey {
			kept = append(kept, line)
		}
	}
	// Trim trailing blank lines but keep a final newline.
	content := strings.TrimRight(strings.Join(kept, "\n"), "\n")
	if content != "" {
		content += "\n"
	}
	if err := os.WriteFile(authKeys, []byte(content), 0600); err != nil {
		return fmt.Errorf("write authorized_keys: %w", err)
	}
	if err := os.Chown(authKeys, uid, gid); err != nil {
		return fmt.Errorf("chown authorized_keys: %w", err)
	}
	return nil
}

// AddSSHKey appends pubkey to ~username/.ssh/authorized_keys, creating the
// directory and file with correct permissions if needed. Skips if key already present.
func AddSSHKey(username, pubkey string) error {
	pubkey = strings.TrimSpace(pubkey)
	if pubkey == "" {
		return nil
	}
	u, err := user.Lookup(username)
	if err != nil {
		return fmt.Errorf("user %q not found: %w", username, err)
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)

	sshDir := filepath.Join(u.HomeDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return fmt.Errorf("create .ssh dir: %w", err)
	}
	if err := os.Chown(sshDir, uid, gid); err != nil {
		return fmt.Errorf("chown .ssh: %w", err)
	}

	authKeys := filepath.Join(sshDir, "authorized_keys")

	existing, _ := os.ReadFile(authKeys)
	for _, line := range strings.Split(string(existing), "\n") {
		if strings.TrimSpace(line) == pubkey {
			return nil
		}
	}

	f, err := os.OpenFile(authKeys, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("open authorized_keys: %w", err)
	}
	_, werr := fmt.Fprintln(f, pubkey)
	f.Close()
	if werr != nil {
		return fmt.Errorf("write authorized_keys: %w", werr)
	}

	if err := os.Chown(authKeys, uid, gid); err != nil {
		return fmt.Errorf("chown authorized_keys: %w", err)
	}
	return nil
}
