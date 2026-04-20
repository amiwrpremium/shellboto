package main

import (
	"fmt"
	"os"

	"gorm.io/gorm"

	"github.com/amiwrpremium/shellboto/internal/config"
	"github.com/amiwrpremium/shellboto/internal/db"
)

const defaultConfigPath = "/etc/shellboto/config.toml"

const (
	exitOK        = 0
	exitErr       = 1
	exitUsage     = 2
	exitCheckFail = 3
)

func dispatchSubcommand(name string, args []string) int {
	switch name {
	case "doctor":
		return cmdDoctor(args)
	case "config":
		return cmdConfig(args)
	case "audit":
		return cmdAudit(args)
	case "db":
		return cmdDB(args)
	case "users":
		return cmdUsers(args)
	case "service":
		return cmdService(args)
	case "simulate":
		return cmdSimulate(args)
	case "mint-seed":
		return cmdMintSeed(args)
	case "completion":
		return cmdCompletion(args)
	case "help", "-h", "--help":
		fmt.Print(topLevelHelp())
		return exitOK
	default:
		fmt.Fprintf(os.Stderr, "shellboto: unknown command %q\n\n", name)
		fmt.Fprint(os.Stderr, topLevelHelp())
		return exitUsage
	}
}

func topLevelHelp() string {
	return `shellboto — Telegram bot + CLI for remote VPS shell access.

Run with no args (or with -config / -version) to start the bot service.

Available subcommands:
  doctor              one-shot preflight check (config + env + paths + perms)
  config check [path] validate a config file and print effective values
  audit verify        walk the audit hash chain and report integrity
  audit search        list recent audit events (filters: --user, --kind, --since, --limit)
  audit export        stream audit events to stdout (--format json|csv)
  audit replay        cross-check journald audit mirror vs DB (stdin or --file)
  db backup <path>    online SQLite backup (VACUUM INTO)
  db info             size, row counts, pragma stats
  db vacuum           reclaim freelist space (refuses while service is running)
  users list          list all whitelisted users
  users tree          render the whitelist as a promoted-by tree
  service <verb>      wrappers: status/start/stop/restart/enable/disable/logs
  simulate <cmd...>   run the danger matcher against a command (no side effects)
  mint-seed           print a fresh 32-byte hex seed for SHELLBOTO_AUDIT_SEED
  completion <shell>  print a completion script (bash|zsh|fish)
  help                show this help

Every subcommand that reads config accepts -config <path> (default ` + defaultConfigPath + `).
Run "shellboto <cmd> -h" for per-command flags.
`
}

// loadConfigForCLI wraps config.Load with a consistent error prefix. CLI
// subcommands print the returned error to stderr and exit non-zero.
func loadConfigForCLI(path string) (*config.Config, error) {
	cfg, err := config.Load(path)
	if err != nil {
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	return cfg, nil
}

// openDBForCLI wraps db.Open with a consistent error prefix and returns a
// cleanup closure the caller MUST defer.
func openDBForCLI(path string) (*gorm.DB, func(), error) {
	gormDB, err := db.Open(path)
	if err != nil {
		return nil, func() {}, fmt.Errorf("open db %s: %w", path, err)
	}
	cleanup := func() { _ = db.Close(gormDB) }
	return gormDB, cleanup, nil
}
