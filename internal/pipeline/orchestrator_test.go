package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ZeezyCodes/vaultchron/internal/collector"
	"github.com/ZeezyCodes/vaultchron/internal/config"
	"github.com/ZeezyCodes/vaultchron/internal/vault"
)

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
			projectName: "My Project",
			wantSlug:    "my-project",
			wantLang:    "go",
		},
		{
			name:        "nil config with MyAwesomeApp falls back to slugify and go",
			cfg:         nil,
			projectName: "MyAwesomeApp",
			wantSlug:    "myawesomeapp",
			wantLang:    "go",
		},
		{
			name: "empty project tags falls back to slugify and go",
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
					"SpecialProject": {
						Slug: "special-slug",
						Lang: "rust",
					},
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
			name: "configured project tag takes precedence for special project",
			cfg: &config.Config{
				ProjectTags: map[string]config.ProjectTag{
					"SpecialProject": {
						Slug: "special-slug",
						Lang: "rust",
					},
				},
			},
			projectName: "SpecialProject",
			wantSlug:    "special-slug",
			wantLang:    "rust",
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
		{
			name: "unmatched project tag with other project falls back to slugify and go",
			cfg: &config.Config{
				ProjectTags: map[string]config.ProjectTag{
					"OtherProject": {
						Slug: "other-slug",
						Lang: "python",
					},
				},
			},
			projectName: "UnmatchedProject",
			wantSlug:    "unmatchedproject",
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
		{"Hello World", "hello-world"},
		{"special_chars!@#$%^&*()", "special-chars"},
		{"--leading-trailing--", "leading-trailing"},
		{"multiple   spaces", "multiple-spaces"},
		{"", ""},
		{"already-slugified", "already-slugified"},
	}

	for _, tt := range tests {
		got := vault.Slugify(tt.input)
		if got != tt.want {
			t.Errorf("Slugify(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBaseURLResolution(t *testing.T) {
	tests := []struct {
		name        string
		cfgBaseURL  string
		wantBaseURL string
	}{
		{
			name:        "empty config defaults to Gemini endpoint",
			cfgBaseURL:  "",
			wantBaseURL: defaultLLMBaseURL,
		},
		{
			name:        "custom base_url preserved",
			cfgBaseURL:  "https://custom-llm.example.com/v1",
			wantBaseURL: "https://custom-llm.example.com/v1",
		},
		{
			name:        "ollama local endpoint preserved",
			cfgBaseURL:  "http://localhost:11434/v1",
			wantBaseURL: "http://localhost:11434/v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseURL := tt.cfgBaseURL
			if baseURL == "" {
				baseURL = defaultLLMBaseURL
			}
			if baseURL != tt.wantBaseURL {
				t.Errorf("expected baseURL = %q, got %q", tt.wantBaseURL, baseURL)
			}
		})
	}
}

// TestRun_AgentLogsGating verifies that HarvestAgentContext is never invoked
// in day mode, and only in legacy window mode when cfg.AgentLogs.Enabled is true.
func TestRun_AgentLogsGating(t *testing.T) {
	t.Run("disabled by default in legacy mode - no harvest call", func(t *testing.T) {
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

		_ = Run(cfg, PipelineOptions{DryRun: true, Window: "24.hours.ago", IsWindow: true})
		if called {
			t.Error("expected harvestAgentContextFn NOT to be called when AgentLogs.Enabled is false")
		}
	})

	t.Run("enabled in legacy mode - invokes harvest", func(t *testing.T) {
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

		_ = Run(cfg, PipelineOptions{DryRun: true, Window: "24.hours.ago", IsWindow: true})
		if !called {
			t.Error("expected harvestAgentContextFn to be called when AgentLogs.Enabled is true")
		}
	})

	t.Run("day mode - harvest is not called even if enabled", func(t *testing.T) {
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

		now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
		_ = Run(cfg, PipelineOptions{
			Date: "2026-10-02",
			Now:  func() time.Time { return now },
			Loc:  time.UTC,
		})
		if called {
			t.Error("expected harvestAgentContextFn NOT to be called in day mode even if AgentLogs.Enabled is true")
		}
	})
}

// setupTestGitRepo creates an isolated test repository with empty config and fixed identity.
func setupTestGitRepo(t *testing.T) (string, func(envDates []string, args ...string)) {
	t.Helper()
	dir := t.TempDir()
	emptyConfig := filepath.Join(dir, ".emptyconfig")
	if err := os.WriteFile(emptyConfig, []byte(""), 0o600); err != nil {
		t.Fatalf("failed to create empty gitconfig: %v", err)
	}

	runGit := func(envDates []string, args ...string) {
		t.Helper()
		cmdArgs := append([]string{"-C", dir}, args...)
		cmd := exec.Command("git", cmdArgs...)
		cmd.Env = append(os.Environ(),
			"GIT_CONFIG_NOSYSTEM=1",
			"GIT_CONFIG_GLOBAL="+emptyConfig,
			"GIT_AUTHOR_NAME=Test Author",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test Committer",
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

	runGit(nil, "init")
	runGit(nil, "checkout", "-b", "main")
	return dir, runGit
}

// newMockLLMServer starts an httptest server recording LLM requests.
func newMockLLMServer(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var mu sync.Mutex
	var reqs []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			http.NotFound(w, r)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var payload struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.Unmarshal(body, &payload)
		userContent := ""
		for _, m := range payload.Messages {
			if m.Role == "user" {
				userContent = m.Content
				break
			}
		}

		mu.Lock()
		reqs = append(reqs, userContent)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := `{
  "choices": [
    {
      "message": {
        "role": "assistant",
        "content": "> [!abstract] Architectural Evolution\n> Synthesized architecture content.\n\n> [!info] Data Contracts\n> Synthesized data contract.\n\n> [!bug] Regressions & Defect Remediations\n> None.\n\n> [!warning] Immediate Action Items\n> - [ ] Verification checklist item\n\n> [!check] Verification\n> Tests passing."
      }
    }
  ],
  "model": "mock-model"
}`
		_, _ = w.Write([]byte(resp))
	}))

	return ts, &reqs
}

// hashVaultFiles returns a map of relative file path to SHA-256 hash.
func hashVaultFiles(t *testing.T, vaultRoot string) map[string]string {
	t.Helper()
	hashes := make(map[string]string)
	err := filepath.WalkDir(vaultRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(vaultRoot, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(content)
		hashes[rel] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatalf("hashing vault files: %v", err)
	}
	return hashes
}

func setupTestConfig(t *testing.T, repoDir, serverURL string) *config.Config {
	t.Helper()
	t.Setenv("DUMMY_KEY", "dummy-key")
	vaultDir := t.TempDir()
	seven := 7
	twenty := 20
	return &config.Config{
		Vault: config.VaultConfig{
			Path:        vaultDir,
			IndexFile:   "00-Dev-Index.md",
			RollupsDir:  "Daily-Rollups",
			ProjectsDir: "Projects",
			RecentDays:  7,
		},
		Scan: config.ScanConfig{
			Roots:       []string{repoDir},
			MaxDepth:    2,
			CatchUpDays: &seven,
		},
		LLM: config.LLMConfig{
			Provider:       "openai-compatible",
			BaseURL:        serverURL,
			Waterfall:      []string{"mock-model"},
			APIKeyEnv:      "DUMMY_KEY",
			MaxCallsPerRun: &twenty,
		},
	}
}

// TestPipeline_CatchUpOrder asserts that catch-up fills 3 missing days oldest first
// and calls the LLM in that exact order.
func TestPipeline_CatchUpOrder(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*3600)
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, loc)
	repoDir, runGit := setupTestGitRepo(t)

	// Commits on 3 days: 2026-09-28, 2026-09-29, 2026-09-30
	days := []string{"2026-09-28", "2026-09-29", "2026-09-30"}
	for _, d := range days {
		fPath := filepath.Join(repoDir, fmt.Sprintf("file_%s.txt", d))
		if err := os.WriteFile(fPath, []byte("content on "+d+"\n"), 0o644); err != nil {
			t.Fatalf("writing file: %v", err)
		}
		runGit(nil, "add", filepath.Base(fPath))
		runGit([]string{
			fmt.Sprintf("GIT_AUTHOR_DATE=%sT10:00:00-05:00", d),
			fmt.Sprintf("GIT_COMMITTER_DATE=%sT10:00:00-05:00", d),
		}, "commit", "-m", "Commit on "+d)
	}

	ts, reqs := newMockLLMServer(t)
	defer ts.Close()

	cfg := setupTestConfig(t, repoDir, ts.URL)
	opts := PipelineOptions{
		CatchUp: 3,
		Now:     func() time.Time { return now },
		Loc:     loc,
	}

	if err := Run(cfg, opts); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if len(*reqs) != 3 {
		t.Fatalf("expected 3 LLM calls, got %d", len(*reqs))
	}

	// Assert oldest first: 2026-09-28, then 2026-09-29, then 2026-09-30
	for i, expectedDay := range days {
		if !strings.Contains((*reqs)[i], expectedDay) {
			t.Errorf("request %d: expected date %q, got prompt:\n%s", i, expectedDay, (*reqs)[i])
		}
	}
}

