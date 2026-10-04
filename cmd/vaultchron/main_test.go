package main

import (
	"bytes"
	"fmt"
	"io"
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

func setupTestGitRepo(t *testing.T) string {
	t.Helper()
	repoDir := filepath.Join(t.TempDir(), "testrepo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("creating repo dir: %v", err)
	}
	emptyConfig := filepath.Join(t.TempDir(), ".emptyconfig")
	if err := os.WriteFile(emptyConfig, []byte(""), 0o600); err != nil {
		t.Fatalf("creating empty git config: %v", err)
	}

	runGit := func(envDates []string, args ...string) {
		t.Helper()
		cmdArgs := append([]string{"-C", repoDir}, args...)
		cmd := exec.Command("git", cmdArgs...)
		cmd.Env = append(os.Environ(),
			"GIT_CONFIG_NOSYSTEM=1",
			"GIT_CONFIG_GLOBAL="+emptyConfig,
			"GIT_AUTHOR_NAME=Test User",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test User",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		if len(envDates) > 0 {
			cmd.Env = append(cmd.Env, envDates...)
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v: %s", strings.Join(args, " "), err, string(out))
		}
	}

	runGit(nil, "init", "-b", "main")

	days := []string{"2026-09-29", "2026-09-30"}
	for _, d := range days {
		fPath := filepath.Join(repoDir, fmt.Sprintf("file_%s.txt", d))
		if err := os.WriteFile(fPath, []byte("content on "+d+"\n"), 0o644); err != nil {
			t.Fatalf("writing file: %v", err)
		}
		runGit(nil, "add", filepath.Base(fPath))
		runGit([]string{
			fmt.Sprintf("GIT_AUTHOR_DATE=%sT12:00:00-05:00", d),
			fmt.Sprintf("GIT_COMMITTER_DATE=%sT12:00:00-05:00", d),
		}, "commit", "-m", "Commit on "+d)
	}

	return repoDir
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	outChan := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		outChan <- buf.String()
	}()

	fn()

	_ = w.Close()
	os.Stdout = oldStdout
	out := <-outChan
	_ = r.Close()
	return out
}

func TestRunWithArgs_DayMode(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*3600)
	fixedNow := time.Date(2026, 10, 3, 12, 0, 0, 0, loc)
	nowFunc := func() time.Time { return fixedNow }

	repoDir := setupTestGitRepo(t)
	scanRoot := filepath.Dir(repoDir)
	vaultDir := filepath.Join(t.TempDir(), "vault")
	if err := os.MkdirAll(vaultDir, 0o755); err != nil {
		t.Fatalf("creating vault dir: %v", err)
	}

	cfgDir := t.TempDir()
	cfgFile := filepath.Join(cfgDir, "config.yaml")
	cfgContent := fmt.Sprintf(`
vault:
  path: %q
scan:
  roots: [%q]
`, filepath.ToSlash(vaultDir), filepath.ToSlash(scanRoot))
	if err := os.WriteFile(cfgFile, []byte(cfgContent), 0o600); err != nil {
		t.Fatalf("writing test config: %v", err)
	}

	assertNoVaultWrites := func(label string) {
		t.Helper()
		entries, err := os.ReadDir(vaultDir)
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("%s: reading vault dir: %v", label, err)
		}
		if len(entries) != 0 {
			t.Errorf("%s: expected vault dir to be empty, found %d entries", label, len(entries))
		}
	}

	// 1. -dry-run -from 2026-09-29 -to 2026-09-30 prints two WOULD WRITE rows and writes nothing under vault dir
	{
		var stdout, stderr bytes.Buffer
		args := []string{"-config", cfgFile, "-dry-run", "-from", "2026-09-29", "-to", "2026-09-30"}
		var code int
		out := captureStdout(t, func() {
			code = runWithArgs(args, &stdout, &stderr, nowFunc, loc)
		})
		if code != 0 {
			t.Fatalf("-dry-run -from -to failed with exit code %d, stderr: %s", code, stderr.String())
		}
		if strings.Count(out, "WOULD WRITE") != 2 {
			t.Errorf("expected 2 WOULD WRITE rows, got:\n%s", out)
		}
		assertNoVaultWrites("-dry-run -from -to")
	}

	// 2. -scan -from 2026-09-29 -to 2026-09-30 prints a Repository/Date/Commits table with both days and makes no vault writes
	{
		var stdout, stderr bytes.Buffer
		args := []string{"-config", cfgFile, "-scan", "-from", "2026-09-29", "-to", "2026-09-30"}
		var code int
		out := captureStdout(t, func() {
			code = runWithArgs(args, &stdout, &stderr, nowFunc, loc)
		})
		if code != 0 {
			t.Fatalf("-scan -from -to failed with exit code %d, stderr: %s", code, stderr.String())
		}
		if !strings.Contains(out, "Repository") || !strings.Contains(out, "Date") || !strings.Contains(out, "Commits") {
			t.Errorf("expected table header Repository/Date/Commits, got:\n%s", out)
		}
		if !strings.Contains(out, "2026-09-29") || !strings.Contains(out, "2026-09-30") {
			t.Errorf("expected table to contain both days 2026-09-29 and 2026-09-30, got:\n%s", out)
		}
		assertNoVaultWrites("-scan -from -to")
	}

	// 3. -dry-run -date <a day with no commits> prints 0 would be written and 1 repo-days had no commits
	{
		var stdout, stderr bytes.Buffer
		args := []string{"-config", cfgFile, "-dry-run", "-date", "2026-09-28"}
		var code int
		out := captureStdout(t, func() {
			code = runWithArgs(args, &stdout, &stderr, nowFunc, loc)
		})
		if code != 0 {
			t.Fatalf("-dry-run -date (no commits) failed with exit code %d, stderr: %s", code, stderr.String())
		}
		if !strings.Contains(out, "0 would be written") {
			t.Errorf("expected output to contain '0 would be written', got:\n%s", out)
		}
		if !strings.Contains(out, "1 repo-days had no commits") {
			t.Errorf("expected output to contain '1 repo-days had no commits', got:\n%s", out)
		}
		assertNoVaultWrites("-dry-run -date (no commits)")
	}
}

