package detect

import (
	"bufio"
	"os"
	"strings"
)

// PackageManager identifies the system's primary package manager.
type PackageManager int

const (
	PkgUnknown PackageManager = iota
	PkgApt                    // Debian, Ubuntu, Mint...
	PkgDnf                    // Fedora, RHEL, Rocky, Alma...
	PkgPacman                 // Arch, Manjaro, EndeavourOS...
	PkgApk                    // Alpine
	PkgZypper                 // openSUSE
	PkgEmerge                 // Gentoo
)

func (p PackageManager) String() string {
	switch p {
	case PkgApt:
		return "apt"
	case PkgDnf:
		return "dnf"
	case PkgPacman:
		return "pacman"
	case PkgApk:
		return "apk"
	case PkgZypper:
		return "zypper"
	case PkgEmerge:
		return "emerge"
	default:
		return "unknown"
	}
}

// OSInfo holds the detected OS details.
type OSInfo struct {
	PrettyName string
	ID         string // e.g. "ubuntu", "fedora", "arch"
	IDLike     string // e.g. "debian" — parent family declared by the distro
	Pkg        PackageManager
}

// DetectOS reads /etc/os-release and returns the parsed OS info.
func DetectOS() OSInfo {
	fields := parseOSRelease()

	info := OSInfo{
		PrettyName: fields["PRETTY_NAME"],
		ID:         fields["ID"],
		IDLike:     fields["ID_LIKE"],
	}
	info.Pkg = resolvePackageManager(info.ID, info.IDLike)
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
		// skip comments and blank lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		// strip surrounding quotes if present
		value = strings.Trim(value, `"'`)
		result[key] = value
	}
	return result
}

// resolvePackageManager maps a distro ID (and its ID_LIKE fallback) to
// the package manager we should use for that system.
func resolvePackageManager(id, idLike string) PackageManager {
	// Check ID first, then fall back to ID_LIKE so that derivatives
	// (e.g. Linux Mint: id=linuxmint id_like=ubuntu debian) are handled.
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
