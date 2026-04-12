package detect

import (
	"os"
	"strings"
)

// VirtType describes the detected virtualization platform.
type VirtType int

const (
	VirtNone       VirtType = iota // bare metal
	VirtKVM                        // most VPS providers (DigitalOcean, Linode...)
	VirtVMware                     // VMware Workstation / ESXi
	VirtVirtualBox                 // local dev VMs
	VirtQEMU                       // QEMU without KVM
	VirtXen                        // older VPS providers (AWS EC2 classic)
	VirtContainer                  // Docker, LXC
	VirtUnknown                    // hypervisor detected but type unclear
)

func (v VirtType) String() string {
	switch v {
	case VirtKVM:
		return "KVM virtual machine"
	case VirtVMware:
		return "VMware virtual machine"
	case VirtVirtualBox:
		return "VirtualBox virtual machine"
	case VirtQEMU:
		return "QEMU virtual machine"
	case VirtXen:
		return "Xen virtual machine"
	case VirtContainer:
		return "Linux container"
	case VirtNone:
		return "bare metal"
	default:
		return "unknown"
	}
}

// UseCase is the intended profile for this machine.
type UseCase int

const (
	UseCaseVPS        UseCase = iota
	UseCaseDevMachine
)

func (u UseCase) String() string {
	switch u {
	case UseCaseVPS:
		return "VPS / Remote server"
	default:
		return "Dev machine / Test VM"
	}
}

// Environment holds the detected system info.
type Environment struct {
	Virt        VirtType
	ViaSSH      bool
	HasPublicIP bool
}

// SuggestedUseCase returns the recommended use case based on detection.
// SSH + public IP → internet-facing VPS. Everything else → local machine.
func (e Environment) SuggestedUseCase() UseCase {
	if e.ViaSSH && e.HasPublicIP {
		return UseCaseVPS
	}
	return UseCaseDevMachine
}

// Detect reads system files to determine the environment.
// No external commands — reads /sys and /proc directly.
func Detect() Environment {
	return Environment{
		Virt:        detectVirt(),
		ViaSSH:      os.Getenv("SSH_CLIENT") != "" || os.Getenv("SSH_TTY") != "",
		HasPublicIP: hasPublicIP(),
	}
}

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
