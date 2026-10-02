//go:build !windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// lockFilePerms is the restricted permission set for the per-user lock file.
const lockFilePerms = 0o600

// acquireLock attempts to acquire an exclusive, non-blocking advisory lock
// on path using syscall.Flock.
// It returns:
//   - (*os.File, true, nil) if lock is acquired;
//   - (nil, false, nil) if another instance already holds the lock;
//   - (nil, false, err) if opening or creating the file fails (e.g. permissions or disk error).
func acquireLock(path string) (*os.File, bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, false, fmt.Errorf("creating lock directory %q: %w", filepath.Dir(path), err)
	}

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, lockFilePerms)
	if err != nil {
		return nil, false, fmt.Errorf("opening lock file %q: %w", path, err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, false, nil
	}
	return f, true, nil
}

// releaseLock releases the advisory lock and closes the file handle.
// Safe to call multiple times.
func releaseLock(f *os.File) {
	if f == nil {
		return
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	_ = f.Close()
}
