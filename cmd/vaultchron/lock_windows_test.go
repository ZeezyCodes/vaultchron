//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAcquireLock_Windows verifies that acquireLock on Windows creates parent directories,
// acquires an exclusive lock, rejects secondary acquisition with acquired=false and nil error,
// and permits acquisition after release.
func TestAcquireLock_Windows(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "sub", "test.lock")

	// 1. Initial acquisition (parent directory created)
	f1, acquired1, err1 := acquireLock(lockPath)
	if err1 != nil {
		t.Fatalf("unexpected error acquiring initial lock: %v", err1)
	}
	if !acquired1 || f1 == nil {
		t.Fatalf("expected initial lock to be acquired")
	}

	// Verify parent directory was created
	dirInfo, err := os.Stat(filepath.Dir(lockPath))
	if err != nil || !dirInfo.IsDir() {
		t.Fatalf("expected parent directory %q to be created: %v", filepath.Dir(lockPath), err)
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
