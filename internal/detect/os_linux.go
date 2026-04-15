//go:build linux

package detect

import (
	"bufio"
	"os"
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
	return info
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