// TestPipeline_CallCap asserts that max-calls leaves PENDING rows and the next run completes them.
func TestPipeline_CallCap(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*3600)
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, loc)
	repoDir, runGit := setupTestGitRepo(t)

	days := []string{"2026-09-28", "2026-09-29", "2026-09-30"}
	for _, d := range days {
		fPath := filepath.Join(repoDir, fmt.Sprintf("file_%s.txt", d))
		if err := os.WriteFile(fPath, []byte("content on "+d+"\n"), 0o644); err != nil {
			t.Fatalf("writing file: %v", err)
		}
		runGit(nil, "add", filepath.Base(fPath))
		runGit([]string{
			fmt.Sprintf("GIT_AUTHOR_DATE=%sT10:00:00-05:00", d),
			fmt.Sprintf("GIT_COMMITTER_DATE=%sT10:00:00-05:00", d),
		}, "commit", "-m", "Commit on "+d)
	}

	ts, reqs := newMockLLMServer(t)
	defer ts.Close()

	cfg := setupTestConfig(t, repoDir, ts.URL)

	// Run 1: MaxCalls = 1
	plan1, err := ResolvePlan(PipelineOptions{
		CatchUp:  3,
		MaxCalls: 1,
		Now:      func() time.Time { return now },
		Loc:      loc,
	}, cfg, now, loc)
	if err != nil {
		t.Fatalf("ResolvePlan failed: %v", err)
	}

	rows1, counts1, err := RunPlan(context.Background(), cfg, plan1)
	if err != nil {
		t.Fatalf("RunPlan run 1 failed: %v", err)
	}
	if counts1.Written != 1 || counts1.Pending != 2 {
		t.Errorf("run 1: expected 1 written, 2 pending; got %d written, %d pending", counts1.Written, counts1.Pending)
	}
	if len(*reqs) != 1 {
		t.Errorf("run 1: expected 1 LLM call, got %d", len(*reqs))
	}
	if len(rows1) != 3 {
		t.Fatalf("run 1: expected 3 rows, got %d", len(rows1))
	}
	if rows1[0].Status != "WRITTEN" || rows1[1].Status != "PENDING" || rows1[2].Status != "PENDING" {
		t.Errorf("run 1 row statuses unexpected: %v, %v, %v", rows1[0].Status, rows1[1].Status, rows1[2].Status)
	}

	// Run 2: MaxCalls = 2 completes remaining rows
	plan2, err := ResolvePlan(PipelineOptions{
		CatchUp:  3,
		MaxCalls: 2,
		Now:      func() time.Time { return now },
		Loc:      loc,
	}, cfg, now, loc)
	if err != nil {
		t.Fatalf("ResolvePlan failed: %v", err)
	}

	rows2, counts2, err := RunPlan(context.Background(), cfg, plan2)
	if err != nil {
		t.Fatalf("RunPlan run 2 failed: %v", err)
	}
	if counts2.Skipped != 1 || counts2.Written != 2 || counts2.Pending != 0 {
		t.Errorf("run 2: expected 1 skipped, 2 written, 0 pending; got %d skipped, %d written, %d pending",
			counts2.Skipped, counts2.Written, counts2.Pending)
	}
	if len(*reqs) != 3 {
		t.Errorf("cumulative: expected 3 LLM calls, got %d", len(*reqs))
	}
	if rows2[0].Status != "SKIPPED" || rows2[1].Status != "WRITTEN" || rows2[2].Status != "WRITTEN" {
		t.Errorf("run 2 row statuses unexpected: %v, %v, %v", rows2[0].Status, rows2[1].Status, rows2[2].Status)
	}
}

// TestPipeline_IdempotentRerun asserts that a second run makes ZERO LLM calls
// and leaves every file in the vault byte-identical.
func TestPipeline_IdempotentRerun(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*3600)
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, loc)
	repoDir, runGit := setupTestGitRepo(t)

	fPath := filepath.Join(repoDir, "app.go")
	if err := os.WriteFile(fPath, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("writing app.go: %v", err)
	}
	runGit(nil, "add", "app.go")
	runGit([]string{
		"GIT_AUTHOR_DATE=2026-09-30T10:00:00-05:00",
		"GIT_COMMITTER_DATE=2026-09-30T10:00:00-05:00",
	}, "commit", "-m", "Commit on 2026-09-30")

	ts, reqs := newMockLLMServer(t)
	defer ts.Close()

	cfg := setupTestConfig(t, repoDir, ts.URL)
	opts := PipelineOptions{
		CatchUp: 1,
		Now:     func() time.Time { return now },
		Loc:     loc,
	}

	// First run
	if err := Run(cfg, opts); err != nil {
		t.Fatalf("first run failed: %v", err)
	}
	if len(*reqs) != 1 {
		t.Fatalf("expected 1 LLM call on first run, got %d", len(*reqs))
	}

	hashesBefore := hashVaultFiles(t, cfg.Vault.Path)

	// Second run
	if err := Run(cfg, opts); err != nil {
		t.Fatalf("second run failed: %v", err)
	}
	if len(*reqs) != 1 {
		t.Fatalf("expected ZERO additional LLM calls on second run (total remains 1), got %d", len(*reqs))
	}

	hashesAfter := hashVaultFiles(t, cfg.Vault.Path)
	if len(hashesBefore) != len(hashesAfter) {
		t.Fatalf("vault file count changed between runs: %d vs %d", len(hashesBefore), len(hashesAfter))
	}
	for f, hashBefore := range hashesBefore {
		hashAfter, ok := hashesAfter[f]
		if !ok {
			t.Errorf("file %s missing in second run", f)
		} else if hashBefore != hashAfter {
			t.Errorf("file %s byte content changed: hash %s -> %s", f, hashBefore, hashAfter)
		}
	}
}

