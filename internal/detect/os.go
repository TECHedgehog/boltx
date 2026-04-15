package detect

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
	PkgBrew                   // macOS Homebrew
	PkgPort                   // macOS MacPorts
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
	case PkgBrew:
		return "homebrew"
	case PkgPort:
		return "macports"
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
	Hostname   string // current system hostname
}
