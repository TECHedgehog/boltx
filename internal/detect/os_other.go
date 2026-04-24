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

// DetectTimezones returns a minimal hardcoded list on unsupported platforms.
func DetectTimezones() []string {
	return []string{
		"Africa/Cairo",
		"America/Chicago",
		"America/Los_Angeles",
		"America/New_York",
		"America/Sao_Paulo",
		"Asia/Kolkata",
		"Asia/Shanghai",
		"Asia/Tokyo",
		"Australia/Sydney",
		"Europe/Berlin",
		"Europe/London",
		"Europe/Madrid",
		"Europe/Paris",
		"UTC",
	}
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
