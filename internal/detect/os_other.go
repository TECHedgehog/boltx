//go:build !linux && !darwin

package detect

import "os"

// DetectOS returns minimal info on unsupported platforms (hostname only).
func DetectOS() OSInfo {
	info := OSInfo{}
	if h, err := os.Hostname(); err == nil {
		info.Hostname = h
	}
	return info
}
