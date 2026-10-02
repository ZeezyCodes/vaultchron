//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAcquireLock_PermissionsAndBehavior verifies that acquireLock creates a file
// with 0o600 permissions, distinguishes open errors from held locks, and releases cleanly.
func TestAcquireLock_PermissionsAndBehavior(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "sub", "test.lock")

	// 1. Initial acquisition
	f1, acquired1, err1 := acquireLock(lockPath)
	if err1 != nil {
		t.Fatalf("unexpected error acquiring lock: %v", err1)
	}
	if !acquired1 || f1 == nil {
		t.Fatalf("expected lock to be acquired")
	}

	// Verify permissions are 0o600
	fi, err := os.Stat(lockPath)
	if err != nil {
		t.Fatalf("failed to stat lock file: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("expected lock file permissions 0600, got %#o", fi.Mode().Perm())
	}

	// 2. Second acquisition attempt on the same locked file (should report lock held, err == nil)
	f2, acquired2, err2 := acquireLock(lockPath)
	if err2 != nil {
		t.Errorf("expected nil error when lock is held by another handle, got %v", err2)
	}
	if acquired2 {
		t.Errorf("expected acquired=false when lock is already held")
		releaseLock(f2)
	}
	if f2 != nil {
		t.Errorf("expected nil file handle when lock is held")
	}

	// 3. Release initial lock, then try again (should succeed)
	releaseLock(f1)

	f3, acquired3, err3 := acquireLock(lockPath)
	if err3 != nil {
		t.Fatalf("unexpected error re-acquiring released lock: %v", err3)
	}
	if !acquired3 || f3 == nil {
		t.Fatalf("expected lock to be acquired after release")
	}
	releaseLock(f3)

	// 4. Open failure (invalid path that cannot be created)
	// Using a path inside a file rather than a directory
	badPath := filepath.Join(lockPath, "impossible", "lock.file")
	f4, acquired4, err4 := acquireLock(badPath)
	if err4 == nil {
		t.Errorf("expected non-nil error when file cannot be opened/created")
		releaseLock(f4)
	}
	if acquired4 {
		t.Errorf("expected acquired=false on open failure")
	}
}
