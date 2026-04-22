package apply

import (
	"strings"
	"testing"
)

// --- validateUsername ---

func TestValidateUsername_Valid(t *testing.T) {
	cases := []string{
		"alice",
		"bob123",
		"_svc",
		"a",
		"x-daemon",
		"user_name",
		strings.Repeat("a", 32), // max length
	}
	for _, name := range cases {
		if err := validateUsername(name); err != nil {
			t.Errorf("validateUsername(%q) unexpected error: %v", name, err)
		}
	}
}

func TestValidateUsername_Invalid(t *testing.T) {
	cases := []struct {
		name   string
		substr string
	}{
		{"Alice", "invalid username"},          // uppercase
		{"0user", "invalid username"},          // starts with digit
		{"-user", "invalid username"},          // starts with hyphen
		{"user name", "invalid username"},      // space
		{"user@host", "invalid username"},      // @ not allowed
		{"über", "invalid username"},           // non-ASCII
		{strings.Repeat("a", 33), "too long"}, // 33 chars
	}
	for _, tc := range cases {
		err := validateUsername(tc.name)
		if err == nil {
			t.Errorf("validateUsername(%q) expected error, got nil", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.substr) {
			t.Errorf("validateUsername(%q) error %q does not contain %q", tc.name, err.Error(), tc.substr)
		}
	}
}

// --- apply function smoke tests (validation layer only) ---
// These run without root. Invalid inputs must be caught before any system call.

func TestCreateUser_RejectsEmptyUsername(t *testing.T) {
	err := CreateUser("", "secret")
	if err == nil {
		t.Fatal("CreateUser(\"\", ...) expected error")
	}
	if !strings.Contains(err.Error(), "cannot be empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateUser_RejectsInvalidBeforeSystemCall(t *testing.T) {
	err := CreateUser("UPPERCASE", "secret")
	if err == nil {
		t.Fatal("CreateUser(\"UPPERCASE\", ...) expected error")
	}
	if !strings.Contains(err.Error(), "invalid username") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateUser_RejectsEmptyPassword(t *testing.T) {
	err := CreateUser("alice", "")
	if err == nil {
		t.Fatal("CreateUser(\"alice\", \"\") expected error")
	}
	if !strings.Contains(err.Error(), "password cannot be empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}
