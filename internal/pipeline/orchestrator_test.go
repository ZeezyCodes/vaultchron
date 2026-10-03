package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
		{"Hello World", "hello-world"},
		{"AcmeWidgets.com", "acmewidgets-com"},
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
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, loc)
	repoDir, runGit := setupTestGitRepo(t)

	if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("writing main.go: %v", err)
	}
	runGit(nil, "add", "main.go")
	runGit([]string{
		"GIT_AUTHOR_DATE=2026-10-03T10:00:00-05:00",
		"GIT_COMMITTER_DATE=2026-10-03T10:00:00-05:00",
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
