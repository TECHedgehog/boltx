package apply

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEditSSHDConfig_KeyPresent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sshd_config")
	os.WriteFile(p, []byte("PermitRootLogin yes\nPort 22\n"), 0644)

	if err := editSSHDConfigAt(p, "PermitRootLogin", "no"); err != nil {
		t.Fatal(err)
	}
	got, _ := detectSSHDConfigAt(p, "PermitRootLogin")
	if got != "no" {
		t.Errorf("want no, got %q", got)
	}
}

func TestEditSSHDConfig_KeyCommented(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sshd_config")
	os.WriteFile(p, []byte("#PermitRootLogin yes\nPort 22\n"), 0644)

	if err := editSSHDConfigAt(p, "PermitRootLogin", "no"); err != nil {
		t.Fatal(err)
	}
	got, _ := detectSSHDConfigAt(p, "PermitRootLogin")
	if got != "no" {
		t.Errorf("want no, got %q", got)
	}
}

func TestEditSSHDConfig_KeyAbsent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sshd_config")
	os.WriteFile(p, []byte("Port 22\n"), 0644)

	if err := editSSHDConfigAt(p, "PermitRootLogin", "no"); err != nil {
		t.Fatal(err)
	}
	got, _ := detectSSHDConfigAt(p, "PermitRootLogin")
	if got != "no" {
		t.Errorf("want no, got %q", got)
	}
}

func TestDetectSSHDConfig_Absent(t *testing.T) {
	got, err := detectSSHDConfigAt("/nonexistent/path/sshd_config", "PermitRootLogin")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("want empty, got %q", got)
	}
}
