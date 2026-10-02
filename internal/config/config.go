package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultLLMBaseURL is the default OpenAI-compatible API endpoint (Google Gemini).
const DefaultLLMBaseURL = "https://generativelanguage.googleapis.com/v1beta/openai"

// VaultConfig defines paths and settings for the Obsidian vault.
type VaultConfig struct {
	Path        string `yaml:"path"`
	IndexFile   string `yaml:"index_file"`
	RollupsDir  string `yaml:"rollups_dir"`
	ProjectsDir string `yaml:"projects_dir"`
	RecentDays  int    `yaml:"recent_days"`
}

// ScanConfig defines repository discovery scan parameters.
type ScanConfig struct {
	Roots    []string `yaml:"roots"`
	MaxDepth int      `yaml:"max_depth"`
	Excludes []string `yaml:"excludes"`
}

// LLMConfig defines the language model provider, base URL, and waterfall fallback chain.
type LLMConfig struct {
	Provider  string   `yaml:"provider,omitempty"`
	BaseURL   string   `yaml:"base_url,omitempty"`
	Waterfall []string `yaml:"waterfall"`
	APIKeyEnv string   `yaml:"api_key_env"`
}

// AgentLogsConfig defines paths to agent session log directories and controls
// whether session logs are harvested.
type AgentLogsConfig struct {
	Enabled         bool   `yaml:"enabled"`
	AntigravityPath string `yaml:"antigravity_path"`
	PoolsidePath    string `yaml:"poolside_path"`
}

// ProjectTag defines slug and language metadata for a vault project.
type ProjectTag struct {
	Slug string `yaml:"slug"`
	Lang string `yaml:"lang"`
}

// Config is the top-level configuration struct deserialized from YAML.
type Config struct {
	Vault       VaultConfig           `yaml:"vault"`
	Scan        ScanConfig            `yaml:"scan"`
	LLM         LLMConfig             `yaml:"llm"`
	AgentLogs   AgentLogsConfig       `yaml:"agent_logs"`
	ProjectTags map[string]ProjectTag `yaml:"project_tags,omitempty"`
}