// TestPipeline_PartialNoteRegeneration asserts that a partial note (written with -date today)
// is regenerated once the day is complete, and the third run does nothing.
func TestPipeline_PartialNoteRegeneration(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*3600)
	timeDay1 := time.Date(2026, 10, 3, 14, 0, 0, 0, loc)
	repoDir, runGit := setupTestGitRepo(t)

	fPath := filepath.Join(repoDir, "app.go")
	if err := os.WriteFile(fPath, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("writing app.go: %v", err)
	}
	runGit(nil, "add", "app.go")
	runGit([]string{
		"GIT_AUTHOR_DATE=2026-10-03T10:00:00-05:00",
		"GIT_COMMITTER_DATE=2026-10-03T10:00:00-05:00",
	}, "commit", "-m", "Commit on 2026-10-03")

	ts, reqs := newMockLLMServer(t)
	defer ts.Close()

	cfg := setupTestConfig(t, repoDir, ts.URL)
	projectName := collector.ProjectName(repoDir)

	// Run 1: Written with -date today (2026-10-03)
	opts1 := PipelineOptions{
		Date: "2026-10-03",
		Now:  func() time.Time { return timeDay1 },
		Loc:  loc,
	}
	if err := Run(cfg, opts1); err != nil {
		t.Fatalf("run 1 failed: %v", err)
	}
	if len(*reqs) != 1 {
		t.Fatalf("run 1: expected 1 LLM call, got %d", len(*reqs))
	}

	exists, partial, err := vault.DevlogExists(cfg.Vault.Path, projectName, "2026-10-03")
	if err != nil || !exists || !partial {
		t.Fatalf("run 1: expected exists=true, partial=true; got exists=%v, partial=%v, err=%v", exists, partial, err)
	}

	// Advance injected clock to tomorrow (2026-10-04). 2026-10-03 is now complete.
	timeDay2 := time.Date(2026, 10, 4, 10, 0, 0, 0, loc)

	// Run 2: Target 2026-10-03 again. Should regenerate!
	opts2 := PipelineOptions{
		Date: "2026-10-03",
		Now:  func() time.Time { return timeDay2 },
		Loc:  loc,
	}
	if err := Run(cfg, opts2); err != nil {
		t.Fatalf("run 2 failed: %v", err)
	}
	if len(*reqs) != 2 {
		t.Fatalf("run 2: expected 2nd LLM call for regeneration, got total %d", len(*reqs))
	}

	exists, partial, err = vault.DevlogExists(cfg.Vault.Path, projectName, "2026-10-03")
	if err != nil || !exists || partial {
		t.Fatalf("run 2: expected exists=true, partial=false; got exists=%v, partial=%v, err=%v", exists, partial, err)
	}

	// Run 3: Same day (2026-10-04). Should skip without calling LLM!
	opts3 := PipelineOptions{
		Date: "2026-10-03",
		Now:  func() time.Time { return timeDay2 },
		Loc:  loc,
	}
	if err := Run(cfg, opts3); err != nil {
		t.Fatalf("run 3 failed: %v", err)
	}
	if len(*reqs) != 2 {
		t.Fatalf("run 3: expected ZERO additional LLM calls (total remains 2), got %d", len(*reqs))
	}
}

// TestPipeline_ForceAndFromTo tests -force overwriting, -from/-to ranges, and zero-commit day handling.
func TestPipeline_ForceAndFromTo(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*3600)
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, loc)
	repoDir, runGit := setupTestGitRepo(t)

	// Commits on 2026-09-29 and 2026-10-01 (2026-09-30 has ZERO commits)
	if err := os.WriteFile(filepath.Join(repoDir, "file1.txt"), []byte("day1\n"), 0o644); err != nil {
		t.Fatalf("writing file1: %v", err)
	}
	runGit(nil, "add", "file1.txt")
	runGit([]string{
		"GIT_AUTHOR_DATE=2026-09-29T10:00:00-05:00",
		"GIT_COMMITTER_DATE=2026-09-29T10:00:00-05:00",
	}, "commit", "-m", "Commit on 2026-09-29")

	if err := os.WriteFile(filepath.Join(repoDir, "file2.txt"), []byte("day2\n"), 0o644); err != nil {
		t.Fatalf("writing file2: %v", err)
	}
	runGit(nil, "add", "file2.txt")
	runGit([]string{
		"GIT_AUTHOR_DATE=2026-10-01T10:00:00-05:00",
		"GIT_COMMITTER_DATE=2026-10-01T10:00:00-05:00",
	}, "commit", "-m", "Commit on 2026-10-01")

	ts, reqs := newMockLLMServer(t)
	defer ts.Close()

	cfg := setupTestConfig(t, repoDir, ts.URL)
	projectName := collector.ProjectName(repoDir)

	// Run from 2026-09-29 to 2026-10-01 (3 days)
	plan, err := ResolvePlan(PipelineOptions{
		From:  "2026-09-29",
		To:    "2026-10-01",
		Force: true,
		Now:   func() time.Time { return now },
		Loc:   loc,
	}, cfg, now, loc)
	if err != nil {
		t.Fatalf("ResolvePlan failed: %v", err)
	}

	rows, counts, err := RunPlan(context.Background(), cfg, plan)
	if err != nil {
		t.Fatalf("RunPlan failed: %v", err)
	}

	// 2026-09-29: written; 2026-09-30: zero commits (counted, not in rows); 2026-10-01: written
	if counts.Written != 2 || counts.ZeroCommits != 1 {
		t.Errorf("expected 2 written, 1 zero-commit; got %d written, %d zero-commits", counts.Written, counts.ZeroCommits)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 table rows (zero-commit day excluded), got %d", len(rows))
	}

	// Verify zero-commit note was NOT written
	existsZero, _, _ := vault.DevlogExists(cfg.Vault.Path, projectName, "2026-09-30")
	if existsZero {
		t.Error("expected 2026-09-30 note NOT to be written for zero-commit day even with -force")
	}

	// Re-run with -force: should overwrite existing notes (2 LLM calls again)
	planForce, err := ResolvePlan(PipelineOptions{
		From:  "2026-09-29",
		To:    "2026-10-01",
		Force: true,
		Now:   func() time.Time { return now },
		Loc:   loc,
	}, cfg, now, loc)
	if err != nil {
		t.Fatalf("ResolvePlan force failed: %v", err)
	}

	_, countsForce, err := RunPlan(context.Background(), cfg, planForce)
	if err != nil {
		t.Fatalf("RunPlan force failed: %v", err)
	}
	if countsForce.Written != 2 {
		t.Errorf("expected 2 notes overwritten with force, got %d", countsForce.Written)
	}
	if len(*reqs) != 4 { // 2 on first run + 2 on force run
		t.Errorf("expected 4 total LLM calls, got %d", len(*reqs))
	}
}

// TestPipeline_LegacyWindow asserts that legacy -window mode works and skips existing notes unless -force.
func TestPipeline_LegacyWindow(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*3600)
	now := time.Now().In(loc)
	commitDate := now.Add(-1 * time.Hour).Format(time.RFC3339)
	repoDir, runGit := setupTestGitRepo(t)

	if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("writing main.go: %v", err)
	}
	runGit(nil, "add", "main.go")
	runGit([]string{
		"GIT_AUTHOR_DATE=" + commitDate,
		"GIT_COMMITTER_DATE=" + commitDate,
	}, "commit", "-m", "Commit on 2026-10-03")

	ts, reqs := newMockLLMServer(t)
	defer ts.Close()

	cfg := setupTestConfig(t, repoDir, ts.URL)

	// First legacy run: writes devlog for today
	opts1 := PipelineOptions{
		Window:   "24.hours.ago",
		IsWindow: true,
		Now:      func() time.Time { return now },
		Loc:      loc,
	}
	if err := Run(cfg, opts1); err != nil {
		t.Fatalf("first legacy run failed: %v", err)
	}
	if len(*reqs) != 1 {
		t.Fatalf("expected 1 LLM call, got %d", len(*reqs))
	}

	// Second legacy run without -force: skips existing note!
	if err := Run(cfg, opts1); err != nil {
		t.Fatalf("second legacy run failed: %v", err)
	}
	if len(*reqs) != 1 {
		t.Fatalf("expected skip-existing on second legacy run (total calls remains 1), got %d", len(*reqs))
	}

	// Third legacy run with -force: overwrites!
	optsForce := PipelineOptions{
		Window:   "24.hours.ago",
		IsWindow: true,
		Force:    true,
		Now:      func() time.Time { return now },
		Loc:      loc,
	}
	if err := Run(cfg, optsForce); err != nil {
		t.Fatalf("third legacy run with force failed: %v", err)
	}
	if len(*reqs) != 2 {
		t.Fatalf("expected overwrite on third legacy run with force, got %d calls", len(*reqs))
	}
}

// TestPipeline_DryRun asserts that dry-run mode makes zero LLM calls and zero writes.
func TestPipeline_DryRun(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*3600)
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, loc)
	repoDir, runGit := setupTestGitRepo(t)

	if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("writing main.go: %v", err)
	}
	runGit(nil, "add", "main.go")
	runGit([]string{
		"GIT_AUTHOR_DATE=2026-10-02T10:00:00-05:00",
		"GIT_COMMITTER_DATE=2026-10-02T10:00:00-05:00",
	}, "commit", "-m", "Commit on 2026-10-02")

	ts, reqs := newMockLLMServer(t)
	defer ts.Close()

	cfg := setupTestConfig(t, repoDir, ts.URL)
	opts := PipelineOptions{
		Date:   "2026-10-02",
		DryRun: true,
		Now:    func() time.Time { return now },
		Loc:    loc,
	}

	if err := Run(cfg, opts); err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}

	if len(*reqs) != 0 {
		t.Errorf("expected 0 LLM calls in dry-run, got %d", len(*reqs))
	}

	projectName := collector.ProjectName(repoDir)
	exists, _, _ := vault.DevlogExists(cfg.Vault.Path, projectName, "2026-10-02")
	if exists {
		t.Error("expected note NOT to be written in dry-run mode")
	}
}

