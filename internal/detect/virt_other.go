//go:build !linux && !darwin

package detect

func detectVirt() VirtType {
	return VirtNone
}
