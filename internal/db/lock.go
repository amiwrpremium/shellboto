package db

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// AcquireInstanceLock takes an exclusive non-blocking flock on
// `<dir>/shellboto.lock`, where dir is the parent of the configured
// DB path. Returns the held *os.File — caller MUST keep it alive for
// the process lifetime (closing it releases the lock). On conflict
// (another instance holding the lock) returns an error with a clear
// operator-facing message.
//
// Without this, two shellboto processes pointed at the same
// state.db would race on the audit hash chain — our `logMu` mutex
// serializes audit writes within a process but not across processes.
//
// Uses a dedicated lockfile rather than flock-ing state.db directly
// to avoid interfering with SQLite's own POSIX advisory locks on the
// DB file. Kernel auto-releases on process death (clean exit or
// crash), so stale lockfiles are never a problem.
func AcquireInstanceLock(dbPath string) (*os.File, error) {
	lockPath := filepath.Join(filepath.Dir(dbPath), "shellboto.lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lockfile %s: %w", lockPath, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf(
				"another shellboto instance is holding the lock at %s; "+
					"if systemd reports the service running, this is expected — "+
					"do not run manually",
				lockPath)
		}
		return nil, fmt.Errorf("flock %s: %w", lockPath, err)
	}
	return f, nil
}