// TestPipeline_InvalidFlagCombinations asserts that invalid flag combinations error
// with zero LLM calls.
func TestPipeline_InvalidFlagCombinations(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*3600)
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, loc)
	ts, reqs := newMockLLMServer(t)
	defer ts.Close()

	cfg := setupTestConfig(t, t.TempDir(), ts.URL)

	tests := []struct {
		name string
		opts PipelineOptions
	}{
		{
			name: "-date with -from",
			opts: PipelineOptions{
				Date: "2026-10-01",
				From: "2026-10-01",
				Visited: map[string]bool{
					"date": true,
					"from": true,
				},
			},
		},
		{
			name: "-date with -to",
			opts: PipelineOptions{
				Date: "2026-10-01",
				To:   "2026-10-01",
				Visited: map[string]bool{
					"date": true,
					"to":   true,
				},
			},
		},
		{
			name: "-date with -window",
			opts: PipelineOptions{
				Date:     "2026-10-01",
				Window:   "24.hours.ago",
				IsWindow: true,
				Visited: map[string]bool{
					"date":   true,
					"window": true,
				},
			},
		},
		{
			name: "-date with -catch-up",
			opts: PipelineOptions{
				Date:    "2026-10-01",
				CatchUp: 3,
				Visited: map[string]bool{
					"date":     true,
					"catch-up": true,
				},
			},
		},
		{
			name: "-from with -window",
			opts: PipelineOptions{
				From:     "2026-10-01",
				Window:   "24.hours.ago",
				IsWindow: true,
				Visited: map[string]bool{
					"from":   true,
					"window": true,
				},
			},
		},
		{
			name: "-from with -catch-up",
			opts: PipelineOptions{
				From:    "2026-10-01",
				CatchUp: 3,
				Visited: map[string]bool{
					"from":     true,
					"catch-up": true,
				},
			},
		},
		{
			name: "-to without -from",
			opts: PipelineOptions{
				To: "2026-10-01",
				Visited: map[string]bool{
					"to": true,
				},
			},
		},
		{
			name: "-catch-up with -window",
			opts: PipelineOptions{
				CatchUp:  3,
				Window:   "24.hours.ago",
				IsWindow: true,
				Visited: map[string]bool{
					"catch-up": true,
					"window":   true,
				},
			},
		},
		{
			name: "-catch-up < 1",
			opts: PipelineOptions{
				CatchUp: 0,
				Visited: map[string]bool{
					"catch-up": true,
				},
			},
		},
		{
			name: "-max-calls < 0",
			opts: PipelineOptions{
				MaxCalls: -2,
				Visited: map[string]bool{
					"max-calls": true,
				},
			},
		},
		{
			name: "invalid date format",
			opts: PipelineOptions{
				Date: "not-a-date",
				Visited: map[string]bool{
					"date": true,
				},
			},
		},
		{
			name: "future date",
			opts: PipelineOptions{
				Date: "2026-10-10",
				Visited: map[string]bool{
					"date": true,
				},
			},
		},
		{
			name: "from after to",
			opts: PipelineOptions{
				From: "2026-10-02",
				To:   "2026-09-28",
				Visited: map[string]bool{
					"from": true,
					"to":   true,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.opts.Now = func() time.Time { return now }
			tt.opts.Loc = loc
			err := Run(cfg, tt.opts)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
			if len(*reqs) != 0 {
				t.Errorf("expected 0 LLM calls for invalid flags %s, got %d", tt.name, len(*reqs))
			}
		})
	}
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w

	fn()

	_ = w.Close()
	os.Stderr = old
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading captured stderr: %v", err)
	}
	return string(out)
}

func TestPipeline_IndexMissing(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*3600)
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, loc)
	repoDir, runGit := setupTestGitRepo(t)

	days := []string{"2026-09-29", "2026-09-30"}
	for _, d := range days {
		fPath := filepath.Join(repoDir, fmt.Sprintf("file_%s.txt", d))
		if err := os.WriteFile(fPath, []byte("content on "+d+"\n"), 0o644); err != nil {
			t.Fatalf("writing file: %v", err)
		}
		runGit(nil, "add", filepath.Base(fPath))
		runGit([]string{
			fmt.Sprintf("GIT_AUTHOR_DATE=%sT10:00:00-05:00", d),
			fmt.Sprintf("GIT_COMMITTER_DATE=%sT10:00:00-05:00", d),
		}, "commit", "-m", "Commit on "+d)
	}

	ts, reqs := newMockLLMServer(t)
	defer ts.Close()

	cfg := setupTestConfig(t, repoDir, ts.URL)
	plan, err := ResolvePlan(PipelineOptions{
		From:    "2026-09-29",
		To:      "2026-09-30",
		Visited: map[string]bool{"from": true, "to": true},
		Now:     func() time.Time { return now },
		Loc:     loc,
	}, cfg, now, loc)
	if err != nil {
		t.Fatalf("ResolvePlan: %v", err)
	}

	var rows []DayResultRow
	var counts DaySummaryCounts
	var runErr error

	captured := captureStderr(t, func() {
		rows, counts, runErr = RunPlan(context.Background(), cfg, plan)
	})

	if runErr != nil {
		t.Fatalf("RunPlan failed: %v", runErr)
	}
	if counts.Errors != 0 {
		t.Errorf("expected 0 errors, got %d", counts.Errors)
	}
	if counts.Written != 2 {
		t.Errorf("expected 2 written, got %d", counts.Written)
	}
	for _, r := range rows {
		if r.Status != "WRITTEN" {
			t.Errorf("row for %s status = %q, want WRITTEN", r.Date, r.Status)
		}
	}

	warnLine := fmt.Sprintf("[WARN] index file %s not found; notes are written but the index is not updated",
		filepath.Join(cfg.Vault.Path, cfg.Vault.IndexFile))
	if strings.Contains(captured, warnLine) {
		t.Errorf("expected no warning about missing index, got in stderr:\n%s", captured)
	}

	// Verify index was created and contains rows
	indexPath := filepath.Join(cfg.Vault.Path, cfg.Vault.IndexFile)
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("expected bootstrapped index at %s: %v", indexPath, err)
	}
	indexContent := string(indexData)
	projectName := collector.ProjectName(repoDir)
	for _, d := range days {
		expectedRow := fmt.Sprintf("[[Projects/%s/Devlog/%s|%s]]", projectName, d, d)
		if !strings.Contains(indexContent, expectedRow) {
			t.Errorf("expected index to contain entry %s, got:\n%s", expectedRow, indexContent)
		}
	}

	// Verify overview stub was created
	overviewPath := filepath.Join(cfg.Vault.Path, "Projects", projectName, "Overview.md")
	if _, err := os.Stat(overviewPath); err != nil {
		t.Errorf("expected overview stub to exist at %s: %v", overviewPath, err)
	}

	// Verify notes written on disk
	for _, d := range days {
		exists, _, _ := vault.DevlogExists(cfg.Vault.Path, projectName, d)
		if !exists {
			t.Errorf("expected devlog note for %s to exist on disk", d)
		}
	}

	// Rerun gives zero new LLM requests and SKIPPED rows
	initialReqs := len(*reqs)
	rowsRerun, countsRerun, errRerun := RunPlan(context.Background(), cfg, plan)
	if errRerun != nil {
		t.Fatalf("rerun failed: %v", errRerun)
	}
	if len(*reqs) != initialReqs {
		t.Errorf("expected zero new LLM requests on rerun, got %d (was %d)", len(*reqs), initialReqs)
	}
	if countsRerun.Skipped != 2 {
		t.Errorf("expected 2 skipped on rerun, got %d", countsRerun.Skipped)
	}
	for _, r := range rowsRerun {
		if r.Status != "SKIPPED" {
			t.Errorf("rerun row for %s status = %q, want SKIPPED", r.Date, r.Status)
		}
	}
}

