//go:build darwin

package apply

import (
	"errors"

	"boltx/internal/detect"
)

func installPackage(_ detect.PackageManager, _ string) error {
	return errors.New("package install not supported on macOS")
}

func detectPackageInstalled(_ detect.PackageManager, _ string) bool { return false }
