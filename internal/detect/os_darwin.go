//go:build darwin

package detect

import (
	"os"
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
	return info
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
