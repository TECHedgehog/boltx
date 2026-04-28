//go:build linux

package detect

import (
	"fmt"
	"os"
	"strconv"
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

// viaSSHProc walks the parent process chain looking for an sshd ancestor.
// Handles VSCode Remote SSH, which doesn't propagate SSH_CLIENT/SSH_TTY.
func viaSSHProc() bool {
	pid := os.Getpid()
	for range 32 { // cap depth to avoid infinite loops
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
		if err != nil {
			return false
		}
		var ppid int
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "PPid:") {
				ppid, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "PPid:")))
				break
			}
		}
		if ppid <= 1 {
			return false
		}
		comm, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", ppid))
		if err != nil {
			return false
		}
		if strings.TrimSpace(string(comm)) == "sshd" {
			return true
		}
		pid = ppid
	}
	return false
}