func TestPipeline_IndexBroken(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*3600)
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, loc)
	repoDir, runGit := setupTestGitRepo(t)

	days := []string{"2026-09-28", "2026-09-29", "2026-09-30"}
	for _, d := range days {
		fPath := filepath.Join(repoDir, fmt.Sprintf("file_%s.txt", d))
		if err := os.WriteFile(fPath, []byte("content on "+d+"\n"), 0o644); err != nil {
			t.Fatalf("writing file: %v", err)
		}
		runGit(nil, "add", filepath.Base(fPath))
		runGit([]string{
			fmt.Sprintf("GIT_AUTHOR_DATE=%sT10:00:00-05:00", d),
			fmt.Sprintf("GIT_COMMITTER_DATE=%sT10:00:00-05:00", d),
		}, "commit", "-m", "Commit on "+d)
	}

	ts, reqs := newMockLLMServer(t)
	defer ts.Close()

	cfg := setupTestConfig(t, repoDir, ts.URL)
	indexPath := filepath.Join(cfg.Vault.Path, cfg.Vault.IndexFile)
	if err := os.WriteFile(indexPath, []byte("# Index\n"), 0o644); err != nil {
		t.Fatalf("writing broken index: %v", err)
	}

	plan, err := ResolvePlan(PipelineOptions{
		From:    "2026-09-28",
		To:      "2026-09-30",
		Visited: map[string]bool{"from": true, "to": true},
		Now:     func() time.Time { return now },
		Loc:     loc,
	}, cfg, now, loc)
	if err != nil {
		t.Fatalf("ResolvePlan: %v", err)
	}

	rows, counts, runErr := RunPlan(context.Background(), cfg, plan)
	if runErr == nil {
		t.Fatal("expected RunPlan to return an error when index update fails, got nil")
	}
	if len(*reqs) != 1 {
		t.Fatalf("expected exactly 1 LLM request, got %d", len(*reqs))
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	// First row ERROR
	if rows[0].Status != "ERROR" {
		t.Errorf("row 0 status = %q, want ERROR", rows[0].Status)
	}
	if !strings.Contains(rows[0].Output, "index update failed:") || !strings.Contains(rows[0].Output, "; note not written") {
		t.Errorf("row 0 output unexpected: %q", rows[0].Output)
	}

	// Other two rows PENDING with index reason
	for i := 1; i <= 2; i++ {
		if rows[i].Status != "PENDING" {
			t.Errorf("row %d status = %q, want PENDING", i, rows[i].Status)
		}
		if rows[i].Output != "not attempted: index update failed earlier in this run" {
			t.Errorf("row %d output = %q, want 'not attempted: index update failed earlier in this run'", i, rows[i].Output)
		}
	}

	if counts.Errors != 1 {
		t.Errorf("counts.Errors = %d, want 1", counts.Errors)
	}
	if counts.Pending != 2 || counts.IndexPending != 2 {
		t.Errorf("counts.Pending = %d, counts.IndexPending = %d, want 2", counts.Pending, counts.IndexPending)
	}

	// NO note files exist
	projectName := collector.ProjectName(repoDir)
	for _, d := range days {
		exists, _, _ := vault.DevlogExists(cfg.Vault.Path, projectName, d)
		if exists {
			t.Errorf("note for %s should not exist after index failure", d)
		}
	}

	// Replace index with valid one
	validIndex := "# Developer Index\n\n## Recent Dev Logs\n\n## Projects\n"
	if err := os.WriteFile(indexPath, []byte(validIndex), 0o644); err != nil {
		t.Fatalf("writing valid index: %v", err)
	}

	// Rerun
	rowsRerun, countsRerun, errRerun := RunPlan(context.Background(), cfg, plan)
	if errRerun != nil {
		t.Fatalf("rerun failed: %v", errRerun)
	}
	if countsRerun.Written != 3 {
		t.Errorf("countsRerun.Written = %d, want 3", countsRerun.Written)
	}
	if len(rowsRerun) != 3 {
		t.Fatalf("rerun rows count = %d, want 3", len(rowsRerun))
	}
	for _, r := range rowsRerun {
		if r.Status != "WRITTEN" {
			t.Errorf("rerun row %s status = %q, want WRITTEN", r.Date, r.Status)
		}
	}

	// 3 notes written on disk
	for _, d := range days {
		exists, _, _ := vault.DevlogExists(cfg.Vault.Path, projectName, d)
		if !exists {
			t.Errorf("expected note for %s to exist after rerun", d)
		}
	}

	// 3 index rows in date-descending order
	indexContentBytes, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("reading index file: %v", err)
	}
	indexContent := string(indexContentBytes)
	idx30 := strings.Index(indexContent, "2026-09-30")
	idx29 := strings.Index(indexContent, "2026-09-29")
	idx28 := strings.Index(indexContent, "2026-09-28")
	if idx30 == -1 || idx29 == -1 || idx28 == -1 {
		t.Fatalf("missing dates in index file:\n%s", indexContent)
	}
	if !(idx30 < idx29 && idx29 < idx28) {
		t.Errorf("expected dates in descending order (2026-09-30, then 2026-09-29, then 2026-09-28), got positions: %d, %d, %d",
			idx30, idx29, idx28)
	}
}

func TestPipeline_BrokenIndex_DryRun(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*3600)
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, loc)
	repoDir, runGit := setupTestGitRepo(t)

	fPath := filepath.Join(repoDir, "file_2026-09-30.txt")
	if err := os.WriteFile(fPath, []byte("content\n"), 0o644); err != nil {
		t.Fatalf("writing file: %v", err)
	}
	runGit(nil, "add", filepath.Base(fPath))
	runGit([]string{
		"GIT_AUTHOR_DATE=2026-09-30T10:00:00-05:00",
		"GIT_COMMITTER_DATE=2026-09-30T10:00:00-05:00",
	}, "commit", "-m", "Commit on 2026-09-30")

	ts, reqs := newMockLLMServer(t)
	defer ts.Close()

	cfg := setupTestConfig(t, repoDir, ts.URL)
	indexPath := filepath.Join(cfg.Vault.Path, cfg.Vault.IndexFile)
	if err := os.WriteFile(indexPath, []byte("# Index\n"), 0o644); err != nil {
		t.Fatalf("writing broken index: %v", err)
	}

	plan, err := ResolvePlan(PipelineOptions{
		Date:    "2026-09-30",
		DryRun:  true,
		Visited: map[string]bool{"date": true},
		Now:     func() time.Time { return now },
		Loc:     loc,
	}, cfg, now, loc)
	if err != nil {
		t.Fatalf("ResolvePlan: %v", err)
	}

	rows, counts, runErr := RunPlan(context.Background(), cfg, plan)
	if runErr != nil {
		t.Fatalf("dry-run should not fail on broken index, got: %v", runErr)
	}
	if len(*reqs) != 0 {
		t.Errorf("expected 0 LLM requests in dry-run, got %d", len(*reqs))
	}
	if counts.Errors != 0 {
		t.Errorf("counts.Errors = %d, want 0", counts.Errors)
	}
	if counts.Written != 1 {
		t.Errorf("counts.Written = %d, want 1", counts.Written)
	}
	if len(rows) != 1 || rows[0].Status != "WOULD WRITE" {
		t.Errorf("unexpected rows: %v", rows)
	}

	// Verify zero writes: only index file in vault
	files := hashVaultFiles(t, cfg.Vault.Path)
	if len(files) != 1 || files[cfg.Vault.IndexFile] == "" {
		t.Errorf("expected only index file in vault, got: %v", files)
	}
}

