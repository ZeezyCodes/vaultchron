package pipeline

import (
	"testing"
	"time"

	"github.com/ZeezyCodes/vaultchron/internal/collector"
	"github.com/ZeezyCodes/vaultchron/internal/config"
	"github.com/ZeezyCodes/vaultchron/internal/vault"
)

// TestGetProjectTags verifies project tag resolution from config and fallback.
func TestGetProjectTags(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *config.Config
		projectName string
		wantSlug    string
		wantLang    string
	}{
		{
			name:        "nil config falls back to slugify and go",
			cfg:         nil,
			projectName: "MyAwesomeApp",
			wantSlug:    "myawesomeapp",
			wantLang:    "go",
		},
		{
			name: "empty project_tags falls back to slugify and go",
			cfg: &config.Config{
				ProjectTags: map[string]config.ProjectTag{},
			},
			projectName: "AcmeWidgets.com",
			wantSlug:    "acmewidgets-com",
			wantLang:    "go",
		},
		{
			name: "configured project tag takes precedence",
			cfg: &config.Config{
				ProjectTags: map[string]config.ProjectTag{
					"AcmeWidgets.com": {
						Slug: "acmewidgets",
						Lang: "go",
					},
					"Homelab": {
						Slug: "homelab",
						Lang: "docker",
					},
				},
			},
			projectName: "AcmeWidgets.com",
			wantSlug:    "acmewidgets",
			wantLang:    "go",
		},
		{
			name: "unmatched project tag falls back to slugify and go",
			cfg: &config.Config{
				ProjectTags: map[string]config.ProjectTag{
					"Homelab": {
						Slug: "homelab",
						Lang: "docker",
					},
				},
			},
			projectName: "sampleAuth",
			wantSlug:    "sampleauth",
			wantLang:    "go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slug, lang := getProjectTags(tt.cfg, tt.projectName)
			if slug != tt.wantSlug {
				t.Errorf("getProjectTags() slug = %q, want %q", slug, tt.wantSlug)
			}
			if lang != tt.wantLang {
				t.Errorf("getProjectTags() lang = %q, want %q", lang, tt.wantLang)
			}
		})
	}
}

// TestSlugify verifies slug conversion for various inputs.
func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Simple", "simple"},
		{"AcmeWidgets.com", "acmewidgets-com"},
		{"--multiple---hyphens--", "multiple-hyphens"},
		{"Special!@#Characters$%", "special-characters"},
		{"123-numbers-456", "123-numbers-456"},
	}

	for _, tt := range tests {
		got := vault.Slugify(tt.input)
		if got != tt.want {
			t.Errorf("vault.Slugify(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// TestBaseURLResolution verifies that defaultLLMBaseURL is used when cfg.LLM.BaseURL is unset,
// and cfg.LLM.BaseURL is used when provided.
func TestBaseURLResolution(t *testing.T) {
	resolveBaseURL := func(cfg *config.Config) string {
		baseURL := cfg.LLM.BaseURL
		if baseURL == "" {
			baseURL = defaultLLMBaseURL
		}
		return baseURL
	}

	// Case 1: Unset BaseURL falls back to defaultLLMBaseURL
	cfgDefault := &config.Config{
		LLM: config.LLMConfig{},
	}
	if got := resolveBaseURL(cfgDefault); got != defaultLLMBaseURL {
		t.Errorf("expected defaultLLMBaseURL %q, got %q", defaultLLMBaseURL, got)
	}

	// Case 2: Configured BaseURL is respected
	customURL := "https://api.openai.com/v1"
	cfgCustom := &config.Config{
		LLM: config.LLMConfig{
			BaseURL: customURL,
		},
	}
	if got := resolveBaseURL(cfgCustom); got != customURL {
		t.Errorf("expected customURL %q, got %q", customURL, got)
	}
}

// TestRun_AgentLogsGating verifies that HarvestAgentContext is never invoked
// when cfg.AgentLogs.Enabled is false, and is invoked when true.
func TestRun_AgentLogsGating(t *testing.T) {
	t.Run("disabled by default - no harvest call", func(t *testing.T) {
		called := false
		old := harvestAgentContextFn
		harvestAgentContextFn = func(cfg config.AgentLogsConfig, since time.Time) (*collector.AgentContext, error) {
			called = true
			return &collector.AgentContext{}, nil
		}
		defer func() { harvestAgentContextFn = old }()

		cfg := &config.Config{
			Scan: config.ScanConfig{
				Roots: []string{t.TempDir()},
			},
			AgentLogs: config.AgentLogsConfig{
				Enabled: false,
			},
		}

		_ = Run(cfg, PipelineOptions{DryRun: true})
		if called {
			t.Error("expected harvestAgentContextFn NOT to be called when AgentLogs.Enabled is false")
		}
	})

	t.Run("enabled - invokes harvest", func(t *testing.T) {
		called := false
		old := harvestAgentContextFn
		harvestAgentContextFn = func(cfg config.AgentLogsConfig, since time.Time) (*collector.AgentContext, error) {
			called = true
			return &collector.AgentContext{}, nil
		}
		defer func() { harvestAgentContextFn = old }()

		cfg := &config.Config{
			Scan: config.ScanConfig{
				Roots: []string{t.TempDir()},
			},
			AgentLogs: config.AgentLogsConfig{
				Enabled: true,
			},
		}

		_ = Run(cfg, PipelineOptions{DryRun: true})
		if !called {
			t.Error("expected harvestAgentContextFn to be called when AgentLogs.Enabled is true")
		}
	})
}
