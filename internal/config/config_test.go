package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// platformPath returns filepath.Clean(p) on Windows and p unchanged on other platforms.
func platformPath(p string) string {
	if runtime.GOOS == "windows" {
		return filepath.Clean(p)
	}
	return p
}

// TestDefaultConfig verifies that DefaultConfig produces sensible generic defaults
// based on the environment and standard locations.
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig() returned nil")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("skipping home dir check since UserHomeDir failed")
	}

	expectedVault := filepath.Join(home, "vault")
	if cfg.Vault.Path != expectedVault {
		t.Errorf("expected Vault.Path %q, got %q", expectedVault, cfg.Vault.Path)
	}

	if len(cfg.Scan.Roots) != 1 {
		t.Fatalf("expected 1 scan root, got %d", len(cfg.Scan.Roots))
	}
	expectedRoot := filepath.Join(home, "projects")
	if cfg.Scan.Roots[0] != expectedRoot {
		t.Errorf("expected Scan.Roots[0] %q, got %q", expectedRoot, cfg.Scan.Roots[0])
	}

	if cfg.LLM.BaseURL != DefaultLLMBaseURL {
		t.Errorf("expected LLM.BaseURL %q, got %q", DefaultLLMBaseURL, cfg.LLM.BaseURL)
	}

	if len(cfg.LLM.Waterfall) == 0 {
		t.Error("LLM.Waterfall is empty")
	}

	expectedAntigravity := filepath.Join(home, ".antigravity")
	if cfg.AgentLogs.AntigravityPath != expectedAntigravity {
		t.Errorf("expected AntigravityPath %q, got %q", expectedAntigravity, cfg.AgentLogs.AntigravityPath)
	}

	expectedPoolside := filepath.Join(home, ".poolside")
	if cfg.AgentLogs.PoolsidePath != expectedPoolside {
		t.Errorf("expected PoolsidePath %q, got %q", expectedPoolside, cfg.AgentLogs.PoolsidePath)
	}

	if cfg.AgentLogs.Enabled {
		t.Error("expected AgentLogs.Enabled to default to false")
	}

	if cfg.ProjectTags == nil {
		t.Error("ProjectTags map should be initialized")
	}
	if len(cfg.ProjectTags) != 0 {
		t.Errorf("expected default ProjectTags to be empty, got %d items", len(cfg.ProjectTags))
	}
}

// TestLoad_ExpandPaths verifies that Load parses YAML and expands ~ and environment variables.
func TestLoad_ExpandPaths(t *testing.T) {
	yamlContent := `
vault:
  path: ~/test-vault
  index_file: Index.md
  rollups_dir: Rollups
  projects_dir: Projects
  recent_days: 5

scan:
  roots:
    - ~/test-projects
  max_depth: 2
  excludes:
    - vendor

llm:
  base_url: https://api.openai.com/v1
  waterfall:
    - gpt-4o
    - gpt-4o-mini
  api_key_env: OPENAI_API_KEY

project_tags:
  CustomProject:
    slug: custom-proj
    lang: python

agent_logs:
  antigravity_path: ~/test-agents
  poolside_path: ~/test-poolside
`
	tmpFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed writing temporary config: %v", err)
	}

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("skipping home dir check since UserHomeDir failed")
	}

	expectedVault := filepath.Join(home, "test-vault")
	if cfg.Vault.Path != expectedVault {
		t.Errorf("expected Vault.Path %q, got %q", expectedVault, cfg.Vault.Path)
	}

	expectedRoot := filepath.Join(home, "test-projects")
	if len(cfg.Scan.Roots) != 1 || cfg.Scan.Roots[0] != expectedRoot {
		t.Errorf("expected Scan.Roots[0] %q, got %v", expectedRoot, cfg.Scan.Roots)
	}

	if cfg.LLM.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("expected BaseURL https://api.openai.com/v1, got %q", cfg.LLM.BaseURL)
	}

	tag, ok := cfg.ProjectTags["CustomProject"]
	if !ok {
		t.Fatal("CustomProject tag not found in ProjectTags")
	}
	if tag.Slug != "custom-proj" || tag.Lang != "python" {
		t.Errorf("unexpected ProjectTag: %+v", tag)
	}
}

// TestExpandPath verifies tilde and environment variable expansions.
func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("UserHomeDir unavailable")
	}

	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"~", filepath.Clean(home)},
		{"~/vault", filepath.Join(home, "vault")},
		{"$HOME/vault", filepath.Join(home, "vault")},
		{"/static/path", platformPath("/static/path")},
		{"$HOME", home},
		{"${HOME}/vault", filepath.Join(home, "vault")},
		{"~/", home},
		{"$UNSET_XYZ/x", platformPath("/x")},
	}

	for _, tt := range tests {
		got := ExpandPath(tt.input)
		if got != tt.expected {
			t.Errorf("ExpandPath(%q) = %q; want %q", tt.input, got, tt.expected)
		}
	}

	t.Run("env priority", func(t *testing.T) {
		custom := t.TempDir()
		t.Setenv("HOME", custom)
		got := ExpandPath("$HOME/x")
		want := filepath.Join(custom, "x")
		if got != want {
			t.Errorf("ExpandPath(\"$HOME/x\") = %q; want %q", got, want)
		}
	})

	t.Run("windows fallback and backslash", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("skipping Windows-only test on non-windows platform")
		}
		t.Setenv("HOME", "")
		gotHome := ExpandPath("$HOME/vault")
		wantHome := filepath.Join(home, "vault")
		if gotHome != wantHome {
			t.Errorf("ExpandPath(\"$HOME/vault\") = %q; want %q", gotHome, wantHome)
		}
		gotBackslash := ExpandPath(`~\vault`)
		wantBackslash := filepath.Join(home, "vault")
		if gotBackslash != wantBackslash {
			t.Errorf("ExpandPath(`~\\vault`) = %q; want %q", gotBackslash, wantBackslash)
		}
	})

	t.Run("non-windows backslash unchanged", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("skipping non-Windows test on Windows")
		}
		got := ExpandPath(`~\vault`)
		want := `~\vault`
		if got != want {
			t.Errorf("ExpandPath(`~\\vault`) = %q; want %q", got, want)
		}
	})
}

