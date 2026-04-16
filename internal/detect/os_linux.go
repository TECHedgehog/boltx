//go:build linux

package detect

import (
	"bufio"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// DetectOS reads /etc/os-release and returns the parsed OS info.
func DetectOS() OSInfo {
	fields := parseOSRelease()

	info := OSInfo{
		PrettyName: fields["PRETTY_NAME"],
		ID:         fields["ID"],
		IDLike:     fields["ID_LIKE"],
	}
	info.Pkg = resolvePackageManager(info.ID, info.IDLike)
	if h, err := os.Hostname(); err == nil {
		info.Hostname = h
	}
	info.IsRoot = os.Getuid() == 0
	info.Locale = detectLocale()
	return info
}

// detectLocale returns the current locale string.
// Reads LANG env first; falls back to parsing /etc/default/locale.
func detectLocale() string {
	if lang := os.Getenv("LANG"); lang != "" {
		return lang
	}
	f, err := os.Open("/etc/default/locale")
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "LANG=") {
			return strings.Trim(strings.TrimPrefix(line, "LANG="), `"'`)
		}
	}
	return ""
}

// DetectLocales returns a sorted list of available locales on the system.
// Tries localectl list-locales first, then locale -a, then returns a minimal
// hardcoded list if both fail or return nothing.
func DetectLocales() []string {
	if out, err := exec.Command("localectl", "list-locales").Output(); err == nil {
		if items := parseLines(string(out)); len(items) > 0 {
			return items
		}
	}
	if out, err := exec.Command("locale", "-a").Output(); err == nil {
		if items := parseLines(string(out)); len(items) > 0 {
			return items
		}
	}
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

// parseLines splits output into trimmed, non-empty lines, deduped and sorted.
func parseLines(out string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || seen[line] {
			continue
		}
		seen[line] = true
		result = append(result, line)
	}
	sort.Strings(result)
	return result
}

// parseOSRelease reads /etc/os-release and returns key=value pairs.
// Values may be quoted ("ubuntu") or bare (ubuntu) — both are handled.
func parseOSRelease() map[string]string {
	result := make(map[string]string)

	f, err := os.Open("/etc/os-release")
	if err != nil {
		return result
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		value = strings.Trim(value, `"'`)
		result[key] = value
	}
	return result
}

// resolvePackageManager maps a distro ID (and its ID_LIKE fallback) to
// the package manager we should use for that system.
func resolvePackageManager(id, idLike string) PackageManager {
	for _, candidate := range []string{id, idLike} {
		for _, part := range strings.Fields(candidate) {
			switch strings.ToLower(part) {
			case "ubuntu", "debian", "raspbian", "linuxmint", "pop", "elementary", "kali":
				return PkgApt
			case "fedora", "rhel", "centos", "rocky", "almalinux", "ol":
				return PkgDnf
			case "arch", "manjaro", "endeavouros", "garuda", "cachyos":
				return PkgPacman
			case "alpine":
				return PkgApk
			case "opensuse", "opensuse-leap", "opensuse-tumbleweed", "sles":
				return PkgZypper
			case "gentoo":
				return PkgEmerge
			}
		}
	}
	return PkgUnknown
}
