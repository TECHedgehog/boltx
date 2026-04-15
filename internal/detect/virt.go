package detect

import "os"

// VirtType describes the detected virtualization platform.
type VirtType int

const (
	VirtNone       VirtType = iota // bare metal
	VirtKVM                        // most VPS providers (DigitalOcean, Linode...)
	VirtVMware                     // VMware Workstation / ESXi / Fusion
	VirtVirtualBox                 // local dev VMs
	VirtQEMU                       // QEMU without KVM
	VirtXen                        // older VPS providers (AWS EC2 classic)
	VirtContainer                  // Docker, LXC
	VirtUnknown                    // hypervisor detected but type unclear
)

func (v VirtType) String() string {
	switch v {
	case VirtKVM:
		return "KVM"
	case VirtVMware:
		return "VMware"
	case VirtVirtualBox:
		return "VirtualBox"
	case VirtQEMU:
		return "QEMU"
	case VirtXen:
		return "Xen"
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
	UseCaseVPS UseCase = iota
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

// Detect reads system state to determine the environment.
// detectVirt() is implemented per-platform in virt_linux.go / virt_darwin.go.
func Detect() Environment {
	return Environment{
		Virt:        detectVirt(),
		ViaSSH:      os.Getenv("SSH_CLIENT") != "" || os.Getenv("SSH_TTY") != "",
		HasPublicIP: hasPublicIP(),
	}
}
