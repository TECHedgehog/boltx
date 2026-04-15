//go:build linux

package detect

import (
	"os"
	"strings"
)

func detectVirt() VirtType {
	// /sys/class/dmi/id/product_name contains the VM product name on most hypervisors
	if data, err := os.ReadFile("/sys/class/dmi/id/product_name"); err == nil {
		name := strings.ToLower(strings.TrimSpace(string(data)))
		switch {
		case strings.Contains(name, "virtualbox"):
			return VirtVirtualBox
		case strings.Contains(name, "vmware"):
			return VirtVMware
		case strings.Contains(name, "kvm"):
			return VirtKVM
		case strings.Contains(name, "qemu"):
			return VirtQEMU
		case strings.Contains(name, "xen"):
			return VirtXen
		}
	}

	// sys_vendor as fallback — Unraid/KVM VMs typically report "QEMU" here
	if data, err := os.ReadFile("/sys/class/dmi/id/sys_vendor"); err == nil {
		vendor := strings.ToLower(strings.TrimSpace(string(data)))
		switch {
		case strings.Contains(vendor, "vmware"):
			return VirtVMware
		case strings.Contains(vendor, "innotek"), strings.Contains(vendor, "virtualbox"):
			return VirtVirtualBox
		case strings.Contains(vendor, "xen"):
			return VirtXen
		case strings.Contains(vendor, "qemu"):
			return VirtKVM
		}
	}

	// bios_vendor: QEMU VMs use either SeaBIOS or EDK II (OVMF)
	if data, err := os.ReadFile("/sys/class/dmi/id/bios_vendor"); err == nil {
		bios := strings.ToLower(strings.TrimSpace(string(data)))
		if strings.Contains(bios, "seabios") || strings.Contains(bios, "edk ii") {
			return VirtKVM
		}
	}

	// /proc/cpuinfo has a "hypervisor" flag when running inside any VM
	if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		if strings.Contains(string(data), "hypervisor") {
			return VirtUnknown
		}
	}

	// container detection
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return VirtContainer
	}
	if _, err := os.Stat("/run/systemd/container"); err == nil {
		return VirtContainer
	}

	return VirtNone
}
