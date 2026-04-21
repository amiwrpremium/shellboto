package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/amiwrpremium/shellboto/internal/db"
)

// cmdDB dispatches "db <verb>".
func cmdDB(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: shellboto db <backup|info|vacuum> [flags]")
		return exitUsage
	}
	switch args[0] {
	case "backup":
		return cmdDBBackup(args[1:])
	case "info":
		return cmdDBInfo(args[1:])
	case "vacuum":
		return cmdDBVacuum(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown db subcommand %q\n", args[0])
		return exitUsage
	}
}

// cmdDBBackup takes an online snapshot via VACUUM INTO. Safe to run
// against a live DB because SQLite holds only a shared lock for the
// duration of the copy.
func cmdDBBackup(args []string) int {
	fs := flag.NewFlagSet("db backup", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", defaultConfigPath, "path to config file")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: shellboto db backup [-config PATH] <out-path>")
		return exitUsage
	}
	outPath := fs.Arg(0)
	if abs, err := filepath.Abs(outPath); err == nil {
		outPath = abs
	}
	if _, err := os.Stat(outPath); err == nil {
		fmt.Fprintf(os.Stderr, "refuse to overwrite existing file %s\n", outPath)
		return exitErr
	}

	cfg, err := loadConfigForCLI(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitErr
	}
	gormDB, cleanup, err := openDBForCLI(cfg.DBPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitErr
	}
	defer cleanup()

	// VACUUM INTO cannot be parameterized — the target is a literal in
	// the SQL grammar. Build the path quoted safely.
	if err := gormDB.Exec("VACUUM INTO ?", outPath).Error; err != nil {
		// Some drivers don't allow `?` in VACUUM INTO. Fall back to a
		// quoted literal after escaping single quotes.
		escaped := "'" + strings.ReplaceAll(outPath, "'", "''") + "'"
		if err2 := gormDB.Exec("VACUUM INTO " + escaped).Error; err2 != nil {
			fmt.Fprintf(os.Stderr, "vacuum into %s: %v\n", outPath, err2)
			return exitErr
		}
	}
	// Belt + braces: chmod the snapshot 0600. SQLite respects the process
	// umask (we don't tighten it here like Open does) — tighten after.
	_ = os.Chmod(outPath, 0o600)
	info, err := os.Stat(outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stat backup: %v\n", err)
		return exitErr
	}
	fmt.Printf("✅ backup written to %s (%d bytes)\n", outPath, info.Size())
	return exitOK
}

// cmdDBInfo prints file size, row counts, and a few SQLite pragmas.
func cmdDBInfo(args []string) int {
	fs := flag.NewFlagSet("db info", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", defaultConfigPath, "path to config file")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	cfg, err := loadConfigForCLI(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitErr
	}
	gormDB, cleanup, err := openDBForCLI(cfg.DBPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitErr
	}
	defer cleanup()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "path\t%s\n", cfg.DBPath)
	if info, err := os.Stat(cfg.DBPath); err == nil {
		fmt.Fprintf(w, "size\t%d bytes (%s)\n", info.Size(), humanBytes(info.Size()))
		fmt.Fprintf(w, "modified\t%s\n", info.ModTime().UTC().Format(time.RFC3339))
	}
	// Row counts.
	for _, table := range []string{"users", "audit_events", "audit_outputs"} {
		var n int64
		if err := gormDB.Raw("SELECT COUNT(*) FROM " + table).Scan(&n).Error; err != nil {
			fmt.Fprintf(w, "rows(%s)\terror: %v\n", table, err)
		} else {
			fmt.Fprintf(w, "rows(%s)\t%d\n", table, n)
		}
	}
	// Oldest / newest audit row.
	var oldest, newest time.Time
	_ = gormDB.Raw("SELECT COALESCE(MIN(ts), '') FROM audit_events").Scan(&oldest).Error
	_ = gormDB.Raw("SELECT COALESCE(MAX(ts), '') FROM audit_events").Scan(&newest).Error
	if !oldest.IsZero() {
		fmt.Fprintf(w, "audit_events.oldest\t%s\n", oldest.UTC().Format(time.RFC3339))
	}
	if !newest.IsZero() {
		fmt.Fprintf(w, "audit_events.newest\t%s\n", newest.UTC().Format(time.RFC3339))
	}
	// Pragmas.
	for _, p := range []string{"page_count", "page_size", "freelist_count", "journal_mode"} {
		var v string
		if err := gormDB.Raw("PRAGMA " + p).Scan(&v).Error; err == nil && v != "" {
			fmt.Fprintf(w, "pragma.%s\t%s\n", p, v)
		}
	}
	_ = w.Flush()
	return exitOK
}

// cmdDBVacuum runs VACUUM after acquiring the instance lock. If another
// shellboto process holds the lock, we refuse rather than corrupt the
// audit hash chain by racing on writes.
func cmdDBVacuum(args []string) int {
	fs := flag.NewFlagSet("db vacuum", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", defaultConfigPath, "path to config file")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	cfg, err := loadConfigForCLI(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitErr
	}

	// Instance-lock guard: refuse if the service (or another CLI run) holds
	// the lock. VACUUM rewrites every page — racing with the bot's audit
	// writer would break the chain.
	lock, err := db.AcquireInstanceLock(cfg.DBPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitErr
	}
	// Lockfile is never written — only flock'd. Close() returning an error
	// is unexpected; surface it rather than silently defer.
	defer func() {
		if err := lock.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "lockfile close: %v\n", err)
		}
	}()

	gormDB, cleanup, err := openDBForCLI(cfg.DBPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitErr
	}
	defer cleanup()

	if err := gormDB.Exec("VACUUM").Error; err != nil {
		fmt.Fprintf(os.Stderr, "vacuum: %v\n", err)
		return exitErr
	}
	fmt.Println("✅ VACUUM complete.")
	return exitOK
}

// humanBytes formats a byte count as KiB/MiB/GiB.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
