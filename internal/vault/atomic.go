package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// writeFileAtomic writes data to a temporary file in the same directory as path,
// flushes it to disk, and atomically renames it to path to prevent partial writes.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("creating temp file in %s: %w", dir, err)
	}
	tmpName := tmpFile.Name()

	cleanUp := true
	defer func() {
		if cleanUp {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("writing temp file %s: %w", tmpName, err)
	}

	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("syncing temp file %s: %w", tmpName, err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("closing temp file %s: %w", tmpName, err)
	}

	if err := os.Chmod(tmpName, perm); err != nil {
		return fmt.Errorf("setting permissions on %s: %w", tmpName, err)
	}

	var renameErr error
	for attempt := 0; attempt < 5; attempt++ {
		renameErr = os.Rename(tmpName, path)
		if renameErr == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if renameErr != nil {
		return fmt.Errorf("renaming %s to %s: %w", tmpName, path, renameErr)
	}

	cleanUp = false
	return nil
}
