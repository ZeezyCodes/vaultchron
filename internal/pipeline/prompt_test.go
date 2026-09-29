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
