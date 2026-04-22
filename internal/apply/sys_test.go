package apply

import (
	"strings"
	"testing"
)

// --- validateHostname ---

func TestValidateHostname_Valid(t *testing.T) {
	cases := []string{
		"myhost",
		"my-host",
		"my-host-123",
		"a",
		"A1",
		"foo.bar.baz",        // FQDN with dots
		"xn--nxasmq6b",      // punycode-style
		strings.Repeat("a", 63),                        // max label length
		strings.Repeat("a", 63) + "." + strings.Repeat("b", 63), // two max labels
	}
	for _, name := range cases {
		if err := validateHostname(name); err != nil {
			t.Errorf("validateHostname(%q) unexpected error: %v", name, err)
		}
	}
}

func TestValidateHostname_Invalid(t *testing.T) {
	cases := []struct {
		name   string
		substr string // expected fragment in error message
	}{
		{"foo bar", "invalid characters"},
		{"-start", "invalid characters"},
		{"end-", "invalid characters"},
		{"foo..bar", "empty label"},
		{".leading", "empty label"},
		{"trailing.", "empty label"},
		{"foo!bar", "invalid characters"},
		{"füü", "invalid characters"},
		{strings.Repeat("a", 64), "too long"},        // label > 63
		{strings.Repeat("a", 254), "too long"},       // total > 253
	}
	for _, tc := range cases {
		err := validateHostname(tc.name)
		if err == nil {
			t.Errorf("validateHostname(%q) expected error, got nil", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.substr) {
			t.Errorf("validateHostname(%q) error %q does not contain %q", tc.name, err.Error(), tc.substr)
		}
	}
}

// --- validateLocale ---

func TestValidateLocale_Valid(t *testing.T) {
	cases := []string{
		"en_US.UTF-8",
		"fr_FR",
		"de_DE.UTF-8",
		"pt_BR.UTF-8",
		"C",
		"POSIX",
		"en",
		"de@euro",
		"zh_CN.GB2312",
	}
	for _, loc := range cases {
		if err := validateLocale(loc); err != nil {
			t.Errorf("validateLocale(%q) unexpected error: %v", loc, err)
		}
	}
}

func TestValidateLocale_Invalid(t *testing.T) {
	cases := []struct {
		locale string
		substr string
	}{
		{"LANG=en_US.UTF-8", "must not include LANG="},
		{"en US.UTF-8", "invalid locale format"},  // embedded space
		{"123_US", "invalid locale format"},        // starts with digit
		{"en_us.UTF-8", "invalid locale format"},   // lowercase territory
		{"toolong_toolong_toolong", "invalid locale format"},
	}
	for _, tc := range cases {
		err := validateLocale(tc.locale)
		if err == nil {
			t.Errorf("validateLocale(%q) expected error, got nil", tc.locale)
			continue
		}
		if !strings.Contains(err.Error(), tc.substr) {
			t.Errorf("validateLocale(%q) error %q does not contain %q", tc.locale, err.Error(), tc.substr)
		}
	}
}

// --- validateTimezone ---

func TestValidateTimezone_Valid(t *testing.T) {
	cases := []string{
		"America/New_York",
		"Europe/Madrid",
		"UTC",
		"Asia/Kolkata",
		"Pacific/Auckland",
	}
	for _, zone := range cases {
		if err := validateTimezone(zone); err != nil {
			t.Errorf("validateTimezone(%q) unexpected error: %v", zone, err)
		}
	}
}

func TestValidateTimezone_Invalid(t *testing.T) {
	cases := []struct {
		zone   string
		substr string
	}{
		{"../../../etc/passwd", "path traversal"},
		{"Europe/../../../etc/shadow", "path traversal"},
		{"America/New\nYork", "whitespace"},
		{"UTC\t", "whitespace"},
		{"foo bar", "whitespace"},
	}
	for _, tc := range cases {
		err := validateTimezone(tc.zone)
		if err == nil {
			t.Errorf("validateTimezone(%q) expected error, got nil", tc.zone)
			continue
		}
		if !strings.Contains(err.Error(), tc.substr) {
			t.Errorf("validateTimezone(%q) error %q does not contain %q", tc.zone, err.Error(), tc.substr)
		}
	}
}

// --- apply function smoke tests (validation layer only) ---
// These run without root. Valid inputs will fail past validation (no system
// access), but invalid inputs must be caught before any system call.

func TestHostname_RejectsInvalidBeforeSystemCall(t *testing.T) {
	err := Hostname("foo bar")
	if err == nil {
		t.Fatal("Hostname(\"foo bar\") expected error")
	}
	if !strings.Contains(err.Error(), "invalid characters") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLocale_RejectsLANGPrefix(t *testing.T) {
	err := Locale("LANG=en_US.UTF-8")
	if err == nil {
		t.Fatal("Locale(\"LANG=en_US.UTF-8\") expected error")
	}
	if !strings.Contains(err.Error(), "must not include LANG=") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTimezone_RejectsPathTraversal(t *testing.T) {
	err := Timezone("../../../etc/passwd")
	if err == nil {
		t.Fatal("Timezone(\"../../../etc/passwd\") expected error")
	}
	if !strings.Contains(err.Error(), "path traversal") {
		t.Fatalf("unexpected error: %v", err)
	}
}
