package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

// TestLockPathForUID verifies per-user lock path construction.
func TestLockPathForUID(t *testing.T) {
	tempDir := t.TempDir()
	uid := 1000

	path := lockPathForUID(tempDir, uid)
	expected := filepath.Join(tempDir, "vaultchron-1000.lock")
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
	if path == filepath.Join(tempDir, "vaultchron.lock") {
		t.Errorf("lock path should not literally be fallback: %q", path)
	}

	// Test fallback when UID < 0 (Windows style)
	fallbackPath := lockPathForUID(tempDir, -1)
	if !strings.HasSuffix(fallbackPath, "vaultchron.lock") {
		t.Errorf("expected suffix vaultchron.lock, got %q", fallbackPath)
	}
}

// TestValidWindowRegex verifies that valid git window strings pass and invalid/malicious
// strings are rejected.
func TestValidWindowRegex(t *testing.T) {
	validWindows := []string{
		"24.hours.ago",
		"1.hour.ago",
		"12.hours.ago",
		"1.day.ago",
		"7.days.ago",
		"2.weeks.ago",
		"1.week.ago",
		"30.minutes.ago",
		"1.minute.ago",
		"3.months.ago",
	}

	for _, w := range validWindows {
		if !validWindowRegex.MatchString(w) {
			t.Errorf("expected valid window %q to match regex", w)
		}
	}

	invalidWindows := []string{
		"",
		"24 hours ago", // spaces rejected
		"; rm -rf /",   // injection
		"yesterday",    // unapproved word
		"invalid",
		"-1.days.ago",
		"24.hours.ago; echo",
		"24.lightyears.ago",
	}

	for _, w := range invalidWindows {
		if validWindowRegex.MatchString(w) {
			t.Errorf("expected invalid window %q NOT to match regex", w)
		}
	}
}

// TestResolveVersion verifies version resolution across explicit, fallback, and empty cases.
func TestResolveVersion(t *testing.T) {
	tests := []struct {
		name     string
		v        string
		readInfo func() (*debug.BuildInfo, bool)
		expected string
	}{
		{
			name: "explicit version overrides fallback",
			v:    "v1.2.3",
			readInfo: func() (*debug.BuildInfo, bool) {
				return &debug.BuildInfo{
					Main: debug.Module{Version: "v9.9.9"},
				}, true
			},
			expected: "v1.2.3",
		},
		{
			name: "fallback to build info when dev",
			v:    "dev",
			readInfo: func() (*debug.BuildInfo, bool) {
				return &debug.BuildInfo{
					Main: debug.Module{Version: "v0.1.0"},
				}, true
			},
			expected: "v0.1.0",
		},
		{
			name: "fallback skipped when build info is devel",
			v:    "dev",
			readInfo: func() (*debug.BuildInfo, bool) {
				return &debug.BuildInfo{
					Main: debug.Module{Version: "(devel)"},
				}, true
			},
			expected: "dev",
		},
		{
			name: "empty build info version returns dev",
			v:    "dev",
			readInfo: func() (*debug.BuildInfo, bool) {
				return &debug.BuildInfo{
					Main: debug.Module{Version: ""},
				}, true
			},
			expected: "dev",
		},
		{
			name: "nil build info returns dev",
			v:    "dev",
			readInfo: func() (*debug.BuildInfo, bool) {
				return nil, false
			},
			expected: "dev",
		},
		{
			name:     "empty version string returns dev",
			v:        "",
			readInfo: nil,
			expected: "dev",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveVersion(tc.v, tc.readInfo)
			if got != tc.expected {
				t.Errorf("resolveVersion(%q) = %q; want %q", tc.v, got, tc.expected)
			}
		})
	}
}

