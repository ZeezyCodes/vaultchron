package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZeezyCodes/vaultchron/internal/config"
)

func TestResolveVaultPath(t *testing.T) {
	// 1. Explicit vault flag
	if got := resolveVaultPath("/explicit/vault", ""); got != "/explicit/vault" {
		t.Errorf("expected /explicit/vault, got %q", got)
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
	if got := resolveVaultPath("", cfgPath); got != "/custom/from/config" {
		t.Errorf("expected /custom/from/config, got %q", got)
	}

	// 3. Fallback when config is missing
	expectedDefault := config.DefaultConfig().Vault.Path
	if got := resolveVaultPath("", filepath.Join(tmpDir, "nonexistent.yaml")); got != expectedDefault {
		t.Errorf("expected default %q, got %q", expectedDefault, got)
	}
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
