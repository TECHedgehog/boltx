package apply

import (
	"fmt"
	"strconv"
	"strings"
)

// PortRule represents a single firewall rule.
type PortRule struct {
	From     string // e.g. "22" or "25565"
	To       string // range end; empty = single port
	Protocol string // "tcp" / "udp" / "both"
	Existing bool   // true when detected from the live system at startup
}

// String returns a human-readable label, e.g. "22/tcp" or "25565 to 25566/udp".
func (r PortRule) String() string {
	if r.To == "" || r.To == r.From {
		return r.From + "/" + r.Protocol
	}
	return r.From + " to " + r.To + "/" + r.Protocol
}

// UFWSpec returns the ufw-compatible port specification, e.g. "22/tcp" or "25565:25566/udp".
func (r PortRule) UFWSpec() string {
	proto := r.Protocol
	if proto == "both" {
		proto = "any"
	}
	if r.To == "" || r.To == r.From {
		return r.From + "/" + proto
	}
	return r.From + ":" + r.To + "/" + proto
}

// ValidatePort returns an error if s is not a valid port number (1–65535).
func ValidateNetPort(s string) error {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("port must be a number between 1 and 65535")
	}
	return nil
}

// ValidatePortRange returns an error if from/to are not valid port numbers
// and from <= to.
func ValidatePortRange(from, to string) error {
	if err := ValidateNetPort(from); err != nil {
		return fmt.Errorf("start port: %w", err)
	}
	if to == "" {
		return nil
	}
	if err := ValidateNetPort(to); err != nil {
		return fmt.Errorf("end port: %w", err)
	}
	f, _ := strconv.Atoi(strings.TrimSpace(from))
	t, _ := strconv.Atoi(strings.TrimSpace(to))
	if t < f {
		return fmt.Errorf("end port must be >= start port")
	}
	return nil
}

// PresetPortRules returns the default port rules for a named preset.
func PresetPortRules(preset string) []PortRule {
	switch preset {
	case "web":
		return []PortRule{
			{From: "80", Protocol: "tcp"},
			{From: "443", Protocol: "tcp"},
		}
	case "minecraft":
		return []PortRule{
			{From: "25565", Protocol: "tcp"},
			{From: "25565", Protocol: "udp"},
		}
	default:
		return nil
	}
}

// DetectOpenPorts returns the currently allowed UFW rules (Existing=true).
// Returns nil when UFW is inactive or unavailable.
func DetectOpenPorts() []PortRule { return detectOpenPorts() }

// ApplyFirewallRules opens UFW rules for each PortRule.
// UFW must already be enabled (SEC tab). Rules with Protocol="both" are added twice.
func ApplyFirewallRules(rules []PortRule) error {
	return applyFirewallRules(rules)
}

// DeleteFirewallRules removes UFW allow rules for each PortRule.
func DeleteFirewallRules(rules []PortRule) error { return deleteFirewallRules(rules) }

// SetProxyManager records the chosen proxy manager.
// Actual proxy configuration is handled by the RUN tab.
// This is a no-op at apply time — selection is used by syncPkgTab to flag dependencies.
func SetProxyManager(_ string) error { return nil }

// WriteFail2banConfig writes /etc/fail2ban/jail.local for the given preset.
// preset: "ssh" | "web" | "all"
func WriteFail2banConfig(preset string) error {
	return writeFail2banConfig(preset)
}

// EnableFail2ban enables and starts the fail2ban service.
func EnableFail2ban() error { return enableFail2ban() }

// DisableFail2ban disables and stops the fail2ban service.
func DisableFail2ban() error { return disableFail2ban() }

// DetectFail2banActive reports whether fail2ban is currently running.
func DetectFail2banActive() bool { return detectFail2banActive() }
