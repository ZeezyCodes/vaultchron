package pipeline

import (
	"strings"
	"testing"

	"github.com/ZeezyCodes/vaultchron/internal/collector"
)

// TestBuildPrompt_SystemPromptNonEmpty asserts that BuildPrompt returns a
// non-empty systemPrompt.
func TestBuildPrompt_SystemPromptNonEmpty(t *testing.T) {
	meta := &collector.RepoMetadata{
		Name:         "TestRepo",
		Path:         "/path/to/repo",
		Branch:       "main",
		Commits:      []string{"abc1234 Initial commit", "def5678 Fix bug"},
		CommitsCount: 2,
		Shortstat:    "3 files changed, 50 insertions(+), 10 deletions(-)",
		TopPackages:  []string{"internal/pkg", "cmd/app"},
		UnifiedDiff:  "diff --git a/file.go b/file.go\n+new line",
	}
	agentCtx := &collector.AgentContext{
		Goals:    []string{"Implement feature X"},
		Outlines: []string{"Complete feature X"},
		Prompts:  []string{"Build feature X"},
	}

	sysPrompt, _ := BuildPrompt(meta, agentCtx)

	if sysPrompt == "" {
		t.Fatal("systemPrompt should not be empty")
	}
}

// TestBuildPrompt_SystemPromptContainsCallouts asserts that the systemPrompt
// explicitly contains all five v3 callout markers.
func TestBuildPrompt_SystemPromptContainsCallouts(t *testing.T) {
	meta := &collector.RepoMetadata{
		Name:         "TestRepo",
		Path:         "/path/to/repo",
		Branch:       "main",
		Commits:      []string{"abc1234 Initial commit"},
		CommitsCount: 1,
		Shortstat:    "1 file changed, 10 insertions(+)",
		TopPackages:  []string{"internal/pkg"},
		UnifiedDiff:  "diff --git a/file.go b/file.go\n+new line",
	}
	agentCtx := &collector.AgentContext{}

	sysPrompt, _ := BuildPrompt(meta, agentCtx)

	requiredCallouts := []string{"[!abstract]", "[!info]", "[!bug]", "[!warning]", "[!check]"}
	for _, callout := range requiredCallouts {
		if !strings.Contains(sysPrompt, callout) {
			t.Errorf("systemPrompt should contain %q", callout)
		}
	}
}

// TestBuildPrompt_UserPromptContainsTelemetry asserts that the userPrompt
// contains commit messages, churn stat, and diff contents.
func TestBuildPrompt_UserPromptContainsTelemetry(t *testing.T) {
	meta := &collector.RepoMetadata{
		Name:         "TestRepo",
		Path:         "/path/to/repo",
		Branch:       "main",
		Commits:      []string{"abc1234 Initial commit", "def5678 Fix bug"},
		CommitsCount: 2,
		Shortstat:    "3 files changed, 50 insertions(+), 10 deletions(-)",
		TopPackages:  []string{"internal/pkg", "cmd/app"},
		UnifiedDiff:  "diff --git a/file.go b/file.go\n+new line",
	}
	agentCtx := &collector.AgentContext{
		Goals:    []string{"Implement feature X"},
		Outlines: []string{"Complete feature X"},
		Prompts:  []string{"Build feature X"},
	}

	_, userPrompt := BuildPrompt(meta, agentCtx)

	// Assert userPrompt contains commit messages.
	for _, c := range meta.Commits {
		if !strings.Contains(userPrompt, c) {
			t.Errorf("userPrompt should contain commit message %q", c)
		}
	}

	// Assert userPrompt contains churn stat.
	if !strings.Contains(userPrompt, meta.Shortstat) {
		t.Errorf("userPrompt should contain churn stat %q", meta.Shortstat)
	}

	// Assert userPrompt contains diff contents.
	if !strings.Contains(userPrompt, meta.UnifiedDiff) {
		t.Error("userPrompt should contain diff contents")
	}
}

// TestBuildDayPrompt asserts that day mode prompt names the day and partial status,
// contains no "24 hours", and caps commit messages at 200 with an omission count line.
func TestBuildDayPrompt(t *testing.T) {
	t.Run("partial day names day and has no 24 hours", func(t *testing.T) {
		meta := &collector.RepoMetadata{
			Name:         "AcmeApp",
			Path:         "/path/to/acme",
			Branch:       "feature/w9b",
			Commits:      []string{"c1 Commit one", "c2 Commit two"},
			CommitsCount: 2,
			Shortstat:    "1 file changed, 5 insertions(+)",
			TopPackages:  []string{"pkg/api"},
			UnifiedDiff:  "+added code",
		}
		sysPrompt, userPrompt := BuildDayPrompt(meta, "2026-10-03", true)
		if sysPrompt == "" {
			t.Fatal("expected non-empty sysPrompt")
		}
		if !strings.Contains(userPrompt, "2026-10-03") {
			t.Errorf("expected userPrompt to contain date 2026-10-03, got:\n%s", userPrompt)
		}
		if !strings.Contains(userPrompt, "Partial day") || !strings.Contains(userPrompt, "partial, day not over") {
			t.Errorf("expected userPrompt to mention partial day, got:\n%s", userPrompt)
		}
		if strings.Contains(strings.ToLower(userPrompt), "24 hours") {
			t.Errorf("userPrompt should not mention '24 hours':\n%s", userPrompt)
		}
	})

	t.Run("complete day names day and has no 24 hours", func(t *testing.T) {
		meta := &collector.RepoMetadata{
			Name:         "AcmeApp",
			Path:         "/path/to/acme",
			Branch:       "main",
			Commits:      []string{"c1 Commit one"},
			CommitsCount: 1,
			Shortstat:    "1 file changed, 1 insertion(+)",
			TopPackages:  []string{"pkg/api"},
			UnifiedDiff:  "+added code",
		}
		_, userPrompt := BuildDayPrompt(meta, "2026-09-30", false)
		if !strings.Contains(userPrompt, "2026-09-30") {
			t.Errorf("expected userPrompt to contain date 2026-09-30")
		}
		if !strings.Contains(userPrompt, "Complete day") {
			t.Errorf("expected userPrompt to mention complete day")
		}
		if strings.Contains(strings.ToLower(userPrompt), "24 hours") {
			t.Errorf("userPrompt should not mention '24 hours'")
		}
	})

	t.Run("commits capped at 200 with omission line", func(t *testing.T) {
		var commits []string
		for i := 1; i <= 250; i++ {
			commits = append(commits, "commit message")
		}
		meta := &collector.RepoMetadata{
			Name:         "AcmeApp",
			Path:         "/path/to/acme",
			Branch:       "main",
			Commits:      commits,
			CommitsCount: len(commits),
			Shortstat:    "50 files changed",
			TopPackages:  []string{"pkg/api"},
		}
		_, userPrompt := BuildDayPrompt(meta, "2026-09-30", false)
		omissionLine := "... 50 more commits omitted"
		if !strings.Contains(userPrompt, omissionLine) {
			t.Errorf("expected userPrompt to contain %q, got:\n%s", omissionLine, userPrompt)
		}
		if !strings.Contains(userPrompt, "Commits: 250") {
			t.Errorf("expected userPrompt to preserve true commit count 250, got:\n%s", userPrompt)
		}
	})
}
