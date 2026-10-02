package collector

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestExpandPath verifies that agent path expansion supports ~, ~/, and ~\
// and cleans paths portably.
func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("skipping test: UserHomeDir unavailable")
	}

	expectedBackslash := `~\.antigravity`
	expectedNested := `~\.poolside\logs`
	if runtime.GOOS == "windows" {
		expectedBackslash = filepath.Join(home, ".antigravity")
		expectedNested = filepath.Join(home, ".poolside", "logs")
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"bare tilde", "~", filepath.Clean(home)},
		{"forward slash tilde", "~/.antigravity", filepath.Join(home, ".antigravity")},
		{"backslash tilde", `~\.antigravity`, expectedBackslash},
		{"nested path", `~\.poolside\logs`, expectedNested},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandPath(tt.input)
			if got != tt.expected {
				t.Errorf("expandPath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
