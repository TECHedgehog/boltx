package apply

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// hostnameLabel matches a single RFC 1123 label: starts and ends with
// alphanumeric, may contain hyphens in the middle, 1–63 chars.
var hostnameLabel = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$`)

// localeRe matches the standard locale identifier formats:
//
//	language[_territory][.codeset][@modifier]   e.g. en_US.UTF-8, fr_FR, de@euro
var localeRe = regexp.MustCompile(`^[a-zA-Z]{2,3}(_[A-Z]{2,3})?(\.[A-Za-z0-9-]+)?(@[A-Za-z0-9]+)?$`)

func validateHostname(name string) error {
	if len(name) > 253 {
		return fmt.Errorf("hostname too long (%d chars, max 253)", len(name))
	}
	for _, label := range strings.Split(name, ".") {
		if label == "" {
			return fmt.Errorf("hostname has empty label (leading/trailing dot or consecutive dots)")
		}
		if len(label) > 63 {
			return fmt.Errorf("hostname label %q too long (%d chars, max 63)", label, len(label))
		}
		if !hostnameLabel.MatchString(label) {
			return fmt.Errorf("hostname label %q contains invalid characters (only a-z, A-Z, 0-9, hyphen allowed; no leading/trailing hyphen)", label)
		}
	}
	return nil
}

func validateLocale(locale string) error {
	if strings.HasPrefix(locale, "LANG=") {
		return fmt.Errorf("locale must not include LANG= prefix (got %q)", locale)
	}
	if locale == "C" || locale == "POSIX" {
		return nil
	}
	if !localeRe.MatchString(locale) {
		return fmt.Errorf("invalid locale format %q (expected e.g. en_US.UTF-8)", locale)
	}
	return nil
}

func validateTimezone(zone string) error {
	if strings.ContainsAny(zone, " \t\n\r") {
		return fmt.Errorf("timezone must not contain whitespace")
	}
	// Guard the fallback path against path traversal.
	zoneFile := "/usr/share/zoneinfo/" + zone
	if clean := filepath.Clean(zoneFile); !strings.HasPrefix(clean, "/usr/share/zoneinfo/") {
		return fmt.Errorf("invalid timezone %q (path traversal detected)", zone)
	}
	return nil
}

// Locale sets the system locale (LANG variable).
// Tries localectl set-locale first (systemd); falls back to writing /etc/default/locale.
// Requires root privileges.
func Locale(locale string) error {
	locale = strings.TrimSpace(locale)
	if locale == "" {
		return fmt.Errorf("locale cannot be empty")
	}
	if err := validateLocale(locale); err != nil {
		return err
	}

	if _, err := exec.LookPath("localectl"); err == nil {
		out, err := exec.Command("localectl", "set-locale", "LANG="+locale).CombinedOutput()
		if err != nil {
			return fmt.Errorf("localectl set-locale: %w\n%s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}

	content := "LANG=" + locale + "\n"
	if err := os.WriteFile("/etc/default/locale", []byte(content), 0644); err != nil {
		return fmt.Errorf("write /etc/default/locale: %w", err)
	}
	return nil
}

// Timezone sets the system timezone.
// Tries timedatectl set-timezone first (systemd); falls back to symlinking
// /etc/localtime and writing /etc/timezone.
// Requires root privileges.
func Timezone(zone string) error {
	zone = strings.TrimSpace(zone)
	if zone == "" {
		return fmt.Errorf("timezone cannot be empty")
	}
	if err := validateTimezone(zone); err != nil {
		return err
	}

	if _, err := exec.LookPath("timedatectl"); err == nil {
		out, err := exec.Command("timedatectl", "set-timezone", zone).CombinedOutput()
		if err != nil {
			return fmt.Errorf("timedatectl set-timezone: %w\n%s", err, strings.TrimSpace(string(out)))
		}
		// timedatectl updates /etc/localtime symlink but not /etc/timezone on all distros.
		_ = os.WriteFile("/etc/timezone", []byte(zone+"\n"), 0644)
		return nil
	}

	zoneFile := "/usr/share/zoneinfo/" + zone
	if _, err := os.Stat(zoneFile); err != nil {
		return fmt.Errorf("timezone file not found: %s", zoneFile)
	}
	if err := os.Remove("/etc/localtime"); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove /etc/localtime: %w", err)
	}
	if err := os.Symlink(zoneFile, "/etc/localtime"); err != nil {
		return fmt.Errorf("symlink /etc/localtime: %w", err)
	}
	if err := os.WriteFile("/etc/timezone", []byte(zone+"\n"), 0644); err != nil {
		return fmt.Errorf("write /etc/timezone: %w", err)
	}
	return nil
}

// Hostname sets the system hostname.
// Tries hostnamectl first (systemd systems); falls back to writing /etc/hostname
// and running hostname(1) directly (Alpine, minimal containers).
// Requires root privileges — returns a descriptive error if the call fails.
func Hostname(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("hostname cannot be empty")
	}
	if err := validateHostname(name); err != nil {
		return err
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
