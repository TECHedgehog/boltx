//go:build !linux && !darwin

package detect

import "os"

// DetectOS returns minimal info on unsupported platforms (hostname only).
func DetectOS() OSInfo {
	info := OSInfo{}
	if h, err := os.Hostname(); err == nil {
		info.Hostname = h
	}
	info.IsRoot = os.Getuid() == 0
	info.Locale = os.Getenv("LANG")
	return info
}

// DetectLocales returns a minimal hardcoded list on unsupported platforms.
func DetectLocales() []string {
	return []string{
		"C.UTF-8",
		"en_US.UTF-8",
		"en_GB.UTF-8",
		"de_DE.UTF-8",
		"es_ES.UTF-8",
		"fr_FR.UTF-8",
		"it_IT.UTF-8",
		"ja_JP.UTF-8",
		"pt_BR.UTF-8",
		"zh_CN.UTF-8",
	}
}
