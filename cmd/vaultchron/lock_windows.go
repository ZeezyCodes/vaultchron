//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// errorSharingViolation is the Windows error code returned when another process or handle
// holds an exclusive lock on the file (ERROR_SHARING_VIOLATION, Errno 32).
const errorSharingViolation = syscall.Errno(32)

// acquireLock attempts to acquire an exclusive, non-blocking lock on path using syscall.CreateFile
// with share mode 0 (no sharing).
// It returns:
//   - (*os.File, true, nil) if lock is acquired;
//   - (nil, false, nil) if another instance already holds the lock (ERROR_SHARING_VIOLATION);
//   - (nil, false, err) if opening or creating the file fails.
func acquireLock(path string) (*os.File, bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, false, fmt.Errorf("creating lock directory %q: %w", filepath.Dir(path), err)
	}

	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, false, fmt.Errorf("converting lock file path %q: %w", path, err)
	}

	handle, err := syscall.CreateFile(
		pathPtr,
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		0, // exclusive access, no sharing
		nil,
		syscall.OPEN_ALWAYS,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		if errors.Is(err, errorSharingViolation) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("opening lock file %q: %w", path, err)
	}

	return os.NewFile(uintptr(handle), path), true, nil
}

// releaseLock releases the exclusive lock by closing the file handle.
// Safe to call multiple times.
func releaseLock(f *os.File) {
	if f == nil {
		return
	}
	_ = f.Close()
}
