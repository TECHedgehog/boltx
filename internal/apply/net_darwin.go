package apply

import "errors"

func detectOpenPorts() []PortRule    { return nil }
func deleteFirewallRules(_ []PortRule) error { return nil }

func applyFirewallRules(_ []PortRule) error {
	return errors.New("UFW firewall rules not supported on macOS")
}

func writeFail2banConfig(_ string) error {
	return errors.New("fail2ban not supported on macOS")
}

func enableFail2ban() error  { return errors.New("fail2ban not supported on macOS") }
func disableFail2ban() error { return errors.New("fail2ban not supported on macOS") }
func detectFail2banActive() bool { return false }
