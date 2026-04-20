package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/amiwrpremium/shellboto/internal/config"
)

// cmdDoctor runs a preflight check across config, env, filesystem, and
// the configured Unix shell user. Each check prints a ✅ / ❌ line; the
// command exits 0 only if every check passes.
func cmdDoctor(args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", defaultConfigPath, "path to config file")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	return runDoctor(*configPath, os.Stdout)
}

func runDoctor(configPath string, out io.Writer) int {
	r := &doctorReport{out: out}
	fmt.Fprintf(out, "shellboto doctor — using config %s\n\n", configPath)

	// 1) Config file parses + validates.
	cfg, err := config.Load(configPath)
	if err != nil {
		r.fail("config parses + validates", err.Error())
		// Without a valid config we can't run most downstream checks.
		// Still try the env-only ones so operators get a full picture.
		cfg = nil
	} else {
		r.pass("config parses + validates", fmt.Sprintf("db_path=%s log_format=%s", cfg.DBPath, cfg.LogFormat))
	}

	// 2) Required env vars.
	if os.Getenv("SHELLBOTO_TOKEN") == "" {
		r.fail("SHELLBOTO_TOKEN set", "empty or unset")
	} else {
		r.pass("SHELLBOTO_TOKEN set", "")
	}
	if raw := os.Getenv("SHELLBOTO_SUPERADMIN_ID"); raw == "" {
		r.fail("SHELLBOTO_SUPERADMIN_ID set", "empty or unset")
	} else if id, err := strconv.ParseInt(raw, 10, 64); err != nil || id < 1 {
		r.fail("SHELLBOTO_SUPERADMIN_ID set", fmt.Sprintf("must be a positive integer, got %q", raw))
	} else {
		r.pass("SHELLBOTO_SUPERADMIN_ID set", fmt.Sprintf("id=%d", id))
	}

	// 3) Audit seed.
	switch seed, seedSet, err := auditSeedDecode(); {
	case err != nil:
		r.fail("SHELLBOTO_AUDIT_SEED valid", err.Error())
	case !seedSet:
		r.warn("SHELLBOTO_AUDIT_SEED valid", "not set — dev mode (all-zeros fallback)")
	default:
		r.pass("SHELLBOTO_AUDIT_SEED valid", fmt.Sprintf("%d bytes", len(seed)))
	}

	// 4) DB path writable (or already exists).
	if cfg != nil {
		if err := checkDBPath(cfg.DBPath); err != nil {
			r.fail("db_path writable", err.Error())
		} else {
			r.pass("db_path writable", cfg.DBPath)
		}
	}

	// 5) user_shell_user resolution.
	if cfg != nil {
		if cfg.UserShellUser == "" {
			r.warn("user_shell_user set", "empty — user-role shells will run as root (see README § Non-root shells)")
		} else {
			if detail, err := checkUserShell(cfg); err != nil {
				r.fail("user_shell_user resolves", err.Error())
			} else {
				r.pass("user_shell_user resolves", detail)
			}
		}
	}

	fmt.Fprintln(out)
	if r.failed > 0 {
		fmt.Fprintf(out, "%d check(s) failed, %d warning(s).\n", r.failed, r.warned)
		return exitCheckFail
	}
	if r.warned > 0 {
		fmt.Fprintf(out, "All checks passed (%d warning(s)).\n", r.warned)
	} else {
		fmt.Fprintln(out, "All checks passed.")
	}
	return exitOK
}

// auditSeedDecode parses SHELLBOTO_AUDIT_SEED. Returns:
//   - seed: decoded bytes (or nil if env var is empty)
//   - seedSet: true when the env var was present + decoded
//   - err: non-nil when present but invalid (hex / length)
func auditSeedDecode() ([]byte, bool, error) {
	raw := os.Getenv("SHELLBOTO_AUDIT_SEED")
	if raw == "" {
		return nil, false, nil
	}
	seed, err := hex.DecodeString(raw)
	if err != nil {
		return nil, true, fmt.Errorf("not valid hex: %w", err)
	}
	if len(seed) != 32 {
		return nil, true, fmt.Errorf("must decode to 32 bytes, got %d", len(seed))
	}
	return seed, true, nil
}

// checkDBPath verifies the parent dir exists (or is creatable) and is
// writable by the current process.
func checkDBPath(path string) error {
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("mkdir parent %s: %w", parent, err)
	}
	probe := filepath.Join(parent, ".shellboto-doctor-probe")
	f, err := os.OpenFile(probe, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("write probe in %s: %w", parent, err)
	}
	_ = f.Close()
	_ = os.Remove(probe)
	return nil
}

// checkUserShell mirrors resolveUserShell in main.go but returns a detail
// string + error instead of logging + Fatal.
func checkUserShell(cfg *config.Config) (string, error) {
	u, err := user.Lookup(cfg.UserShellUser)
	if err != nil {
		return "", fmt.Errorf("lookup %q: %w", cfg.UserShellUser, err)
	}
	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		return "", fmt.Errorf("parse uid %q: %w", u.Uid, err)
	}
	gid, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		return "", fmt.Errorf("parse gid %q: %w", u.Gid, err)
	}
	if uid == 0 {
		return "", fmt.Errorf("%q resolves to uid=0; point it at a non-root account", cfg.UserShellUser)
	}
	home := cfg.UserShellHome
	if home == "" {
		home = filepath.Join("/home", cfg.UserShellUser)
	}
	return fmt.Sprintf("%s uid=%d gid=%d home=%s", cfg.UserShellUser, uid, gid, home), nil
}

// doctorReport tracks per-check results and writes formatted lines.
type doctorReport struct {
	out    io.Writer
	failed int
	warned int
}

func (r *doctorReport) pass(name, detail string) {
	fmt.Fprintf(r.out, "  ✅ %s", name)
	if detail != "" {
		fmt.Fprintf(r.out, "  %s", detail)
	}
	fmt.Fprintln(r.out)
}

func (r *doctorReport) warn(name, detail string) {
	r.warned++
	fmt.Fprintf(r.out, "  ⚠  %s", name)
	if detail != "" {
		fmt.Fprintf(r.out, "  %s", detail)
	}
	fmt.Fprintln(r.out)
}

func (r *doctorReport) fail(name, detail string) {
	r.failed++
	fmt.Fprintf(r.out, "  ❌ %s", name)
	if detail != "" {
		fmt.Fprintf(r.out, "  %s", strings.TrimSpace(detail))
	}
	fmt.Fprintln(r.out)
}
