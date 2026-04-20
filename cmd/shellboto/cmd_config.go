package main

import (
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
)

// cmdConfig dispatches "config <verb>". Currently only "check".
func cmdConfig(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: shellboto config check [path]")
		return exitUsage
	}
	switch args[0] {
	case "check":
		return cmdConfigCheck(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown config subcommand %q\n", args[0])
		return exitUsage
	}
}

// cmdConfigCheck parses a config file and prints the effective values.
// Accepts the path either as a positional arg or via -config.
func cmdConfigCheck(args []string) int {
	fs := flag.NewFlagSet("config check", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configFlag := fs.String("config", "", "path to config file")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	path := *configFlag
	if path == "" && fs.NArg() > 0 {
		path = fs.Arg(0)
	}
	if path == "" {
		path = defaultConfigPath
	}

	cfg, err := loadConfigForCLI(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitErr
	}

	fmt.Printf("config %s: OK\n\n", path)
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "db_path\t%s\n", cfg.DBPath)
	fmt.Fprintf(w, "log_format\t%s\n", cfg.LogFormat)
	fmt.Fprintf(w, "log_level\t%s\n", cfg.LogLevel)
	fmt.Fprintf(w, "audit_retention\t%s\n", cfg.AuditRetention.Duration)
	fmt.Fprintf(w, "audit_output_mode\t%s\n", cfg.AuditOutputMode)
	fmt.Fprintf(w, "audit_max_blob_bytes\t%d\n", cfg.AuditMaxBlobBytes)
	fmt.Fprintf(w, "default_timeout\t%s\n", cfg.DefaultTimeout.Duration)
	fmt.Fprintf(w, "heartbeat\t%s\n", cfg.Heartbeat.Duration)
	fmt.Fprintf(w, "edit_interval\t%s\n", cfg.EditInterval.Duration)
	fmt.Fprintf(w, "max_message_chars\t%d\n", cfg.MaxMessageChars)
	fmt.Fprintf(w, "max_output_bytes\t%d\n", cfg.MaxOutputBytes)
	fmt.Fprintf(w, "idle_reap\t%s\n", cfg.IdleReap.Duration)
	fmt.Fprintf(w, "confirm_ttl\t%s\n", cfg.ConfirmTTL.Duration)
	fmt.Fprintf(w, "kill_grace\t%s\n", cfg.KillGrace.Duration)
	fmt.Fprintf(w, "queue_while_busy\t%t\n", cfg.QueueWhileBusy)
	fmt.Fprintf(w, "log_rejected\t%t\n", cfg.LogRejected)
	fmt.Fprintf(w, "strict_ascii_commands\t%t\n", cfg.StrictASCIICommands)
	fmt.Fprintf(w, "user_shell_user\t%q\n", cfg.UserShellUser)
	fmt.Fprintf(w, "user_shell_home\t%q\n", cfg.UserShellHome)
	fmt.Fprintf(w, "rate_limit_burst\t%d\n", cfg.RateLimitBurst)
	fmt.Fprintf(w, "rate_limit_refill_per_sec\t%v\n", cfg.RateLimitRefillPerSec)
	fmt.Fprintf(w, "auth_reject_burst\t%d\n", cfg.AuthRejectBurst)
	fmt.Fprintf(w, "auth_reject_refill_per_sec\t%v\n", cfg.AuthRejectRefillPerSec)
	fmt.Fprintf(w, "super_notify_action_ttl\t%s\n", cfg.SuperNotifyActionTTL.Duration)
	fmt.Fprintf(w, "extra_danger_patterns\t%d entries\n", len(cfg.ExtraDangerPatterns))
	fmt.Fprintf(w, "superadmin_id\t%d\n", cfg.SuperadminID)
	fmt.Fprintf(w, "token\t<set, %d bytes>\n", len(cfg.Token))
	_ = w.Flush()
	return exitOK
}
