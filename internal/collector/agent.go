package collector

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ZeezyCodes/vaultchron/internal/config"
)

// AgentContext fields categorize the extracted high-level session metadata:
//   - Goals:   from JSON "goal" fields or "Goal:" labels.
//   - Outlines: from JSON "outcome" fields or "Outcome:"/"Result:" labels.
//   - Prompts:  from JSON "task"/"user_prompt" fields or "Task:"/"Prompt:"/"Plan:" labels.
//
// These regex patterns extract high-level session metadata.

var (
	// jsonGoalPattern extracts the "goal" JSON string value.
	jsonGoalPattern = regexp.MustCompile(`"goal"\s*:\s*"([^"]{5,})"`)
	// jsonOutcomePattern extracts the "outcome" JSON string value.
	jsonOutcomePattern = regexp.MustCompile(`"outcome"\s*:\s*"([^"]{5,})"`)
	// jsonTaskPattern extracts the "task" JSON string value.
	jsonTaskPattern = regexp.MustCompile(`"task"\s*:\s*"([^"]{5,})"`)
	// jsonUserPromptPattern extracts the "user_prompt" JSON string value.
	jsonUserPromptPattern = regexp.MustCompile(`"user_prompt"\s*:\s*"([^"]{5,})"`)

	// labelLinePattern matches labeled lines: Goal:, Task:, Outcome:, etc.
	labelLinePattern = regexp.MustCompile(
		`^(Goal|Task|Outcome|Prompt|Plan|Result)\s*[:\\-]\s*(.+)$`,
	)

	// noisePattern filters out system/error lines that contain common noise tokens.
	noisePattern = regexp.MustCompile(`(?i)(error|stderr|stdout|warning|traceback|exception|command)`)
)

// HarvestAgentContext reads recent JSON/log files modified within the past
// 24 hours (since cutoff) under configured log paths, extracts
// high-level session metadata (goals, outcomes, prompts) using regular
// expressions matching session log patterns,
// deduplicates entries, and returns them categorized in an AgentContext.
func HarvestAgentContext(cfg config.AgentLogsConfig, since time.Time) (*AgentContext, error) {
	ctx := &AgentContext{
		Goals:    []string{},
		Outlines: []string{},
		Prompts:  []string{},
	}
	paths := []string{cfg.AntigravityPath, cfg.PoolsidePath}
	for _, p := range paths {
		expanded := expandPath(p)
		harvestFromDir(expanded, since, ctx)
	}

	// Deduplicate while preserving first-seen order.
	ctx.Goals = deduplicate(ctx.Goals)
	ctx.Outlines = deduplicate(ctx.Outlines)
	ctx.Prompts = deduplicate(ctx.Prompts)

	return ctx, nil
}

// expandPath expands a leading ~ to the user's home directory and expands
// environment variables in the path.
func expandPath(p string) string {
	return config.ExpandPath(p)
}

// harvestFromDir walks a directory tree and extracts agent context from
// files modified after the since cutoff.
func harvestFromDir(dir string, since time.Time, ctx *AgentContext) {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return
	}

	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}

		fi, err := d.Info()
		if err != nil {
			return nil
		}
		if fi.ModTime().Before(since) {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		extractAgentData(string(content), ctx)
		return nil
	})
}

// extractAgentData parses JSON fields and labeled lines from file content,
// categorizing them into the AgentContext fields.
func extractAgentData(content string, ctx *AgentContext) {
	// JSON field extraction.
	for _, match := range jsonGoalPattern.FindAllStringSubmatch(content, -1) {
		ctx.Goals = append(ctx.Goals, match[1])
	}
	for _, match := range jsonOutcomePattern.FindAllStringSubmatch(content, -1) {
		ctx.Outlines = append(ctx.Outlines, match[1])
	}
	for _, match := range jsonTaskPattern.FindAllStringSubmatch(content, -1) {
		ctx.Prompts = append(ctx.Prompts, match[1])
	}
	for _, match := range jsonUserPromptPattern.FindAllStringSubmatch(content, -1) {
		ctx.Prompts = append(ctx.Prompts, match[1])
	}

	// Labeled line extraction.
	for _, line := range strings.Split(content, "\n") {
		match := labelLinePattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		label := strings.ToLower(match[1])
		text := strings.TrimSpace(match[2])
		if len(text) > 200 {
			text = safeTruncate(text, 200)
		}

		// Skip noise lines.
		if noisePattern.MatchString(text) {
			continue
		}

		switch label {
		case "goal":
			ctx.Goals = append(ctx.Goals, text)
		case "outcome", "result":
			ctx.Outlines = append(ctx.Outlines, text)
		case "task", "prompt", "plan":
			ctx.Prompts = append(ctx.Prompts, text)
		}
	}
}

// deduplicate removes duplicate strings while preserving first-seen order.
func deduplicate(items []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}
