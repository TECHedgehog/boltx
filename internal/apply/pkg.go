package apply

import "boltx/internal/detect"

// InstallPackage installs the package identified by label using pm.
func InstallPackage(pm detect.PackageManager, label string) error {
	return installPackage(pm, label)
}

// DetectPackageInstalled reports whether the package identified by label is installed.
func DetectPackageInstalled(pm detect.PackageManager, label string) bool {
	return detectPackageInstalled(pm, label)
}
