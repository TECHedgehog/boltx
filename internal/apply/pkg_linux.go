//go:build !darwin

package apply

import (
	"fmt"
	"os/exec"
	"strings"

	"boltx/internal/detect"
)

// pkgNames maps label → PackageManager → package name.
// Empty string means "same as label" (most packages share the same name across distros).
var pkgNames = map[string]map[detect.PackageManager]string{
	"git":      {},
	"curl":     {},
	"wget":     {},
	"htop":     {},
	"vim":      {},
	"tmux":     {},
	"unzip":    {},
	"rsync":    {},
	"jq":       {},
	"nginx":    {},
	"fail2ban": {},
	"ufw":      {},
	"docker": {
		detect.PkgApt:    "docker.io",
		detect.PkgDnf:    "docker",
		detect.PkgPacman: "docker",
		detect.PkgApk:    "docker",
		detect.PkgZypper: "docker",
	},
}

func resolvedName(pm detect.PackageManager, label string) (string, error) {
	pmMap, ok := pkgNames[label]
	if !ok {
		return "", fmt.Errorf("unknown package label %q", label)
	}
	if name, found := pmMap[pm]; found && name != "" {
		return name, nil
	}
	return label, nil
}

func installPackage(pm detect.PackageManager, label string) error {
	name, err := resolvedName(pm, label)
	if err != nil {
		return err
	}
	var cmd *exec.Cmd
	switch pm {
	case detect.PkgApt:
		cmd = exec.Command("apt-get", "install", "-y", name)
	case detect.PkgDnf:
		cmd = exec.Command("dnf", "install", "-y", name)
	case detect.PkgPacman:
		cmd = exec.Command("pacman", "-S", "--noconfirm", name)
	case detect.PkgApk:
		cmd = exec.Command("apk", "add", name)
	case detect.PkgZypper:
		cmd = exec.Command("zypper", "install", "-y", name)
	default:
		return fmt.Errorf("unsupported package manager: %s", pm)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("install %s: %w\n%s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func detectPackageInstalled(pm detect.PackageManager, label string) bool {
	name, err := resolvedName(pm, label)
	if err != nil {
		return false
	}
	var cmd *exec.Cmd
	switch pm {
	case detect.PkgApt:
		cmd = exec.Command("dpkg-query", "-W", "-f=${Status}", name)
		out, err := cmd.Output()
		if err != nil {
			return false
		}
		return strings.Contains(string(out), "install ok installed")
	case detect.PkgDnf, detect.PkgZypper:
		cmd = exec.Command("rpm", "-q", name)
	case detect.PkgPacman:
		cmd = exec.Command("pacman", "-Q", name)
	case detect.PkgApk:
		cmd = exec.Command("apk", "info", "-e", name)
	default:
		return false
	}
	return cmd.Run() == nil
}
