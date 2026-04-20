package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/amiwrpremium/shellboto/internal/danger"
)

// cmdSimulate runs the danger matcher against a command string and
// reports whether (and which) pattern fired. No side effects; safe to
// paste any command here.
func cmdSimulate(args []string) int {
	fs := flag.NewFlagSet("simulate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", defaultConfigPath, "path to config file (for extra_danger_patterns)")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: shellboto simulate [-config PATH] <command...>")
		return exitUsage
	}
	cmd := strings.Join(fs.Args(), " ")

	// Config is optional — if it fails we fall back to the built-in
	// defaults. The simulate command should be usable even before the
	// config is wired up.
	var extra []string
	if cfg, err := loadConfigForCLI(*configPath); err == nil {
		extra = cfg.ExtraDangerPatterns
	}

	m, err := danger.New(extra)
	if err != nil {
		fmt.Fprintf(os.Stderr, "compile patterns: %v\n", err)
		return exitErr
	}
	pattern, hit := m.Match(cmd)
	if hit {
		fmt.Printf("⚠  DANGER: matched pattern %q\n    cmd: %s\n", pattern, cmd)
		return exitCheckFail
	}
	fmt.Printf("✅ no danger pattern matched\n    cmd: %s\n", cmd)
	return exitOK
}