func TestFormatDaySummaryLine(t *testing.T) {
	tests := []struct {
		name   string
		counts DaySummaryCounts
		dryRun bool
		want   string
	}{
		{
			name: "clean written",
			counts: DaySummaryCounts{
				Written: 2,
				Skipped: 1,
				Pending: 0,
				Errors:  0,
			},
			dryRun: false,
			want:   "2 written, 1 skipped, 0 pending, 0 errors",
		},
		{
			name: "cap pending only",
			counts: DaySummaryCounts{
				Written:    1,
				Skipped:    0,
				Pending:    2,
				CapPending: 2,
				Errors:     0,
			},
			dryRun: false,
			want:   "1 written, 0 skipped, 2 pending (call cap reached; run again to continue), 0 errors",
		},
		{
			name: "index pending only",
			counts: DaySummaryCounts{
				Written:      0,
				Skipped:      0,
				Pending:      2,
				IndexPending: 2,
				Errors:       1,
			},
			dryRun: false,
			want:   "0 written, 0 skipped, 2 pending (index error; fix the index and run again), 1 errors",
		},
		{
			name: "both cap pending and index pending",
			counts: DaySummaryCounts{
				Written:      1,
				Skipped:      0,
				Pending:      3,
				CapPending:   1,
				IndexPending: 2,
				Errors:       1,
			},
			dryRun: false,
			want:   "1 written, 0 skipped, 3 pending (call cap reached; run again to continue) (index error; fix the index and run again), 1 errors",
		},
		{
			name: "dry run with cap pending",
			counts: DaySummaryCounts{
				Written:    2,
				Skipped:    0,
				Pending:    1,
				CapPending: 1,
				Errors:     0,
			},
			dryRun: true,
			want:   "2 would be written, 0 skipped, 1 pending (call cap reached; run again to continue), 0 errors",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatDaySummaryLine(tt.counts, tt.dryRun)
			if got != tt.want {
				t.Errorf("FormatDaySummaryLine() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

func TestResolvePlan_MaxCallsConfigFallback(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*3600)
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, loc)
	repoDir, runGit := setupTestGitRepo(t)

	days := []string{"2026-09-28", "2026-09-29", "2026-09-30"}
	for _, d := range days {
		fPath := filepath.Join(repoDir, fmt.Sprintf("file_%s.txt", d))
		if err := os.WriteFile(fPath, []byte("content on "+d+"\n"), 0o644); err != nil {
			t.Fatalf("writing file: %v", err)
		}
		runGit(nil, "add", filepath.Base(fPath))
		runGit([]string{
			fmt.Sprintf("GIT_AUTHOR_DATE=%sT10:00:00-05:00", d),
			fmt.Sprintf("GIT_COMMITTER_DATE=%sT10:00:00-05:00", d),
		}, "commit", "-m", "Commit on "+d)
	}

	ts, reqs := newMockLLMServer(t)
	defer ts.Close()

	cfg := setupTestConfig(t, repoDir, ts.URL)
	two := 2
	cfg.LLM.MaxCallsPerRun = &two

	// Case 1: zero-value options with Visited nil use the configured cap (cfg max_calls_per_run=2, 3 days: 2 written, 1 pending)
	plan1, err := ResolvePlan(PipelineOptions{
		From: "2026-09-28",
		To:   "2026-09-30",
		Now:  func() time.Time { return now },
		Loc:  loc,
	}, cfg, now, loc)
	if err != nil {
		t.Fatalf("ResolvePlan fallback failed: %v", err)
	}
	if plan1.MaxCalls != 2 {
		t.Fatalf("expected plan1.MaxCalls = 2 from config fallback, got %d", plan1.MaxCalls)
	}

	rows1, counts1, err := RunPlan(context.Background(), cfg, plan1)
	if err != nil {
		t.Fatalf("RunPlan failed: %v", err)
	}
	if counts1.Written != 2 || counts1.Pending != 1 || counts1.CapPending != 1 {
		t.Errorf("expected 2 written, 1 pending (cap); got %d written, %d pending (%d cap pending)",
			counts1.Written, counts1.Pending, counts1.CapPending)
	}
	if len(*reqs) != 2 {
		t.Errorf("expected 2 LLM requests, got %d", len(*reqs))
	}
	if rows1[0].Status != "WRITTEN" || rows1[1].Status != "WRITTEN" || rows1[2].Status != "PENDING" {
		t.Errorf("unexpected rows statuses: %v, %v, %v", rows1[0].Status, rows1[1].Status, rows1[2].Status)
	}
	if rows1[2].Output != "call cap reached" {
		t.Errorf("expected row 2 output 'call cap reached', got %q", rows1[2].Output)
	}

	// Case 2: explicit Visited with "max-calls": true and MaxCalls: 0 means unlimited
	plan2, err := ResolvePlan(PipelineOptions{
		From:     "2026-09-28",
		To:       "2026-09-30",
		MaxCalls: 0,
		Visited: map[string]bool{
			"from":      true,
			"to":        true,
			"max-calls": true,
		},
		Now: func() time.Time { return now },
		Loc: loc,
	}, cfg, now, loc)
	if err != nil {
		t.Fatalf("ResolvePlan explicit unlimited failed: %v", err)
	}
	if plan2.MaxCalls != 0 {
		t.Errorf("expected plan2.MaxCalls = 0 (unlimited), got %d", plan2.MaxCalls)
	}
}

func TestFutureDateErrorMessage(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*3600)
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, loc)

	_, err := collector.NewDayWindow("2026-10-10", loc, now)
	if err == nil {
		t.Fatal("expected error for future date, got nil")
	}
	want := "date 2026-10-10 is in the future"
	if err.Error() != want {
		t.Errorf("error = %q, want exactly %q", err.Error(), want)
	}
}

func TestPipeline_OverviewStub_NotOverwritten(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*3600)
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, loc)
	repoDir, runGit := setupTestGitRepo(t)

	fPath := filepath.Join(repoDir, "file_2026-09-30.txt")
	if err := os.WriteFile(fPath, []byte("content\n"), 0o644); err != nil {
		t.Fatalf("writing file: %v", err)
	}
	runGit(nil, "add", filepath.Base(fPath))
	runGit([]string{
		"GIT_AUTHOR_DATE=2026-09-30T10:00:00-05:00",
		"GIT_COMMITTER_DATE=2026-09-30T10:00:00-05:00",
	}, "commit", "-m", "Commit on 2026-09-30")

	ts, _ := newMockLLMServer(t)
	defer ts.Close()

	cfg := setupTestConfig(t, repoDir, ts.URL)
	plan, err := ResolvePlan(PipelineOptions{
		Date:    "2026-09-30",
		Visited: map[string]bool{"date": true},
		Now:     func() time.Time { return now },
		Loc:     loc,
	}, cfg, now, loc)
	if err != nil {
		t.Fatalf("ResolvePlan: %v", err)
	}

	if _, _, err := RunPlan(context.Background(), cfg, plan); err != nil {
		t.Fatalf("RunPlan failed: %v", err)
	}

	projectName := collector.ProjectName(repoDir)
	overviewPath := filepath.Join(cfg.Vault.Path, "Projects", projectName, "Overview.md")
	if _, err := os.Stat(overviewPath); err != nil {
		t.Fatalf("expected overview stub at %s: %v", overviewPath, err)
	}

	// Modify content of the overview stub
	customContent := "# Custom Project Overview\n\nPreserved user documentation\n"
	if err := os.WriteFile(overviewPath, []byte(customContent), 0o644); err != nil {
		t.Fatalf("modifying overview stub: %v", err)
	}

	// Re-run pipeline with Force = true
	planForce, err := ResolvePlan(PipelineOptions{
		Date:    "2026-09-30",
		Force:   true,
		Visited: map[string]bool{"date": true, "force": true},
		Now:     func() time.Time { return now },
		Loc:     loc,
	}, cfg, now, loc)
	if err != nil {
		t.Fatalf("ResolvePlan force: %v", err)
	}

	if _, _, err := RunPlan(context.Background(), cfg, planForce); err != nil {
		t.Fatalf("RunPlan with force failed: %v", err)
	}

	afterData, err := os.ReadFile(overviewPath)
	if err != nil {
		t.Fatalf("reading overview stub after force run: %v", err)
	}
	if string(afterData) != customContent {
		t.Errorf("overview stub was overwritten on force run; expected %q, got %q", customContent, string(afterData))
	}
}

