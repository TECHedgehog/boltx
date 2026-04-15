//go:build darwin

package detect

import "os"

func detectVirt() VirtType {
	// VMware Fusion installs its tools at a predictable path
	if _, err := os.Stat("/Library/Application Support/VMware Tools"); err == nil {
		return VirtVMware
	}
	// Parallels Desktop exposes a character device in the guest
	if _, err := os.Stat("/dev/prl_tg"); err == nil {
		return VirtUnknown // Parallels — no dedicated VirtType yet
	}
	// VirtualBox Guest Additions
	if _, err := os.Stat("/Library/Application Support/VirtualBox Guest Additions"); err == nil {
		return VirtVirtualBox
	}
	return VirtNone
}
