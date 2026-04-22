package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Token               string   `toml:"-" json:"-" yaml:"-"`
	SuperadminID        int64    `toml:"-" json:"-" yaml:"-"`
	DBPath              string   `toml:"db_path" json:"db_path" yaml:"db_path"`
	AuditRetention      Duration `toml:"audit_retention" json:"audit_retention" yaml:"audit_retention"`
	DefaultTimeout      Duration `toml:"default_timeout" json:"default_timeout" yaml:"default_timeout"`
	Heartbeat           Duration `toml:"heartbeat" json:"heartbeat" yaml:"heartbeat"`
	MaxMessageChars     int      `toml:"max_message_chars" json:"max_message_chars" yaml:"max_message_chars"`
	EditInterval        Duration `toml:"edit_interval" json:"edit_interval" yaml:"edit_interval"`
	IdleReap            Duration `toml:"idle_reap" json:"idle_reap" yaml:"idle_reap"`
	QueueWhileBusy      bool     `toml:"queue_while_busy" json:"queue_while_busy" yaml:"queue_while_busy"`
	LogRejected         bool     `toml:"log_rejected" json:"log_rejected" yaml:"log_rejected"`
	ExtraDangerPatterns []string `toml:"extra_danger_patterns" json:"extra_danger_patterns" yaml:"extra_danger_patterns"`
	ConfirmTTL          Duration `toml:"confirm_ttl" json:"confirm_ttl" yaml:"confirm_ttl"`
	KillGrace           Duration `toml:"kill_grace" json:"kill_grace" yaml:"kill_grace"`
	LogFormat           string   `toml:"log_format" json:"log_format" yaml:"log_format"`
	LogLevel            string   `toml:"log_level" json:"log_level" yaml:"log_level"`
	// UserShellUser is the Unix account that `user`-role pty shells are
	// spawned as. Empty = fall back to root (dev mode) with a warning.
	UserShellUser string `toml:"user_shell_user" json:"user_shell_user" yaml:"user_shell_user"`
	// UserShellHome is the base directory under which per-telegram-user
	// home dirs are created (e.g. /home/shellboto-user/<telegram_id>/).
	// Empty = `/home/<UserShellUser>`.
	UserShellHome string `toml:"user_shell_home" json:"user_shell_home" yaml:"user_shell_home"`
	// StrictASCIICommands, when true, rejects any command containing
	// bytes outside printable ASCII + tab/newline before any regex
	// match. Blocks unicode homoglyph attacks and weird control bytes;
	// breaks legitimate unicode usage (accented filenames, i18n data).
	// Default false.
	StrictASCIICommands bool `toml:"strict_ascii_commands" json:"strict_ascii_commands" yaml:"strict_ascii_commands"`
	// MaxOutputBytes caps the per-command in-memory output buffer. On
	// overflow, the foreground process is SIGKILL'd and further bytes
	// are dropped. 0 = unlimited (not recommended). Default 50 MiB.
	MaxOutputBytes int `toml:"max_output_bytes" json:"max_output_bytes" yaml:"max_output_bytes"`
	// AuditOutputMode controls whether command stdout/stderr is stored
	// in audit_outputs: "always" | "errors_only" | "never".
	// Default: "always" (current behavior).
	AuditOutputMode string `toml:"audit_output_mode" json:"audit_output_mode" yaml:"audit_output_mode"`
	// AuditMaxBlobBytes caps the post-redact output size stored in
	// audit_outputs. Redacted output exceeding this is dropped (the
	// audit row still records metadata + `output_oversized` detail).
	// 0 disables the separate cap — then `MaxOutputBytes` alone bounds
	// the blob. Default 52428800 (50 MiB).
	AuditMaxBlobBytes int `toml:"audit_max_blob_bytes" json:"audit_max_blob_bytes" yaml:"audit_max_blob_bytes"`
	// RateLimitBurst is the per-Telegram-user token-bucket capacity.
	// 0 disables rate limiting. Default 10.
	RateLimitBurst int `toml:"rate_limit_burst" json:"rate_limit_burst" yaml:"rate_limit_burst"`
	// RateLimitRefillPerSec is the bucket refill rate. Default 1.0
	// (i.e., steady-state 60 actions/min per user).
	RateLimitRefillPerSec float64 `toml:"rate_limit_refill_per_sec" json:"rate_limit_refill_per_sec" yaml:"rate_limit_refill_per_sec"`
	// SuperNotifyActionTTL caps how long the quick-action inline
	// keyboard stays attached to super-notification DMs. After TTL a
	// scheduled sweep strips the keyboard; the event description text
	// stays in chat. 0 disables the TTL (buttons persist until tapped).
	// Default 10m.
	SuperNotifyActionTTL Duration `toml:"super_notify_action_ttl" json:"super_notify_action_ttl" yaml:"super_notify_action_ttl"`
	// AuthRejectBurst + AuthRejectRefillPerSec together form a
	// pre-auth rate limiter keyed by Telegram From-id, applied to
	// every auth_reject audit write. Without it, a non-whitelisted
	// attacker sending ~30 msg/sec would write ~600 MB of audit rows
	// per day until the disk fills. With defaults burst=5,
	// refill=0.05/sec, an attacker gets 5 immediate rows and then one
	// audit row per 20 seconds (~4300/day steady-state worst case).
	// burst=0 disables the limit (NOT recommended in production).
	AuthRejectBurst        int     `toml:"auth_reject_burst" json:"auth_reject_burst" yaml:"auth_reject_burst"`
	AuthRejectRefillPerSec float64 `toml:"auth_reject_refill_per_sec" json:"auth_reject_refill_per_sec" yaml:"auth_reject_refill_per_sec"`
}