func TestPipeline_ExistingIndex_NotTouched(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*3600)
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, loc)
	repoDir, runGit := setupTestGitRepo(t)

	fPath := filepath.Join(repoDir, "file_2026-09-30.txt")
	if err := os.WriteFile(fPath, []byte("content\n"), 0o644); err != nil {
		t.Fatalf("writing file: %v", err)
	}
	runGit(nil, "add", filepath.Base(fPath))
	runGit([]string{
		"GIT_AUTHOR_DATE=2026-09-30T10:00:00-05:00",
		"GIT_COMMITTER_DATE=2026-09-30T10:00:00-05:00",
	}, "commit", "-m", "Commit on 2026-09-30")

	ts, _ := newMockLLMServer(t)
	defer ts.Close()

	cfg := setupTestConfig(t, repoDir, ts.URL)
	indexPath := filepath.Join(cfg.Vault.Path, cfg.Vault.IndexFile)

	customIndex := "# Custom Vault Title\n\nCustom preamble note that must stay.\n\n## Recent Dev Logs\n\n| Date | Notes |\n|---|---|\n"
	if err := os.WriteFile(indexPath, []byte(customIndex), 0o644); err != nil {
		t.Fatalf("writing custom index: %v", err)
	}

	plan, err := ResolvePlan(PipelineOptions{
		Date:    "2026-09-30",
		Visited: map[string]bool{"date": true},
		Now:     func() time.Time { return now },
		Loc:     loc,
	}, cfg, now, loc)
	if err != nil {
		t.Fatalf("ResolvePlan: %v", err)
	}

	if _, _, err := RunPlan(context.Background(), cfg, plan); err != nil {
		t.Fatalf("RunPlan failed: %v", err)
	}

	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("reading index: %v", err)
	}
	indexContent := string(indexData)

	if !strings.Contains(indexContent, "# Custom Vault Title") {
		t.Errorf("custom index title was lost:\n%s", indexContent)
	}
	if !strings.Contains(indexContent, "Custom preamble note that must stay.") {
		t.Errorf("custom preamble was lost:\n%s", indexContent)
	}
	projectName := collector.ProjectName(repoDir)
	expectedLink := fmt.Sprintf("[[Projects/%s/Devlog/2026-09-30|2026-09-30]]", projectName)
	if !strings.Contains(indexContent, expectedLink) {
		t.Errorf("expected devlog entry %s in index:\n%s", expectedLink, indexContent)
	}
}

func TestPipeline_DryRun_CreatesNoIndexAndNoStub(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*3600)
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, loc)
	repoDir, runGit := setupTestGitRepo(t)

	fPath := filepath.Join(repoDir, "file_2026-09-30.txt")
	if err := os.WriteFile(fPath, []byte("content\n"), 0o644); err != nil {
		t.Fatalf("writing file: %v", err)
	}
	runGit(nil, "add", filepath.Base(fPath))
	runGit([]string{
		"GIT_AUTHOR_DATE=2026-09-30T10:00:00-05:00",
		"GIT_COMMITTER_DATE=2026-09-30T10:00:00-05:00",
	}, "commit", "-m", "Commit on 2026-09-30")

	ts, _ := newMockLLMServer(t)
	defer ts.Close()

	cfg := setupTestConfig(t, repoDir, ts.URL)
	plan, err := ResolvePlan(PipelineOptions{
		Date:    "2026-09-30",
		DryRun:  true,
		Visited: map[string]bool{"date": true},
		Now:     func() time.Time { return now },
		Loc:     loc,
	}, cfg, now, loc)
	if err != nil {
		t.Fatalf("ResolvePlan: %v", err)
	}

	if _, _, err := RunPlan(context.Background(), cfg, plan); err != nil {
		t.Fatalf("RunPlan dry-run failed: %v", err)
	}

	indexPath := filepath.Join(cfg.Vault.Path, cfg.Vault.IndexFile)
	if _, err := os.Stat(indexPath); !errors.Is(err, os.ErrNotExist) && !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("expected index file to NOT exist on dry-run, got err=%v", err)
	}

	projectName := collector.ProjectName(repoDir)
	overviewPath := filepath.Join(cfg.Vault.Path, "Projects", projectName, "Overview.md")
	if _, err := os.Stat(overviewPath); !errors.Is(err, os.ErrNotExist) && !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("expected overview stub to NOT exist on dry-run, got err=%v", err)
	}
}

func TestPipeline_NoNotes_CreatesNoIndex(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*3600)
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, loc)
	repoDir, _ := setupTestGitRepo(t) // empty repo, 0 commits

	ts, _ := newMockLLMServer(t)
	defer ts.Close()

	cfg := setupTestConfig(t, repoDir, ts.URL)
	plan, err := ResolvePlan(PipelineOptions{
		Date:    "2026-09-30",
		Visited: map[string]bool{"date": true},
		Now:     func() time.Time { return now },
		Loc:     loc,
	}, cfg, now, loc)
	if err != nil {
		t.Fatalf("ResolvePlan: %v", err)
	}

	if _, _, err := RunPlan(context.Background(), cfg, plan); err != nil {
		t.Fatalf("RunPlan failed: %v", err)
	}

	indexPath := filepath.Join(cfg.Vault.Path, cfg.Vault.IndexFile)
	if _, err := os.Stat(indexPath); !errors.Is(err, os.ErrNotExist) && !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("expected index file to NOT exist when no notes are written, got err=%v", err)
	}
}

func TestPipeline_LegacyWindow_IndexBootstrap(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*3600)
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, loc)
	commitDate := now.Add(-1 * time.Hour).Format(time.RFC3339)
	repoDir, runGit := setupTestGitRepo(t)

	if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("writing main.go: %v", err)
	}
	runGit(nil, "add", "main.go")
	runGit([]string{
		"GIT_AUTHOR_DATE=" + commitDate,
		"GIT_COMMITTER_DATE=" + commitDate,
	}, "commit", "-m", "Commit on 2026-10-03")

	ts, _ := newMockLLMServer(t)
	defer ts.Close()

	cfg := setupTestConfig(t, repoDir, ts.URL)

	opts := PipelineOptions{
		Window:   now.Add(-2 * time.Hour).Format(time.RFC3339),
		IsWindow: true,
		Now:      func() time.Time { return now },
		Loc:      loc,
	}

	captured := captureStderr(t, func() {
		if err := Run(cfg, opts); err != nil {
			t.Fatalf("legacy window run failed: %v", err)
		}
	})

	if strings.Contains(captured, "[ERROR]") && strings.Contains(captured, "index update failed") {
		t.Errorf("expected no index update error in legacy run, got in stderr:\n%s", captured)
	}

	indexPath := filepath.Join(cfg.Vault.Path, cfg.Vault.IndexFile)
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("expected bootstrapped index in legacy run at %s: %v", indexPath, err)
	}
	projectName := collector.ProjectName(repoDir)
	today := now.Format("2006-01-02")
	expectedLink := fmt.Sprintf("[[Projects/%s/Devlog/%s|%s]]", projectName, today, today)
	if !strings.Contains(string(indexData), expectedLink) {
		t.Errorf("expected index to contain %s, got:\n%s", expectedLink, string(indexData))
	}

	overviewPath := filepath.Join(cfg.Vault.Path, "Projects", projectName, "Overview.md")
	if _, err := os.Stat(overviewPath); err != nil {
		t.Errorf("expected overview stub in legacy run at %s: %v", overviewPath, err)
	}
}

