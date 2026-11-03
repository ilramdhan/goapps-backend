package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func clearSeedEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"SEED_ADMIN_USERNAME", "SEED_ADMIN_EMAIL", "SEED_ADMIN_PASSWORD", "SEED_ADMIN_PASSWORD_FILE"} {
		t.Setenv(k, "")
	}
}

func TestGetAdminCredentials_DevDefault(t *testing.T) {
	clearSeedEnv(t)
	c, err := getAdminCredentials("development")
	if err != nil || c.Password != "admin123" {
		t.Fatalf("got %+v err=%v", c, err)
	}
}

func TestGetAdminCredentials_EnvSet(t *testing.T) {
	clearSeedEnv(t)
	t.Setenv("SEED_ADMIN_PASSWORD", "s3cret")
	c, err := getAdminCredentials("production")
	if err != nil || c.Password != "s3cret" {
		t.Fatalf("got %+v err=%v", c, err)
	}
}

func TestGetAdminCredentials_GeneratedWrittenToFileNotPrinted(t *testing.T) {
	clearSeedEnv(t)
	path := filepath.Join(t.TempDir(), "pw.txt")
	t.Setenv("SEED_ADMIN_PASSWORD_FILE", path)

	oldOut := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	c, err := getAdminCredentials("production")
	_ = w.Close()
	os.Stdout = oldOut
	out, _ := io.ReadAll(r)

	if err != nil {
		t.Fatal(err)
	}
	if len(c.Password) != 24 {
		t.Fatalf("expected 24-char password, got %d", len(c.Password))
	}
	if strings.Contains(string(out), c.Password) {
		t.Fatal("password leaked to stdout")
	}
	data, err := os.ReadFile(path)
	if err != nil || strings.TrimSpace(string(data)) != c.Password {
		t.Fatalf("file content mismatch: %q err=%v", data, err)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Fatalf("perm = %v, want 0600", fi.Mode().Perm())
	}
}

func TestGetAdminCredentials_WriteFailureReturnsError(t *testing.T) {
	clearSeedEnv(t)
	t.Setenv("SEED_ADMIN_PASSWORD_FILE", filepath.Join(t.TempDir(), "no-dir", "pw.txt"))
	if _, err := getAdminCredentials("staging"); err == nil {
		t.Fatal("expected error")
	}
}