// TestHasVersionFlag verifies detection of -version and --version flags.
func TestHasVersionFlag(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected bool
	}{
		{"single dash", []string{"-version"}, true},
		{"double dash", []string{"--version"}, true},
		{"with preceding flags", []string{"-config", "foo.yaml", "-version"}, true},
		{"with following flags", []string{"-version", "-scan"}, true},
		{"no version flag", []string{"-scan", "-dry-run"}, false},
		{"empty args", []string{}, false},
		{"bare double dash terminates search", []string{"--", "-version"}, false},
		{"flag value is not version flag", []string{"-repo", "version"}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := hasVersionFlag(tc.args)
			if got != tc.expected {
				t.Errorf("hasVersionFlag(%v) = %v; want %v", tc.args, got, tc.expected)
			}
		})
	}
}

// TestVersionDoesNotTouchLock verifies that -version handling executes before
// lock acquisition and does not attempt to acquire, create, or touch the lock file.
func TestVersionDoesNotTouchLock(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("TMPDIR", tmpDir)
	t.Setenv("LocalAppData", tmpDir)

	lockPath := defaultLockPath()

	// 1. Hold exclusive lock on lockPath
	f, acquired, err := acquireLock(lockPath)
	if err != nil {
		t.Fatalf("failed to acquire initial lock: %v", err)
	}
	if !acquired {
		t.Fatalf("expected lock to be acquired")
	}
	defer releaseLock(f)

	// Verify lock is indeed held
	_, secondAcquired, err := acquireLock(lockPath)
	if err != nil {
		t.Fatalf("unexpected error checking lock: %v", err)
	}
	if secondAcquired {
		t.Fatalf("expected lock to be held")
	}

	// 2. Build a temporary vaultchron binary to test full subprocess execution
	binName := "vaultchron_test_bin"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(tmpDir, binName)
	buildCmd := exec.Command("go", "build", "-o", binPath, ".")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build test binary: %v, output: %s", err, string(out))
	}

	// 3. Run binary with -version while lock is held
	runCmd := exec.Command(binPath, "-version")
	runCmd.Env = append(os.Environ(), "TMPDIR="+tmpDir, "LocalAppData="+tmpDir)
	out, err := runCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected exit code 0 when running with -version while lock held, got %v; output: %s", err, string(out))
	}
	outStr := strings.TrimSpace(string(out))
	if !strings.HasPrefix(outStr, "vaultchron ") {
		t.Errorf("expected output starting with 'vaultchron ', got: %q", outStr)
	}
	if strings.Contains(outStr, "Another instance") {
		t.Errorf("output should not mention 'Another instance': %q", outStr)
	}

	// 4. Test --version as well
	runCmdDouble := exec.Command(binPath, "--version")
	runCmdDouble.Env = append(os.Environ(), "TMPDIR="+tmpDir, "LocalAppData="+tmpDir)
	outDouble, err := runCmdDouble.CombinedOutput()
	if err != nil {
		t.Fatalf("expected exit code 0 when running with --version while lock held, got %v; output: %s", err, string(outDouble))
	}
	outDoubleStr := strings.TrimSpace(string(outDouble))
	if !strings.HasPrefix(outDoubleStr, "vaultchron ") {
		t.Errorf("expected output starting with 'vaultchron ', got: %q", outDoubleStr)
	}
}

