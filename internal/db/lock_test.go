package db

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAcquireInstanceLock_FirstCallSucceeds(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "state.db")
	f, err := AcquireInstanceLock(dbPath)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if f == nil {
		t.Fatalf("returned nil file handle")
	}
	t.Cleanup(func() { _ = f.Close() })
}

func TestAcquireInstanceLock_SecondCallFails(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "state.db")
	f1, err := AcquireInstanceLock(dbPath)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	t.Cleanup(func() { _ = f1.Close() })

	f2, err := AcquireInstanceLock(dbPath)
	if err == nil {
		_ = f2.Close()
		t.Fatalf("second acquire should have failed while first holds the lock")
	}
	if !strings.Contains(err.Error(), "another shellboto instance") {
		t.Fatalf("error %q doesn't mention the expected conflict phrase", err)
	}
}

func TestAcquireInstanceLock_ReleaseOnClose(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "state.db")
	f1, err := AcquireInstanceLock(dbPath)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if err := f1.Close(); err != nil {
		t.Fatalf("close first: %v", err)
	}

	// After closing f1 the kernel releases the lock; a fresh acquire
	// must succeed again.
	f2, err := AcquireInstanceLock(dbPath)
	if err != nil {
		t.Fatalf("re-acquire after Close: %v", err)
	}
	t.Cleanup(func() { _ = f2.Close() })
}
