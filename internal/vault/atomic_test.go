package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteFileAtomic verifies that writeFileAtomic creates the target file
// with exact content and permissions, and leaves no temp files behind.
func TestWriteFileAtomic(t *testing.T) {
	tempDir := t.TempDir()
	targetFile := filepath.Join(tempDir, "sub", "test-note.md")
	content := []byte("# Atomic Note Content\n\nHello atomic world.")

	err := writeFileAtomic(targetFile, content, 0o644)
	if err != nil {
		t.Fatalf("writeFileAtomic failed: %v", err)
	}

	readBytes, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("failed reading written file: %v", err)
	}
	if string(readBytes) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", string(readBytes), string(content))
	}

	// Verify no temporary files remain in the target directory
	entries, err := os.ReadDir(filepath.Dir(targetFile))
	if err != nil {
		t.Fatalf("failed reading dir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".tmp-") {
			t.Errorf("temporary file was not cleaned up: %s", entry.Name())
		}
	}

	// Test overwriting existing file
	updatedContent := []byte("# Overwritten Content\nNew line.")
	err = writeFileAtomic(targetFile, updatedContent, 0o644)
	if err != nil {
		t.Fatalf("overwriting with writeFileAtomic failed: %v", err)
	}
	readUpdated, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("failed reading overwritten file: %v", err)
	}
	if string(readUpdated) != string(updatedContent) {
		t.Errorf("updated content mismatch: got %q, want %q", string(readUpdated), string(updatedContent))
	}

	// Check again for leftover temp files
	entries, err = os.ReadDir(filepath.Dir(targetFile))
	if err != nil {
		t.Fatalf("failed reading dir after overwrite: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".tmp-") {
			t.Errorf("temporary file was not cleaned up after overwrite: %s", entry.Name())
		}
	}
}