func TestPipeline_CustomConfig_DayMode(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*3600)
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, loc)
	repoDir, runGit := setupTestGitRepo(t)

	if err := os.WriteFile(filepath.Join(repoDir, "service.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("writing service.go: %v", err)
	}
	runGit(nil, "add", "service.go")
	runGit([]string{
		"GIT_AUTHOR_DATE=2026-10-02T10:00:00-05:00",
		"GIT_COMMITTER_DATE=2026-10-02T10:00:00-05:00",
	}, "commit", "-m", "Commit on 2026-10-02")

	ts, reqs := newMockLLMServer(t)
	defer ts.Close()

	cfg := setupTestConfig(t, repoDir, ts.URL)
	cfg.Vault.IndexFile = "Meta/Dev-Index.md"
	cfg.Vault.ProjectsDir = "Dev/Projects"

	opts := PipelineOptions{
		Date: "2026-10-02",
		Now:  func() time.Time { return now },
		Loc:  loc,
	}

	captured := captureStderr(t, func() {
		if err := Run(cfg, opts); err != nil {
			t.Fatalf("day mode run failed: %v", err)
		}
	})

	if strings.Contains(captured, "[ERROR]") {
		t.Errorf("expected no [ERROR] in stderr, got:\n%s", captured)
	}

	projectName := collector.ProjectName(repoDir)

	// Verify expected files exist
	expectedNotePath := filepath.Join(cfg.Vault.Path, "Dev", "Projects", projectName, "Devlog", "2026-10-02.md")
	if _, err := os.Stat(expectedNotePath); err != nil {
		t.Fatalf("expected note at %s: %v", expectedNotePath, err)
	}
	expectedOverviewPath := filepath.Join(cfg.Vault.Path, "Dev", "Projects", projectName, "Overview.md")
	if _, err := os.Stat(expectedOverviewPath); err != nil {
		t.Fatalf("expected overview stub at %s: %v", expectedOverviewPath, err)
	}
	expectedIndexPath := filepath.Join(cfg.Vault.Path, "Meta", "Dev-Index.md")
	indexBytes, err := os.ReadFile(expectedIndexPath)
	if err != nil {
		t.Fatalf("expected index at %s: %v", expectedIndexPath, err)
	}
	expectedLink := fmt.Sprintf("[[Dev/Projects/%s/Devlog/2026-10-02|2026-10-02]]", projectName)
	if !strings.Contains(string(indexBytes), expectedLink) {
		t.Errorf("expected index to contain %s, got:\n%s", expectedLink, string(indexBytes))
	}

	// Verify NOTHING created under default Projects/ or 00-Dev-Index.md
	defaultProjectsDir := filepath.Join(cfg.Vault.Path, "Projects")
	if _, err := os.Stat(defaultProjectsDir); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("expected NOTHING under default Projects/, but it exists")
	}
	defaultIndexPath := filepath.Join(cfg.Vault.Path, "00-Dev-Index.md")
	if _, err := os.Stat(defaultIndexPath); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("expected NOTHING at default 00-Dev-Index.md, but it exists")
	}

	// Re-run: should skip and make no additional LLM calls
	initialCalls := len(*reqs)
	capturedRerun := captureStderr(t, func() {
		if err := Run(cfg, opts); err != nil {
			t.Fatalf("re-run failed: %v", err)
		}
	})
	if strings.Contains(capturedRerun, "[ERROR]") {
		t.Errorf("expected no [ERROR] in re-run stderr, got:\n%s", capturedRerun)
	}
	if len(*reqs) != initialCalls {
		t.Errorf("expected re-run to make 0 additional LLM calls, got %d", len(*reqs)-initialCalls)
	}
}

func TestPipeline_CustomConfig_LegacyWindow(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*3600)
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, loc)
	repoDir, runGit := setupTestGitRepo(t)

	if err := os.WriteFile(filepath.Join(repoDir, "handler.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("writing handler.go: %v", err)
	}
	runGit(nil, "add", "handler.go")
	runGit([]string{
		"GIT_AUTHOR_DATE=2026-10-03T10:00:00-05:00",
		"GIT_COMMITTER_DATE=2026-10-03T10:00:00-05:00",
	}, "commit", "-m", "Commit on 2026-10-03")

	ts, reqs := newMockLLMServer(t)
	defer ts.Close()

	cfg := setupTestConfig(t, repoDir, ts.URL)
	cfg.Vault.IndexFile = "Meta/Dev-Index.md"
	cfg.Vault.ProjectsDir = "Dev/Projects"

	opts := PipelineOptions{
		Window:   now.Add(-2 * time.Hour).Format(time.RFC3339),
		IsWindow: true,
		Now:      func() time.Time { return now },
		Loc:      loc,
	}

	captured := captureStderr(t, func() {
		if err := Run(cfg, opts); err != nil {
			t.Fatalf("legacy window run failed: %v", err)
		}
	})

	if strings.Contains(captured, "[ERROR]") {
		t.Errorf("expected no [ERROR] in stderr, got:\n%s", captured)
	}

	projectName := collector.ProjectName(repoDir)
	today := now.Format("2006-01-02")

	// Verify expected files exist
	expectedNotePath := filepath.Join(cfg.Vault.Path, "Dev", "Projects", projectName, "Devlog", today+".md")
	if _, err := os.Stat(expectedNotePath); err != nil {
		t.Fatalf("expected note at %s: %v", expectedNotePath, err)
	}
	expectedOverviewPath := filepath.Join(cfg.Vault.Path, "Dev", "Projects", projectName, "Overview.md")
	if _, err := os.Stat(expectedOverviewPath); err != nil {
		t.Fatalf("expected overview stub at %s: %v", expectedOverviewPath, err)
	}
	expectedIndexPath := filepath.Join(cfg.Vault.Path, "Meta", "Dev-Index.md")
	indexBytes, err := os.ReadFile(expectedIndexPath)
	if err != nil {
		t.Fatalf("expected index at %s: %v", expectedIndexPath, err)
	}
	expectedLink := fmt.Sprintf("[[Dev/Projects/%s/Devlog/%s|%s]]", projectName, today, today)
	if !strings.Contains(string(indexBytes), expectedLink) {
		t.Errorf("expected index to contain %s, got:\n%s", expectedLink, string(indexBytes))
	}

	// Verify NOTHING created under default Projects/ or 00-Dev-Index.md
	defaultProjectsDir := filepath.Join(cfg.Vault.Path, "Projects")
	if _, err := os.Stat(defaultProjectsDir); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("expected NOTHING under default Projects/, but it exists")
	}
	defaultIndexPath := filepath.Join(cfg.Vault.Path, "00-Dev-Index.md")
	if _, err := os.Stat(defaultIndexPath); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("expected NOTHING at default 00-Dev-Index.md, but it exists")
	}

	// Re-run: should skip and make no additional LLM calls
	initialCalls := len(*reqs)
	capturedRerun := captureStderr(t, func() {
		if err := Run(cfg, opts); err != nil {
			t.Fatalf("legacy re-run failed: %v", err)
		}
	})
	if strings.Contains(capturedRerun, "[ERROR]") {
		t.Errorf("expected no [ERROR] in re-run stderr, got:\n%s", capturedRerun)
	}
	if len(*reqs) != initialCalls {
		t.Errorf("expected legacy re-run to make 0 additional LLM calls, got %d", len(*reqs)-initialCalls)
	}
}

func TestPipeline_CustomConfig_DryRun(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*3600)
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, loc)
	repoDir, runGit := setupTestGitRepo(t)

	if err := os.WriteFile(filepath.Join(repoDir, "dry.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("writing dry.go: %v", err)
	}
	runGit(nil, "add", "dry.go")
	runGit([]string{
		"GIT_AUTHOR_DATE=2026-10-02T10:00:00-05:00",
		"GIT_COMMITTER_DATE=2026-10-02T10:00:00-05:00",
	}, "commit", "-m", "Commit on 2026-10-02")

	ts, reqs := newMockLLMServer(t)
	defer ts.Close()

	cfg := setupTestConfig(t, repoDir, ts.URL)
	cfg.Vault.IndexFile = "Meta/Dev-Index.md"
	cfg.Vault.ProjectsDir = "Dev/Projects"

	opts := PipelineOptions{
		Date:   "2026-10-02",
		DryRun: true,
		Now:    func() time.Time { return now },
		Loc:    loc,
	}

	if err := Run(cfg, opts); err != nil {
		t.Fatalf("dry-run day mode failed: %v", err)
	}
	if len(*reqs) != 0 {
		t.Errorf("expected 0 LLM calls, got %d", len(*reqs))
	}

	projectName := collector.ProjectName(repoDir)
	exists, _, _ := vault.DevlogExists(cfg.Vault.Path, projectName, "2026-10-02", cfg.Vault.ProjectsDir)
	if exists {
		t.Errorf("expected note NOT to exist after dry-run")
	}

	// Legacy window dry-run
	legacyOpts := PipelineOptions{
		Window:   "24.hours.ago",
		IsWindow: true,
		DryRun:   true,
		Now:      func() time.Time { return now },
		Loc:      loc,
	}
	if err := Run(cfg, legacyOpts); err != nil {
		t.Fatalf("legacy dry-run failed: %v", err)
	}
}