// ExpandPath expands leading ~ to the user's home directory and expands
// environment variables in the path. If $HOME is referenced but unset,
// it falls back to os.UserHomeDir().
func ExpandPath(p string) string {
	if p == "" {
		return ""
	}
	if p == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			p = home
		}
	} else if strings.HasPrefix(p, "~/") || (runtime.GOOS == "windows" && strings.HasPrefix(p, `~\`)) {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, p[2:])
		}
	}

	expanded := os.Expand(p, func(k string) string {
		if k == "HOME" && os.Getenv("HOME") == "" {
			if home, err := os.UserHomeDir(); err == nil {
				return home
			}
		}
		return os.Getenv(k)
	})

	if runtime.GOOS == "windows" && expanded != "" {
		return filepath.Clean(expanded)
	}
	return expanded
}

// ResolveConfigPath returns explicit if non-empty; otherwise, it checks if "config.yaml" exists,
// then "config.example.yaml", falling back to "config.yaml" if neither exists.
func ResolveConfigPath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if _, err := os.Stat("config.yaml"); err == nil {
		return "config.yaml"
	}
	if _, err := os.Stat("config.example.yaml"); err == nil {
		return "config.example.yaml"
	}
	return "config.yaml"
}

// Load reads, parses, applies defaults, and validates a YAML config file into a Config struct.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %q: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing YAML config: %w", err)
	}

	def := DefaultConfig()

	// Merge missing or zero fields with DefaultConfig() defaults:
	// - Scan.MaxDepth (defaults to 3 if zero/omitted)
	// - Scan.Excludes (defaults to standard ignore list if empty)
	// - Vault.IndexFile (defaults to "00-Dev-Index.md" if empty)
	// - Vault.RollupsDir (defaults to "Daily-Rollups" if empty)
	// - Vault.ProjectsDir (defaults to "Projects" if empty)
	// - Vault.RecentDays (defaults to 7 if <= 0)
	// - LLM.Provider (defaults to "gemini" if empty)
	// - LLM.BaseURL (defaults to DefaultLLMBaseURL if empty)
	// - LLM.Waterfall (defaults to standard models if empty)
	// - LLM.APIKeyEnv (defaults to "GOOGLE_API_KEY" if empty)
	// - AgentLogs.AntigravityPath (defaults to ~/.antigravity if empty)
	// - AgentLogs.PoolsidePath (defaults to ~/.poolside if empty)
	if cfg.Scan.MaxDepth == 0 {
		cfg.Scan.MaxDepth = def.Scan.MaxDepth
	}
	if len(cfg.Scan.Excludes) == 0 {
		cfg.Scan.Excludes = append([]string(nil), def.Scan.Excludes...)
	}
	if cfg.Vault.IndexFile == "" {
		cfg.Vault.IndexFile = def.Vault.IndexFile
	}
	if cfg.Vault.RollupsDir == "" {
		cfg.Vault.RollupsDir = def.Vault.RollupsDir
	}
	if cfg.Vault.ProjectsDir == "" {
		cfg.Vault.ProjectsDir = def.Vault.ProjectsDir
	}
	if cfg.Vault.RecentDays <= 0 {
		cfg.Vault.RecentDays = def.Vault.RecentDays
	}
	if cfg.LLM.Provider == "" {
		cfg.LLM.Provider = def.LLM.Provider
	}
	if cfg.LLM.BaseURL == "" {
		cfg.LLM.BaseURL = def.LLM.BaseURL
	}
	if len(cfg.LLM.Waterfall) == 0 {
		cfg.LLM.Waterfall = append([]string(nil), def.LLM.Waterfall...)
	}
	if cfg.LLM.APIKeyEnv == "" {
		cfg.LLM.APIKeyEnv = def.LLM.APIKeyEnv
	}
	if cfg.AgentLogs.AntigravityPath == "" {
		cfg.AgentLogs.AntigravityPath = def.AgentLogs.AntigravityPath
	}
	if cfg.AgentLogs.PoolsidePath == "" {
		cfg.AgentLogs.PoolsidePath = def.AgentLogs.PoolsidePath
	}
	if cfg.ProjectTags == nil {
		cfg.ProjectTags = make(map[string]ProjectTag)
	}

	if cfg.Vault.Path != "" {
		cfg.Vault.Path = ExpandPath(cfg.Vault.Path)
	}
	for i, root := range cfg.Scan.Roots {
		cfg.Scan.Roots[i] = ExpandPath(root)
	}
	if cfg.AgentLogs.AntigravityPath != "" {
		cfg.AgentLogs.AntigravityPath = ExpandPath(cfg.AgentLogs.AntigravityPath)
	}
	if cfg.AgentLogs.PoolsidePath != "" {
		cfg.AgentLogs.PoolsidePath = ExpandPath(cfg.AgentLogs.PoolsidePath)
	}

	// Validation: ensure required settings are present and valid.
	if strings.TrimSpace(cfg.Vault.Path) == "" {
		return nil, fmt.Errorf("invalid config: vault.path must not be empty")
	}
	if len(cfg.Scan.Roots) == 0 {
		return nil, fmt.Errorf("invalid config: scan.roots must contain at least one directory")
	}
	for _, root := range cfg.Scan.Roots {
		if strings.TrimSpace(root) == "" {
			return nil, fmt.Errorf("invalid config: scan.roots must not contain empty path")
		}
	}
	if cfg.Scan.MaxDepth < 1 {
		return nil, fmt.Errorf("invalid config: scan.max_depth must be at least 1, got %d", cfg.Scan.MaxDepth)
	}

	return &cfg, nil
}

// DefaultConfig returns a Config pre-populated with sensible generic defaults.
func DefaultConfig() *Config {
	home, _ := os.UserHomeDir()
	vaultPath := filepath.Join(home, "vault")
	rootsPath := filepath.Join(home, "projects")
	antigravityPath := filepath.Join(home, ".antigravity")
	poolsidePath := filepath.Join(home, ".poolside")
	if home == "" {
		vaultPath = os.ExpandEnv("$HOME/vault")
		rootsPath = os.ExpandEnv("$HOME/projects")
		antigravityPath = os.ExpandEnv("$HOME/.antigravity")
		poolsidePath = os.ExpandEnv("$HOME/.poolside")
	}

	return &Config{
		Vault: VaultConfig{
			Path:        vaultPath,
			IndexFile:   "00-Dev-Index.md",
			RollupsDir:  "Daily-Rollups",
			ProjectsDir: "Projects",
			RecentDays:  7,
		},
		Scan: ScanConfig{
			Roots:    []string{rootsPath},
			MaxDepth: 3,
			Excludes: []string{
				".nvm", ".local", ".cache",
				"node_modules", "vendor",
			},
		},
		LLM: LLMConfig{
			Provider: "gemini",
			BaseURL:  DefaultLLMBaseURL,
			Waterfall: []string{
				"gemini-3.8-flash",
				"gemini-3.7-flash",
				"gemini-3.6-flash",
			},
			APIKeyEnv: "GOOGLE_API_KEY",
		},
		AgentLogs: AgentLogsConfig{
			Enabled:         false,
			AntigravityPath: antigravityPath,
			PoolsidePath:    poolsidePath,
		},
		ProjectTags: map[string]ProjectTag{},
	}
}