// TestLoad_DefaultsOmittedMaxDepth verifies that omitting max_depth in YAML gets the default value.
func TestLoad_DefaultsOmittedMaxDepth(t *testing.T) {
	yamlContent := `
vault:
  path: ~/test-vault
scan:
  roots:
    - ~/test-projects
`
	tmpFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed writing temporary config: %v", err)
	}

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Scan.MaxDepth != DefaultConfig().Scan.MaxDepth {
		t.Errorf("expected Scan.MaxDepth %d, got %d", DefaultConfig().Scan.MaxDepth, cfg.Scan.MaxDepth)
	}
	if cfg.Vault.RecentDays != DefaultConfig().Vault.RecentDays {
		t.Errorf("expected Vault.RecentDays %d, got %d", DefaultConfig().Vault.RecentDays, cfg.Vault.RecentDays)
	}
	if cfg.LLM.BaseURL != DefaultLLMBaseURL {
		t.Errorf("expected default LLM.BaseURL %q, got %q", DefaultLLMBaseURL, cfg.LLM.BaseURL)
	}
	if len(cfg.LLM.Waterfall) == 0 {
		t.Error("expected default LLM.Waterfall to be populated")
	}
}

// TestLoad_EmptyVaultPathError verifies that a config with empty vault.path returns an error.
func TestLoad_EmptyVaultPathError(t *testing.T) {
	yamlContent := `
scan:
  roots:
    - ~/test-projects
`
	tmpFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed writing temporary config: %v", err)
	}

	_, err := Load(tmpFile)
	if err == nil {
		t.Fatal("expected error for empty vault.path, got nil")
	}
}

// TestLoad_EmptyScanRootsError verifies that a config with empty scan.roots returns an error.
func TestLoad_EmptyScanRootsError(t *testing.T) {
	yamlContent := `
vault:
  path: ~/test-vault
scan:
  roots: []
`
	tmpFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed writing temporary config: %v", err)
	}

	_, err := Load(tmpFile)
	if err == nil {
		t.Fatal("expected error for empty scan.roots, got nil")
	}
}

// TestLoad_InvalidMaxDepthError verifies that max_depth < 1 returns an error.
func TestLoad_InvalidMaxDepthError(t *testing.T) {
	yamlContent := `
vault:
  path: ~/test-vault
scan:
  roots:
    - ~/test-projects
  max_depth: -1
`
	tmpFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed writing temporary config: %v", err)
	}

	_, err := Load(tmpFile)
	if err == nil {
		t.Fatal("expected error for max_depth < 1, got nil")
	}
}

// TestLoad_MinimalValidConfig verifies that minimal required fields load successfully.
func TestLoad_MinimalValidConfig(t *testing.T) {
	yamlContent := `
vault:
  path: /tmp/test-vault
scan:
  roots:
    - /tmp/test-projects
`
	tmpFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed writing temporary config: %v", err)
	}

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("Load failed for minimal config: %v", err)
	}
	expectedVault := platformPath("/tmp/test-vault")
	if cfg.Vault.Path != expectedVault {
		t.Errorf("expected Vault.Path %q, got %q", expectedVault, cfg.Vault.Path)
	}
	expectedRoot := platformPath("/tmp/test-projects")
	if len(cfg.Scan.Roots) != 1 || cfg.Scan.Roots[0] != expectedRoot {
		t.Errorf("expected Scan.Roots [%s], got %v", expectedRoot, cfg.Scan.Roots)
	}
	if cfg.Scan.MaxDepth != 3 {
		t.Errorf("expected defaulted MaxDepth 3, got %d", cfg.Scan.MaxDepth)
	}
}

// TestResolveConfigPath verifies resolution order: explicit > config.yaml > config.example.yaml.
func TestResolveConfigPath(t *testing.T) {
	// Explicit path should always be returned as-is
	if got := ResolveConfigPath("/custom/path.yaml"); got != "/custom/path.yaml" {
		t.Errorf("expected /custom/path.yaml, got %q", got)
	}

	// Change working directory to a clean temp dir to test fallbacks
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current wd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir to tempDir: %v", err)
	}
	defer func() {
		_ = os.Chdir(origWd)
	}()

	// Neither file exists: defaults to "config.yaml"
	if got := ResolveConfigPath(""); got != "config.yaml" {
		t.Errorf("expected config.yaml when neither exists, got %q", got)
	}

	// Only config.example.yaml exists
	if err := os.WriteFile("config.example.yaml", []byte(""), 0o644); err != nil {
		t.Fatalf("failed to write config.example.yaml: %v", err)
	}
	if got := ResolveConfigPath(""); got != "config.example.yaml" {
		t.Errorf("expected config.example.yaml, got %q", got)
	}

	// Both config.yaml and config.example.yaml exist: prefers config.yaml
	if err := os.WriteFile("config.yaml", []byte(""), 0o644); err != nil {
		t.Fatalf("failed to write config.yaml: %v", err)
	}
	if got := ResolveConfigPath(""); got != "config.yaml" {
		t.Errorf("expected config.yaml over config.example.yaml, got %q", got)
	}
}
