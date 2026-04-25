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


// Timezone sets the system timezone.
// Tries timedatectl set-timezone first (systemd); falls back to symlinking
// /etc/localtime and writing /etc/timezone. The symlink fallback also works on macOS.
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

