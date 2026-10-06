package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
)

func platformPath(p string) string {
	if runtime.GOOS == "windows" {
		return filepath.Clean(p)
	}
	return p
}

func TestResolveVaultPath(t *testing.T) {
	var errOut bytes.Buffer
	// 1. Explicit vault flag
	got, err := resolveVaultPath("/explicit/vault", "", false, &errOut)
	if err != nil || got != "/explicit/vault" {
		t.Errorf("expected /explicit/vault, got %q, err: %v", got, err)
	}

	// 2. Config file provides vault path
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "custom-config.yaml")
	cfgContent := `
vault:
  path: /custom/from/config
scan:
  roots:
    - /tmp/projects
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	expectedVault := platformPath("/custom/from/config")
	got, err = resolveVaultPath("", cfgPath, false, &errOut)
	if err != nil || got != expectedVault {
		t.Errorf("expected %s, got %q, err: %v", expectedVault, got, err)
	}

	// 3. Fallback when config is missing returns error
	got, err = resolveVaultPath("", filepath.Join(tmpDir, "nonexistent.yaml"), false, &errOut)
	if err == nil {
		t.Errorf("expected error when config is missing, got %q", got)
	}
}

func TestResolveVaultPath_CallSites(t *testing.T) {
	// Isolate environment
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("VAULTCHRON_CONFIG", "")

	t.Run("real run with no config returns error and runMigrate exits 1", func(t *testing.T) {
		tDir := t.TempDir()
		t.Chdir(tDir)

		var out, errOut bytes.Buffer
		code := runMigrate("", "", "", false, &out, &errOut)
		if code != 1 {
			t.Fatalf("expected code 1 with no config, got %d", code)
		}
		if !strings.Contains(errOut.String(), "no config file found") {
			t.Errorf("expected searched paths error, got: %s", errOut.String())
		}
	})

	t.Run("dry-run with only example config succeeds", func(t *testing.T) {
		tDir := t.TempDir()
		t.Chdir(tDir)

		vaultDir := filepath.Join(tDir, "test-vault")
		_ = os.MkdirAll(vaultDir, 0o755)
		exampleCfg := filepath.Join(tDir, "config.example.yaml")
		cfgContent := "vault:\n  path: " + filepath.ToSlash(vaultDir) + "\nscan:\n  roots: [" + filepath.ToSlash(tDir) + "]\n"
		if err := os.WriteFile(exampleCfg, []byte(cfgContent), 0o644); err != nil {
			t.Fatal(err)
		}

		var out, errOut bytes.Buffer
		code := runMigrate("", "", "", true, &out, &errOut)
		if code != 0 {
			t.Fatalf("expected code 0 in dry-run with example config, got %d. stderr: %s", code, errOut.String())
		}
		if !strings.Contains(errOut.String(), "[INFO] config.yaml not found, falling back to config.example.yaml") {
			t.Errorf("expected [INFO] fallback message, got: %s", errOut.String())
		}
	})

	t.Run("real run with only example config fails", func(t *testing.T) {
		tDir := t.TempDir()
		t.Chdir(tDir)

		vaultDir := filepath.Join(tDir, "test-vault")
		_ = os.MkdirAll(vaultDir, 0o755)
		exampleCfg := filepath.Join(tDir, "config.example.yaml")
		cfgContent := "vault:\n  path: " + filepath.ToSlash(vaultDir) + "\nscan:\n  roots: [" + filepath.ToSlash(tDir) + "]\n"
		if err := os.WriteFile(exampleCfg, []byte(cfgContent), 0o644); err != nil {
			t.Fatal(err)
		}

		var out, errOut bytes.Buffer
		code := runMigrate("", "", "", false, &out, &errOut)
		if code != 1 {
			t.Fatalf("expected code 1 in real run with only example config, got %d", code)
		}
	})
}

func TestRunMigrate_SingleFileDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.md")
	legacyContent := "## Telemetry\n- commit 12345\n\n## Architecture\nsome arch notes"
	if err := os.WriteFile(testFile, []byte(legacyContent), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	var out, errOut bytes.Buffer
	code := runMigrate("", "", testFile, true, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected code 0, got %d. stderr: %s", code, errOut.String())
	}

	outStr := out.String()
	if !strings.Contains(outStr, "Dry Run Preview") {
		t.Errorf("expected Dry Run Preview in stdout, got: %s", outStr)
	}
	if !strings.Contains(outStr, "[!note]- 📊 Session Telemetry") {
		t.Errorf("expected v3 callout in preview, got: %s", outStr)
	}

	// Verify file was NOT modified in dry-run
	data, _ := os.ReadFile(testFile)
	if string(data) != legacyContent {
		t.Errorf("file modified during dry run")
	}
}

func TestRunMigrate_SingleFileDryRun_MissingFile(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runMigrate("", "", "/nonexistent/path/note.md", true, &out, &errOut)
	if code != 1 {
		t.Fatalf("expected exit code 1 on missing file, got %d", code)
	}
	if !strings.Contains(errOut.String(), "error reading file") {
		t.Errorf("expected error reading file in stderr, got: %s", errOut.String())
	}
}

func TestRunMigrate_SingleFileActual(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.md")
	legacyContent := "## Telemetry\n- commit 12345\n\n## Architecture\nsome arch notes"
	if err := os.WriteFile(testFile, []byte(legacyContent), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// First run: should migrate
	var out1, errOut1 bytes.Buffer
	code1 := runMigrate("", "", testFile, false, &out1, &errOut1)
	if code1 != 0 {
		t.Fatalf("expected code 0, got %d. stderr: %s", code1, errOut1.String())
	}
	if !strings.Contains(out1.String(), "Migrated:") {
		t.Errorf("expected 'Migrated:' in stdout, got: %s", out1.String())
	}

	// Verify file was transformed
	migratedData, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read migrated file: %v", err)
	}
	if !strings.Contains(string(migratedData), "[!note]- 📊 Session Telemetry") {
		t.Errorf("expected v3 callout in migrated content: %s", string(migratedData))
	}

	// Second run: already up to date
	var out2, errOut2 bytes.Buffer
	code2 := runMigrate("", "", testFile, false, &out2, &errOut2)
	if code2 != 0 {
		t.Fatalf("expected code 0 on second run, got %d. stderr: %s", code2, errOut2.String())
	}
	if !strings.Contains(out2.String(), "Already up to date:") {
		t.Errorf("expected 'Already up to date:' in stdout, got: %s", out2.String())
	}
}

func TestRunMigrate_SingleFile_InvalidPath(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runMigrate("", "", "/nonexistent/test.md", false, &out, &errOut)
	if code != 1 {
		t.Fatalf("expected code 1 on non-existent file, got %d", code)
	}
	if !strings.Contains(errOut.String(), "error migrating file") {
		t.Errorf("expected error migrating file in stderr, got: %s", errOut.String())
	}
}

func TestRunMigrate_VaultDryRunAndActual(t *testing.T) {
	vaultDir := t.TempDir()
	devlogDir := filepath.Join(vaultDir, "Projects", "SampleRepo", "Devlog")
	if err := os.MkdirAll(devlogDir, 0o755); err != nil {
		t.Fatalf("failed to create devlog dir: %v", err)
	}

	notePath := filepath.Join(devlogDir, "2026-09-28.md")
	legacyContent := "## Telemetry\n- commit 12345\n\n## Architecture\nsome arch notes"
	if err := os.WriteFile(notePath, []byte(legacyContent), 0o644); err != nil {
		t.Fatalf("failed to write test devlog: %v", err)
	}

	// 1. Dry run vault
	var outDry, errOutDry bytes.Buffer
	codeDry := runMigrate(vaultDir, "", "", true, &outDry, &errOutDry)
	if codeDry != 0 {
		t.Fatalf("expected code 0 in vault dry run, got %d. stderr: %s", codeDry, errOutDry.String())
	}
	if !strings.Contains(outDry.String(), "Found 1 devlog notes") {
		t.Errorf("expected 1 devlog notes found, got: %s", outDry.String())
	}
	if !strings.Contains(outDry.String(), "[PENDING]") {
		t.Errorf("expected [PENDING] note, got: %s", outDry.String())
	}
	if !strings.Contains(outDry.String(), "1/1 files would be modified") {
		t.Errorf("expected 1/1 files would be modified, got: %s", outDry.String())
	}

	// 2. Actual vault migration
	var outAct, errOutAct bytes.Buffer
	codeAct := runMigrate(vaultDir, "", "", false, &outAct, &errOutAct)
	if codeAct != 0 {
		t.Fatalf("expected code 0 in vault migration, got %d. stderr: %s", codeAct, errOutAct.String())
	}
	if !strings.Contains(outAct.String(), "Successfully migrated 1 devlog notes") {
		t.Errorf("expected migration success message, got: %s", outAct.String())
	}

	// Verify note was actually modified
	migrated, _ := os.ReadFile(notePath)
	if !strings.Contains(string(migrated), "[!note]- 📊 Session Telemetry") {
		t.Errorf("expected v3 callout in migrated note")
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
		{"with preceding flags", []string{"-vault", "/tmp", "-version"}, true},
		{"with following flags", []string{"-version", "-dry-run"}, true},
		{"no version flag", []string{"-dry-run"}, false},
		{"empty args", []string{}, false},
		{"bare double dash terminates search", []string{"--", "-version"}, false},
		{"flag value is not version flag", []string{"-file", "version"}, false},
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

func TestRunMigrate_CustomConfig(t *testing.T) {
	tDir := t.TempDir()
	vaultDir := filepath.Join(tDir, "vault")
	devlogDir := filepath.Join(vaultDir, "Dev", "Projects", "SampleRepo", "Devlog")
	if err := os.MkdirAll(devlogDir, 0o755); err != nil {
		t.Fatalf("failed to create devlog dir: %v", err)
	}

	notePath := filepath.Join(devlogDir, "2026-09-28.md")
	legacyContent := "## Telemetry\n- commit 12345\n\n## Architecture\nsome arch notes"
	if err := os.WriteFile(notePath, []byte(legacyContent), 0o644); err != nil {
		t.Fatalf("failed to write test devlog: %v", err)
	}

	cfgPath := filepath.Join(tDir, "config.yaml")
	cfgContent := "vault:\n  path: " + filepath.ToSlash(vaultDir) + "\n  projects_dir: Dev/Projects\n  index_file: Meta/Dev-Index.md\nscan:\n  roots: [" + filepath.ToSlash(tDir) + "]\n"
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// 1. Dry-run finds note in custom projects dir
	var outDry, errOutDry bytes.Buffer
	codeDry := runMigrate(vaultDir, cfgPath, "", true, &outDry, &errOutDry)
	if codeDry != 0 {
		t.Fatalf("expected code 0, got %d. stderr: %s", codeDry, errOutDry.String())
	}
	if !strings.Contains(outDry.String(), "Found 1 devlog notes") {
		t.Errorf("expected 1 devlog note found in custom dir, got: %s", outDry.String())
	}

	// 2. Real migration updates note with custom breadcrumbs
	var outAct, errOutAct bytes.Buffer
	codeAct := runMigrate(vaultDir, cfgPath, "", false, &outAct, &errOutAct)
	if codeAct != 0 {
		t.Fatalf("expected code 0, got %d. stderr: %s", codeAct, errOutAct.String())
	}
	migratedBytes, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("reading migrated note: %v", err)
	}
	expectedBreadcrumb := "[[Meta/Dev-Index|🏠 Index]] / [[Dev/Projects/SampleRepo/Overview|SampleRepo]]"
	if !strings.Contains(string(migratedBytes), expectedBreadcrumb) {
		t.Errorf("expected breadcrumb %q, got:\n%s", expectedBreadcrumb, string(migratedBytes))
	}
}
