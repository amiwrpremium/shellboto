package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSecret_Env(t *testing.T) {
	t.Setenv("TEST_SECRET_VAR", "from-env")
	t.Setenv("CREDENTIALS_DIRECTORY", "")

	got, err := ResolveSecret("TEST_SECRET_VAR", "ignored")
	if err != nil {
		t.Fatal(err)
	}
	if got != "from-env" {
		t.Fatalf("got %q, want %q", got, "from-env")
	}
}

func TestResolveSecret_EnvTrimsWhitespace(t *testing.T) {
	t.Setenv("TEST_SECRET_VAR", "  padded\n")

	got, _ := ResolveSecret("TEST_SECRET_VAR", "ignored")
	if got != "padded" {
		t.Fatalf("got %q, want %q", got, "padded")
	}
}

func TestResolveSecret_CredsFileFallback(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mycred"), []byte("from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_SECRET_VAR", "")
	t.Setenv("CREDENTIALS_DIRECTORY", dir)

	got, src, err := ResolveSecretWithSource("TEST_SECRET_VAR", "mycred")
	if err != nil {
		t.Fatal(err)
	}
	if got != "from-file" {
		t.Fatalf("got %q, want %q", got, "from-file")
	}
	if src != SecretSourceCreds {
		t.Fatalf("source=%v, want SecretSourceCreds", src)
	}
}

func TestResolveSecret_EnvBeatsFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mycred"), []byte("from-file"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_SECRET_VAR", "from-env")
	t.Setenv("CREDENTIALS_DIRECTORY", dir)

	got, src, _ := ResolveSecretWithSource("TEST_SECRET_VAR", "mycred")
	if got != "from-env" {
		t.Fatalf("got %q, want %q", got, "from-env")
	}
	if src != SecretSourceEnv {
		t.Fatalf("source=%v, want SecretSourceEnv", src)
	}
}

func TestResolveSecret_BothUnset(t *testing.T) {
	t.Setenv("TEST_SECRET_VAR", "")
	t.Setenv("CREDENTIALS_DIRECTORY", "")

	got, src, err := ResolveSecretWithSource("TEST_SECRET_VAR", "mycred")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
	if src != SecretSourceNone {
		t.Fatalf("source=%v, want SecretSourceNone", src)
	}
}

func TestResolveSecret_CredsDirSetFileMissing(t *testing.T) {
	// CREDENTIALS_DIRECTORY is set but the named file isn't there — should
	// be treated as "not found" (not an error), so the caller's "was it
	// required?" logic can decide.
	dir := t.TempDir()
	t.Setenv("TEST_SECRET_VAR", "")
	t.Setenv("CREDENTIALS_DIRECTORY", dir)

	got, src, err := ResolveSecretWithSource("TEST_SECRET_VAR", "missing")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
	if src != SecretSourceNone {
		t.Fatalf("source=%v, want SecretSourceNone", src)
	}
}

func TestResolveSecret_CredsFileUnreadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file perms; test requires an unprivileged uid")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "unreadable")
	if err := os.WriteFile(path, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
	t.Setenv("TEST_SECRET_VAR", "")
	t.Setenv("CREDENTIALS_DIRECTORY", dir)

	_, _, err := ResolveSecretWithSource("TEST_SECRET_VAR", "unreadable")
	if err == nil {
		t.Fatal("expected error on unreadable cred file, got nil")
	}
}

func TestSecretSourceString(t *testing.T) {
	cases := map[SecretSource]string{
		SecretSourceEnv:   "env",
		SecretSourceCreds: "systemd-creds",
		SecretSourceNone:  "unset",
	}
	for src, want := range cases {
		if got := src.String(); got != want {
			t.Errorf("SecretSource(%d).String() = %q, want %q", src, got, want)
		}
	}
}
