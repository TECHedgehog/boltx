//go:build !linux && !darwin

package detect

func detectVirt() VirtType { return VirtNone }

func viaSSHProc() bool { return false }
