package apply

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const (
	sshdConfigPath  = "/etc/ssh/sshd_config"
	sshdDropInDir   = "/etc/ssh/sshd_config.d"
	boltxConfPath   = "/etc/ssh/sshd_config.d/00-boltx.conf"
	sshdIncludeLine = "Include /etc/ssh/sshd_config.d/*.conf"
)

// opensshSupportsInclude returns true when the installed sshd is OpenSSH ≥ 7.3,
// which introduced the Include directive (2016).
func opensshSupportsInclude() bool {
	out, _ := exec.Command("sshd", "-V").CombinedOutput()
	s := string(out)
	idx := strings.Index(s, "OpenSSH_")
	if idx < 0 {
		return false
	}
	ver := s[idx+len("OpenSSH_"):]
	var major, minor int
	if n, _ := fmt.Sscanf(ver, "%d.%d", &major, &minor); n < 2 {
		return false
	}
	return major > 7 || (major == 7 && minor >= 3)
}

// ensureSSHDDropIn creates the sshd_config.d/ directory if absent, then
// inserts "Include /etc/ssh/sshd_config.d/*.conf" after the leading comment
// block in sshd_config so that 99-boltx.conf takes precedence over all
// defaults (OpenSSH: first match wins).
func ensureSSHDDropIn() error {
	if os.Getuid() != 0 {
		return fmt.Errorf("must run as root to modify sshd config")
	}
	if err := os.MkdirAll(sshdDropInDir, 0755); err != nil {
		return fmt.Errorf("create %s: %w", sshdDropInDir, err)
	}

	// Migrate legacy 99-boltx.conf if present from before the rename.
	legacy := sshdDropInDir + "/99-boltx.conf"
	if _, err := os.Stat(legacy); err == nil {
		_ = os.Rename(legacy, boltxConfPath)
	}

	data, err := os.ReadFile(sshdConfigPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", sshdConfigPath, err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == sshdIncludeLine {
			return nil // already present
		}
	}

	// Insert after the leading comment block (first non-comment, non-blank line).
	lines := strings.Split(string(data), "\n")
	insertAt := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || trimmed == "" {
			insertAt = i + 1
		} else {
			break
		}
	}

	newLines := make([]string, 0, len(lines)+2)
	newLines = append(newLines, lines[:insertAt]...)
	newLines = append(newLines, sshdIncludeLine, "")
	newLines = append(newLines, lines[insertAt:]...)

	content := strings.Join(newLines, "\n")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return os.WriteFile(sshdConfigPath, []byte(content), 0644)
}

// editBoltxConf sets key=value in the appropriate sshd config location.
// OpenSSH ≥ 7.3: uses the drop-in at 00-boltx.conf (takes precedence via Include).
// OpenSSH < 7.3: edits sshd_config directly.
func editBoltxConf(key, value string) error {
	if os.Getuid() != 0 {
		return fmt.Errorf("must run as root to modify sshd config")
	}
	if opensshSupportsInclude() {
		if err := ensureSSHDDropIn(); err != nil {
			return err
		}
		return editSSHDConfigAt(boltxConfPath, key, value)
	}
	return editSSHDConfigAt(sshdConfigPath, key, value)
}

// ValidatePort returns an error if s is not a valid TCP port number (1–65535).
func ValidatePort(s string) error {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("port must be a number between 1 and 65535")
	}
	return nil
}

// ApplySSHPort writes Port to sshd config, updates the firewall if active, then restarts sshd.
func ApplySSHPort(port string) error {
	oldPort, _ := DetectSSHDConfig("Port")
	if oldPort == "" {
		oldPort = "22"
	}
	if err := editBoltxConf("Port", port); err != nil {
		return err
	}
	if err := updateFirewallForPort(oldPort, port); err != nil {
		return fmt.Errorf("port set; firewall update failed: %w", err)
	}
	return restartSSHD()
}

// editSSHDConfigAt sets key to value in the given sshd_config-format file.
// Handles: key already set, key commented out, key absent (appends).
func editSSHDConfigAt(path, key, value string) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", path, err)
	}

	target := key + " " + value
	found := false
	var out []string

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimLeft(line, "# \t")
		if strings.HasPrefix(trimmed, key+" ") || trimmed == key {
			if !found {
				out = append(out, target)
				found = true
			}
			continue
		}
		out = append(out, line)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan %s: %w", path, err)
	}

	if !found {
		out = append(out, target)
	}

	content := strings.Join(out, "\n")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// DetectSSHDConfig returns the effective value for key.
// Checks 99-boltx.conf first (it takes precedence when Include is at the top),
// then falls back to the base sshd_config.
func DetectSSHDConfig(key string) (string, error) {
	if v, err := detectSSHDConfigAt(boltxConfPath, key); err == nil && v != "" {
		return v, nil
	}
	return detectSSHDConfigAt(sshdConfigPath, key)
}

func detectSSHDConfigAt(path, key string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read %s: %w", path, err)
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 && strings.EqualFold(parts[0], key) {
			return parts[1], nil
		}
	}
	return "", scanner.Err()
}

// FirewallActive reports whether the system firewall is currently active.
func FirewallActive() bool { return firewallActive() }

// ApplySSHOption writes key=value to 00-boltx.conf and restarts sshd.
// Used by all SSH KindCycle and KindTextInput options.
func ApplySSHOption(key, value string) error {
	if err := editBoltxConf(key, value); err != nil {
		return err
	}
	return restartSSHD()
}

