package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// configLoadEnvOnly exercises Load with no TOML file (path=""). It
// only checks the env-var-derived fields. We pass "" to skip the
// DecodeFile branch and rely on the defaults + env overrides.
func loadNoFile(t *testing.T) (*Config, error) {
	t.Helper()
	return Load("")
}

func TestLoad_SuperadminID_Valid(t *testing.T) {
	t.Setenv("SHELLBOTO_TOKEN", "dummy")
	t.Setenv("SHELLBOTO_SUPERADMIN_ID", "12345")
	cfg, err := loadNoFile(t)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SuperadminID != 12345 {
		t.Fatalf("SuperadminID = %d, want 12345", cfg.SuperadminID)
	}
}

func TestLoad_SuperadminID_RejectsZero(t *testing.T) {
	t.Setenv("SHELLBOTO_TOKEN", "dummy")
	t.Setenv("SHELLBOTO_SUPERADMIN_ID", "0")
	_, err := loadNoFile(t)
	if err == nil {
		t.Fatalf("expected error for superadmin id=0")
	}
	if !strings.Contains(err.Error(), "positive integer") {
		t.Errorf("error %q doesn't say positive-integer", err)
	}
}

// TestLoad_SuperadminID_RejectsNegative locks in the guard against
// negative SHELLBOTO_SUPERADMIN_ID values. Before the guard, a negative
// id parsed cleanly and silently seeded a ghost superadmin (no real
// Telegram user matches), disabling all super-only operations.
func TestLoad_SuperadminID_RejectsNegative(t *testing.T) {
	for _, v := range []string{"-1", "-12345"} {
		t.Setenv("SHELLBOTO_TOKEN", "dummy")
		t.Setenv("SHELLBOTO_SUPERADMIN_ID", v)
		_, err := loadNoFile(t)
		if err == nil {
			t.Errorf("expected error for negative superadmin id %q", v)
			continue
		}
		if !strings.Contains(err.Error(), "positive integer") {
			t.Errorf("%q: error %q doesn't say positive-integer", v, err)
		}
	}
}

func TestLoad_SuperadminID_RejectsNonNumeric(t *testing.T) {
	t.Setenv("SHELLBOTO_TOKEN", "dummy")
	t.Setenv("SHELLBOTO_SUPERADMIN_ID", "abc")
	_, err := loadNoFile(t)
	if err == nil {
		t.Fatalf("expected error for non-numeric superadmin id")
	}
}

// writeTempConfig writes contents to a file with the given extension
// inside a test-scoped tempdir, returning the path.
func writeTempConfig(t *testing.T, ext, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg"+ext)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// loadFromFile exercises Load with env set + a specific file path.
func loadFromFile(t *testing.T, path string) (*Config, error) {
	t.Helper()
	t.Setenv("SHELLBOTO_TOKEN", "dummy")
	t.Setenv("SHELLBOTO_SUPERADMIN_ID", "12345")
	return Load(path)
}

func TestLoad_TOML(t *testing.T) {
	path := writeTempConfig(t, ".toml", `
db_path = "/tmp/state.db"
default_timeout = "3m"
max_message_chars = 2048
rate_limit_burst = 7
extra_danger_patterns = ["foo", "bar"]
strict_ascii_commands = true
`)
	c, err := loadFromFile(t, path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	assertCommonFields(t, c)
}

func TestLoad_YAML(t *testing.T) {
	path := writeTempConfig(t, ".yaml", `
db_path: /tmp/state.db
default_timeout: 3m
max_message_chars: 2048
rate_limit_burst: 7
extra_danger_patterns:
  - foo
  - bar
strict_ascii_commands: true
`)
	c, err := loadFromFile(t, path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	assertCommonFields(t, c)
}

func TestLoad_YML(t *testing.T) {
	// .yml should also work.
	path := writeTempConfig(t, ".yml", `
db_path: /tmp/state.db
default_timeout: 3m
max_message_chars: 2048
rate_limit_burst: 7
extra_danger_patterns: [foo, bar]
strict_ascii_commands: true
`)
	c, err := loadFromFile(t, path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	assertCommonFields(t, c)
}

func TestLoad_JSON(t *testing.T) {
	path := writeTempConfig(t, ".json", `{
  "db_path": "/tmp/state.db",
  "default_timeout": "3m",
  "max_message_chars": 2048,
  "rate_limit_burst": 7,
  "extra_danger_patterns": ["foo", "bar"],
  "strict_ascii_commands": true
}`)
	c, err := loadFromFile(t, path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	assertCommonFields(t, c)
}

func TestLoad_UnknownExtension(t *testing.T) {
	path := writeTempConfig(t, ".ini", "db_path=/tmp/x\n")
	_, err := loadFromFile(t, path)
	if err == nil {
		t.Fatalf("expected error for unsupported extension")
	}
	if !strings.Contains(err.Error(), "unsupported extension") {
		t.Errorf("error %q doesn't mention unsupported extension", err)
	}
}

// assertCommonFields verifies the common field values used by the
// three format tests — they all populate the same fields with the
// same values, so the assertion set is shared.
func assertCommonFields(t *testing.T, c *Config) {
	t.Helper()
	if c.DBPath != "/tmp/state.db" {
		t.Errorf("DBPath = %q", c.DBPath)
	}
	if c.DefaultTimeout.Duration != 3*time.Minute {
		t.Errorf("DefaultTimeout = %v", c.DefaultTimeout.Duration)
	}
	if c.MaxMessageChars != 2048 {
		t.Errorf("MaxMessageChars = %d", c.MaxMessageChars)
	}
	if c.RateLimitBurst != 7 {
		t.Errorf("RateLimitBurst = %d", c.RateLimitBurst)
	}
	if !c.StrictASCIICommands {
		t.Errorf("StrictASCIICommands = false, want true")
	}
	if len(c.ExtraDangerPatterns) != 2 ||
		c.ExtraDangerPatterns[0] != "foo" ||
		c.ExtraDangerPatterns[1] != "bar" {
		t.Errorf("ExtraDangerPatterns = %v", c.ExtraDangerPatterns)
	}
	// Unset fields must still hold their defaults.
	if c.AuditRetention.Duration != 90*24*time.Hour {
		t.Errorf("AuditRetention default lost: %v", c.AuditRetention.Duration)
	}
	if c.AuditOutputMode != "always" {
		t.Errorf("AuditOutputMode default lost: %q", c.AuditOutputMode)
	}
}
