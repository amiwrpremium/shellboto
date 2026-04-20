package db

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open creates or opens the SQLite DB, runs AutoMigrate for every model,
// and returns the GORM handle. Callers build repositories on top of the
// returned *gorm.DB.
func Open(path string) (*gorm.DB, error) {
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return nil, fmt.Errorf("mkdir db parent: %w", err)
	}
	// MkdirAll only sets mode on newly-created dirs. If the
	// dir pre-existed under a looser umask (likely when someone runs
	// the bot manually outside systemd, without StateDirectoryMode=
	// 0700), tighten it now so the state dir doesn't leak directory
	// enumeration to other users on the host.
	_ = os.Chmod(parent, 0o700)
	// Tighten the process umask so any file sqlite creates
	// (.db, .db-wal, .db-shm) is born 0600 instead of umask-default
	// 0644. Closes the startup window where a newly-created DB sat
	// world-readable between driver-open and the post-hoc Chmod below.
	// Restored before Open returns so the rest of the process is
	// unaffected. Not goroutine-safe, but main calls this before any
	// other goroutine is spawned.
	oldMask := syscall.Umask(0o177)
	defer syscall.Umask(oldMask)

	// Clean URI DSN with no driver-specific query params.
	// Pragmas below are applied via db.Exec so they work under both
	// mattn/go-sqlite3 (`_foreign_keys=on`) and modernc/sqlite
	// (`_pragma=foreign_keys(ON)`) drivers without DSN divergence.
	dsn := "file:" + path
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("gorm.Open: %w", err)
	}
	// Defense-in-depth: explicitly chmod the main DB file in case it
	// pre-existed under a looser mode from an older deploy. The umask
	// above covers freshly-created files; this covers the legacy case.
	_ = os.Chmod(path, 0o600)

	// Constrain the connection pool to a single conn so
	// per-connection pragmas (foreign_keys, busy_timeout) stay
	// applied to every DB op. With a multi-conn pool, each fresh
	// connection would default back to `foreign_keys=OFF`, silently
	// breaking the audit_outputs → audit_events cascade. Our workload
	// is serialized elsewhere already (logMu mutex on audit.Log) and
	// low-volume, so 1 conn is not a throughput concern.
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("sqlDB: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	// Apply pragmas explicitly. journal_mode is per-database (persists
	// in the file); the rest are per-connection but stick because of
	// the pool cap above.
	for _, pragma := range []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
	} {
		if err := db.Exec(pragma).Error; err != nil {
			return nil, fmt.Errorf("apply %q: %w", pragma, err)
		}
	}

	if err := Migrate(db); err != nil {
		return nil, err
	}
	return db, nil
}

// Close closes the underlying *sql.DB.
func Close(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