func TestConfigResolutionCLI(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*3600)
	fixedNow := time.Date(2026, 10, 3, 12, 0, 0, 0, loc)
	nowFunc := func() time.Time { return fixedNow }

	setupEnvAndDirs := func(t *testing.T) (string, string) {
		t.Helper()
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)
		t.Setenv("USERPROFILE", tempHome)
		t.Setenv("XDG_CONFIG_HOME", tempHome)
		t.Setenv("APPDATA", tempHome)
		t.Setenv("VAULTCHRON_CONFIG", "")

		workDir := t.TempDir()
		t.Chdir(workDir)
		return tempHome, workDir
	}

	writeMinimalConfig := func(t *testing.T, path, vaultPath, repoDir string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		content := fmt.Sprintf("vault:\n  path: %q\nscan:\n  roots: [%q]\n", filepath.ToSlash(vaultPath), filepath.ToSlash(repoDir))
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("real run with no config fails with searched paths and vault untouched", func(t *testing.T) {
		_, workDir := setupEnvAndDirs(t)
		vaultDir := filepath.Join(workDir, "vault")
		_ = os.MkdirAll(vaultDir, 0o755)

		var stdout, stderr bytes.Buffer
		code := runWithArgs([]string{"-date", "2026-10-02"}, &stdout, &stderr, nowFunc, loc)
		if code != 1 {
			t.Fatalf("expected code 1, got %d", code)
		}
		errStr := stderr.String()
		if !strings.Contains(errStr, "no config file found; looked in this order:") {
			t.Errorf("expected searched paths message in stderr, got: %s", errStr)
		}
		entries, _ := os.ReadDir(vaultDir)
		if len(entries) != 0 {
			t.Errorf("expected vault dir untouched, found %d entries", len(entries))
		}
	})

	t.Run("dry-run and scan with only config.example.yaml in cwd succeeds with INFO line", func(t *testing.T) {
		_, workDir := setupEnvAndDirs(t)
		repoDir := setupTestGitRepo(t)
		vaultDir := filepath.Join(workDir, "vault")
		_ = os.MkdirAll(vaultDir, 0o755)

		examplePath := filepath.Join(workDir, "config.example.yaml")
		writeMinimalConfig(t, examplePath, vaultDir, repoDir)

		// 1. -dry-run
		var stdout1, stderr1 bytes.Buffer
		code1 := runWithArgs([]string{"-dry-run", "-date", "2026-10-02"}, &stdout1, &stderr1, nowFunc, loc)
		if code1 != 0 {
			t.Fatalf("dry-run failed with code %d: %s", code1, stderr1.String())
		}
		if !strings.Contains(stderr1.String(), "[INFO] config.yaml not found, falling back to config.example.yaml") {
			t.Errorf("expected INFO fallback line in stderr, got: %s", stderr1.String())
		}

		// 2. -scan
		var stdout2, stderr2 bytes.Buffer
		code2 := runWithArgs([]string{"-scan"}, &stdout2, &stderr2, nowFunc, loc)
		if code2 != 0 {
			t.Fatalf("scan failed with code %d: %s", code2, stderr2.String())
		}
		if !strings.Contains(stderr2.String(), "[INFO] config.yaml not found, falling back to config.example.yaml") {
			t.Errorf("expected INFO fallback line in stderr, got: %s", stderr2.String())
		}
	})

	t.Run("migrate-vault with only an example config fails", func(t *testing.T) {
		_, workDir := setupEnvAndDirs(t)
		vaultDir := filepath.Join(workDir, "vault")
		_ = os.MkdirAll(vaultDir, 0o755)
		examplePath := filepath.Join(workDir, "config.example.yaml")
		writeMinimalConfig(t, examplePath, vaultDir, workDir)

		var stdout, stderr bytes.Buffer
		code := runWithArgs([]string{"-migrate-vault"}, &stdout, &stderr, nowFunc, loc)
		if code != 1 {
			t.Fatalf("expected code 1 for migrate-vault with example config, got %d", code)
		}
		if !strings.Contains(stderr.String(), "no config file found") {
			t.Errorf("expected no config file found error, got: %s", stderr.String())
		}
	})

	t.Run("per-user config found with no -config flag uses it and prints INFO", func(t *testing.T) {
		tempHome, workDir := setupEnvAndDirs(t)
		repoDir := setupTestGitRepo(t)
		vaultDir := filepath.Join(workDir, "vault")
		_ = os.MkdirAll(vaultDir, 0o755)

		userCfgPath := filepath.Join(tempHome, "vaultchron", "config.yaml")
		writeMinimalConfig(t, userCfgPath, vaultDir, repoDir)

		var stdout, stderr bytes.Buffer
		code := runWithArgs([]string{"-dry-run", "-date", "2026-10-02"}, &stdout, &stderr, nowFunc, loc)
		if code != 0 {
			t.Fatalf("expected code 0, got %d. stderr: %s", code, stderr.String())
		}
		if !strings.Contains(stderr.String(), "[INFO] using config "+userCfgPath) {
			t.Errorf("expected [INFO] using config %s in stderr, got: %s", userCfgPath, stderr.String())
		}
	})

	t.Run("VAULTCHRON_CONFIG beats ./config.yaml", func(t *testing.T) {
		_, workDir := setupEnvAndDirs(t)
		repoDir := setupTestGitRepo(t)
		vaultDir := filepath.Join(workDir, "vault")
		_ = os.MkdirAll(vaultDir, 0o755)

		cwdCfg := filepath.Join(workDir, "config.yaml")
		writeMinimalConfig(t, cwdCfg, vaultDir, repoDir)

		envCfgDir := t.TempDir()
		envCfg := filepath.Join(envCfgDir, "env-config.yaml")
		writeMinimalConfig(t, envCfg, vaultDir, repoDir)
		t.Setenv("VAULTCHRON_CONFIG", envCfg)

		var stdout, stderr bytes.Buffer
		code := runWithArgs([]string{"-dry-run", "-date", "2026-10-02"}, &stdout, &stderr, nowFunc, loc)
		if code != 0 {
			t.Fatalf("expected code 0, got %d. stderr: %s", code, stderr.String())
		}
		if !strings.Contains(stderr.String(), "[INFO] using config "+envCfg) {
			t.Errorf("expected [INFO] using config %s in stderr, got: %s", envCfg, stderr.String())
		}
	})

	t.Run("VAULTCHRON_CONFIG pointing at a missing file fails", func(t *testing.T) {
		_, workDir := setupEnvAndDirs(t)
		missingCfg := filepath.Join(workDir, "missing.yaml")
		t.Setenv("VAULTCHRON_CONFIG", missingCfg)

		var stdout, stderr bytes.Buffer
		code := runWithArgs([]string{"-dry-run", "-date", "2026-10-02"}, &stdout, &stderr, nowFunc, loc)
		if code != 1 {
			t.Fatalf("expected code 1, got %d", code)
		}
		if !strings.Contains(stderr.String(), "config file "+missingCfg+" (from $VAULTCHRON_CONFIG) not found") {
			t.Errorf("expected missing env config error in stderr, got: %s", stderr.String())
		}
	})
}