func TestRunWithArgs_FlagConflicts(t *testing.T) {
	loc := time.UTC
	fixedNow := time.Date(2026, 10, 3, 12, 0, 0, 0, loc)
	nowFunc := func() time.Time { return fixedNow }

	// Create a minimal temporary valid config file
	cfgDir := t.TempDir()
	cfgFile := filepath.Join(cfgDir, "config.yaml")
	cfgContent := `
vault:
  path: "` + filepath.ToSlash(cfgDir) + `"
scan:
  roots: ["` + filepath.ToSlash(cfgDir) + `"]
`
	if err := os.WriteFile(cfgFile, []byte(cfgContent), 0o600); err != nil {
		t.Fatalf("writing test config: %v", err)
	}

	tests := []struct {
		name       string
		args       []string
		wantSubstr string
	}{
		{
			name:       "-date with -from",
			args:       []string{"-config", cfgFile, "-date", "2026-09-30", "-from", "2026-09-20"},
			wantSubstr: "cannot combine -date",
		},
		{
			name:       "-date with -to",
			args:       []string{"-config", cfgFile, "-date", "2026-09-30", "-to", "2026-09-30"},
			wantSubstr: "cannot combine -date",
		},
		{
			name:       "-date with -window",
			args:       []string{"-config", cfgFile, "-date", "2026-09-30", "-window", "24.hours.ago"},
			wantSubstr: "cannot combine -date",
		},
		{
			name:       "-date with -catch-up",
			args:       []string{"-config", cfgFile, "-date", "2026-09-30", "-catch-up", "3"},
			wantSubstr: "cannot combine -date",
		},
		{
			name:       "-from with -window",
			args:       []string{"-config", cfgFile, "-from", "2026-09-20", "-window", "24.hours.ago"},
			wantSubstr: "cannot combine -from/-to with -window or -catch-up",
		},
		{
			name:       "-to without -from",
			args:       []string{"-config", cfgFile, "-to", "2026-09-30"},
			wantSubstr: "-to flag requires -from",
		},
		{
			name:       "-catch-up with -window",
			args:       []string{"-config", cfgFile, "-catch-up", "3", "-window", "24.hours.ago"},
			wantSubstr: "cannot combine -catch-up with -window",
		},
		{
			name:       "-catch-up 0",
			args:       []string{"-config", cfgFile, "-catch-up", "0"},
			wantSubstr: "must be at least 1",
		},
		{
			name:       "-max-calls -2",
			args:       []string{"-config", cfgFile, "-max-calls", "-2"},
			wantSubstr: "must be non-negative",
		},
		{
			name:       "future date in -date",
			args:       []string{"-config", cfgFile, "-date", "2099-01-01"},
			wantSubstr: "is in the future",
		},
		{
			name:       "-from after -to",
			args:       []string{"-config", cfgFile, "-from", "2026-10-02", "-to", "2026-09-29"},
			wantSubstr: "is after -to",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runWithArgs(tt.args, &stdout, &stderr, nowFunc, loc)
			if code == 0 {
				t.Errorf("expected non-zero exit code, got 0")
			}
			errOut := stderr.String()
			if !strings.Contains(errOut, tt.wantSubstr) {
				t.Errorf("expected stderr to contain %q, got: %q", tt.wantSubstr, errOut)
			}
		})
	}
}

func TestRunWithArgs_ValidDryRun(t *testing.T) {
	loc := time.UTC
	fixedNow := time.Date(2026, 10, 3, 12, 0, 0, 0, loc)
	nowFunc := func() time.Time { return fixedNow }

	cfgDir := t.TempDir()
	cfgFile := filepath.Join(cfgDir, "config.yaml")
	cfgContent := fmt.Sprintf(`
vault:
  path: %q
scan:
  roots: [%q]
`, filepath.ToSlash(cfgDir), filepath.ToSlash(cfgDir))
	if err := os.WriteFile(cfgFile, []byte(cfgContent), 0o600); err != nil {
		t.Fatalf("writing test config: %v", err)
	}

	runs := [][]string{
		{"-config", cfgFile, "-dry-run", "-date", "2026-09-30"},
		{"-config", cfgFile, "-dry-run", "-from", "2026-09-28", "-to", "2026-09-30"},
		{"-config", cfgFile, "-dry-run", "-catch-up", "2"},
		{"-config", cfgFile, "-dry-run", "-max-calls", "5"},
	}

	for _, args := range runs {
		var stdout, stderr bytes.Buffer
		code := runWithArgs(args, &stdout, &stderr, nowFunc, loc)
		if code != 0 {
			t.Errorf("args %v failed with code %d; stderr: %s", args, code, stderr.String())
		}
	}
}
