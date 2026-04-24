//go:build darwin

package detect

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DetectOS reads macOS system files and returns the parsed OS info.
func DetectOS() OSInfo {
	info := OSInfo{
		PrettyName: detectMacOSName(),
		ID:         "darwin",
		Pkg:        detectMacOSPkg(),
	}
	if h, err := os.Hostname(); err == nil {
		info.Hostname = h
	}
	info.IsRoot = os.Getuid() == 0
	info.Locale = os.Getenv("LANG")
	return info
}

// DetectLocales returns available locales via locale -a, or a minimal fallback.
func DetectLocales() []string {
	return []string{
		"C.UTF-8",
		"en_US.UTF-8",
		"en_GB.UTF-8",
		"de_DE.UTF-8",
		"es_ES.UTF-8",
		"fr_FR.UTF-8",
		"it_IT.UTF-8",
		"ja_JP.UTF-8",
		"pt_BR.UTF-8",
		"zh_CN.UTF-8",
	}
}

// detectMacOSName parses /System/Library/CoreServices/SystemVersion.plist
// to extract the human-readable macOS version (e.g. "macOS 15.4.1").
// Falls back to "macOS" if the file is unreadable or malformed.
func detectMacOSName() string {
	data, err := os.ReadFile("/System/Library/CoreServices/SystemVersion.plist")
	if err != nil {
		return "macOS"
	}
	content := string(data)

	name := plistValue(content, "ProductName")
	version := plistValue(content, "ProductUserVisibleVersion")
	if version == "" {
		version = plistValue(content, "ProductVersion")
	}

	if name == "" {
		return "macOS"
	}
	if version != "" {
		return name + " " + version
	}
	return name
}

// plistValue extracts the <string> value that immediately follows a <key>KEY</key>
// in a plain XML plist. No plist library needed — the format is predictable.
func plistValue(content, key string) string {
	keyTag := "<key>" + key + "</key>"
	idx := strings.Index(content, keyTag)
	if idx == -1 {
		return ""
	}
	rest := content[idx+len(keyTag):]
	start := strings.Index(rest, "<string>")
	end := strings.Index(rest, "</string>")
	if start == -1 || end == -1 || start >= end {
		return ""
	}
	return rest[start+len("<string>") : end]
}

// DetectTimezones returns a sorted list of available timezones by walking /usr/share/zoneinfo.
// Falls back to a minimal hardcoded list if the directory is unreadable.
func DetectTimezones() []string {
	const root = "/usr/share/zoneinfo"
	var zones []string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(path, root+"/")
		// Skip metadata files (e.g. leap-seconds.list, +VERSION, posix/, right/)
		if strings.HasPrefix(rel, "+") || strings.HasPrefix(rel, "posix/") || strings.HasPrefix(rel, "right/") || !strings.Contains(rel, "/") {
			return nil
		}
		zones = append(zones, rel)
		return nil
	})
	if len(zones) > 0 {
		sort.Strings(zones)
		return zones
	}
	return []string{
		"Africa/Cairo",
		"America/Chicago",
		"America/Los_Angeles",
		"America/New_York",
		"America/Sao_Paulo",
		"Asia/Kolkata",
		"Asia/Shanghai",
		"Asia/Tokyo",
		"Australia/Sydney",
		"Europe/Berlin",
		"Europe/London",
		"Europe/Madrid",
		"Europe/Paris",
		"UTC",
	}
}

// detectMacOSPkg checks for Homebrew or MacPorts at their standard install paths.
// No external commands — checks file existence only.
func detectMacOSPkg() PackageManager {
	// Apple Silicon Homebrew prefix
	if _, err := os.Stat("/opt/homebrew/bin/brew"); err == nil {
		return PkgBrew
	}
	// Intel Homebrew prefix
	if _, err := os.Stat("/usr/local/bin/brew"); err == nil {
		return PkgBrew
	}
	// MacPorts
	if _, err := os.Stat("/opt/local/bin/port"); err == nil {
		return PkgPort
	}
	return PkgUnknown
}