type Duration struct{ time.Duration }

// UnmarshalText is honored by all three config parsers:
//   - BurntSushi/toml: native.
//   - encoding/json: falls back to UnmarshalText for JSON quoted strings.
//   - gopkg.in/yaml.v3: falls back to TextUnmarshaler for scalar strings.
//
// So Duration survives across every supported format with a single impl.
func (d *Duration) UnmarshalText(text []byte) error {
	v, err := time.ParseDuration(string(text))
	if err != nil {
		return err
	}
	d.Duration = v
	return nil
}

// Load reads a config file at `path` (extension-dispatched to the
// appropriate parser) and overlays it on top of built-in defaults.
// Supported extensions: .toml, .yaml, .yml, .json. An unknown
// extension is a fatal error rather than silently trying a default —
// operators should explicitly name the format.
//
// Every critical value also draws from the environment:
//
//   - SHELLBOTO_TOKEN (required). If empty, falls back to reading
//     $CREDENTIALS_DIRECTORY/shellboto-token — the file-backed delivery
//     used by systemd-creds. See ResolveSecret.
//   - SHELLBOTO_SUPERADMIN_ID (required). Env-only (not a secret; just
//     an identifier).
//   - SHELLBOTO_AUDIT_SEED (read via main, same env/creds fallback as
//     the token — credential name: shellboto-audit-seed).
func Load(path string) (*Config, error) {
	c := &Config{
		DBPath:                 "/var/lib/shellboto/state.db",
		AuditRetention:         Duration{90 * 24 * time.Hour},
		DefaultTimeout:         Duration{5 * time.Minute},
		Heartbeat:              Duration{30 * time.Second},
		MaxMessageChars:        4096,
		EditInterval:           Duration{time.Second},
		IdleReap:               Duration{time.Hour},
		QueueWhileBusy:         true,
		ConfirmTTL:             Duration{60 * time.Second},
		KillGrace:              Duration{5 * time.Second},
		LogFormat:              "json",
		LogLevel:               "info",
		MaxOutputBytes:         50 * 1024 * 1024, // 50 MiB
		AuditOutputMode:        "always",
		AuditMaxBlobBytes:      50 * 1024 * 1024, // 50 MiB
		RateLimitBurst:         10,
		RateLimitRefillPerSec:  1.0,
		SuperNotifyActionTTL:   Duration{10 * time.Minute},
		AuthRejectBurst:        5,
		AuthRejectRefillPerSec: 0.05,
	}
	if path != "" {
		if err := decodeConfigFile(path, c); err != nil {
			return nil, err
		}
	}
	token, err := ResolveSecret("SHELLBOTO_TOKEN", "shellboto-token")
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, fmt.Errorf("SHELLBOTO_TOKEN is empty " +
			"(not set in env and no shellboto-token in $CREDENTIALS_DIRECTORY)")
	}
	c.Token = token
	superStr := os.Getenv("SHELLBOTO_SUPERADMIN_ID")
	if superStr == "" {
		return nil, fmt.Errorf("SHELLBOTO_SUPERADMIN_ID env var is empty")
	}
	id, err := strconv.ParseInt(superStr, 10, 64)
	if err != nil || id < 1 {
		// Reject 0 AND negative values — real Telegram user IDs are
		// strictly positive; a negative value would seed a "ghost"
		// superadmin that nobody's /role, /promote, etc. can ever
		// match, silently disabling all super-only operations.
		return nil, fmt.Errorf("SHELLBOTO_SUPERADMIN_ID must be a positive integer, got %q", superStr)
	}
	c.SuperadminID = id

	switch c.LogFormat {
	case "json", "console":
	default:
		return nil, fmt.Errorf("config: log_format must be \"json\" or \"console\", got %q", c.LogFormat)
	}
	if c.DBPath == "" {
		return nil, fmt.Errorf("config: db_path is required")
	}
	switch c.AuditOutputMode {
	case "always", "errors_only", "never":
	default:
		return nil, fmt.Errorf("config: audit_output_mode must be one of always | errors_only | never, got %q", c.AuditOutputMode)
	}
	return c, nil
}

// decodeConfigFile dispatches by extension. Each parser gets the raw
// file bytes (or filename for TOML) and overlays onto `c`. Unknown
// extensions are a hard error to avoid silent misconfiguration.
func decodeConfigFile(path string, c *Config) error {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".toml":
		if _, err := toml.DecodeFile(path, c); err != nil {
			return fmt.Errorf("config %s: %w", path, err)
		}
	case ".yaml", ".yml":
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read config %s: %w", path, err)
		}
		if err := yaml.Unmarshal(data, c); err != nil {
			return fmt.Errorf("config %s: %w", path, err)
		}
	case ".json":
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read config %s: %w", path, err)
		}
		if err := json.Unmarshal(data, c); err != nil {
			return fmt.Errorf("config %s: %w", path, err)
		}
	default:
		return fmt.Errorf("config %s: unsupported extension %q (use .toml, .yaml, .yml, or .json)", path, ext)
	}
	return nil
}
