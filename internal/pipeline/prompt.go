package pipeline

import (
	"fmt"
	"strings"

	"github.com/ZeezyCodes/vaultchron/internal/collector"
)

// systemPrompt is the LLM system instruction that enforces the Vault Callout v3
// taxonomy. The template already renders the title and telemetry callout, so the LLM
// should output ONLY the five structural callout sections.
const systemPrompt = `You are a principal engineer writing a technical journal entry for an Obsidian vault.

The YAML frontmatter, breadcrumb navigation, title, and telemetry callout have ALREADY been rendered above. Your task is to output ONLY the following five Obsidian callout sections in this exact order. Do NOT include any title, frontmatter, telemetry block, or preamble of your own. Use NO markdown headings (#, ##, ###) inside callout bodies — use bold inline headers (**Title**) or bullet lists (- item) instead.

Here is the EXACT output format required:

> [!abstract] Architectural Evolution & Design Decisions
> <content: system boundary changes, architectural patterns, pipeline topology>

> [!info] Data Contracts & Interface Shifts
> <content: struct changes, schema updates, function signatures, Go/HTMX interfaces>

> [!bug] Regressions & Defect Remediations
> <content: DEF-XXX tracking, edge cases, incident bugs>

> [!warning] Immediate Action Items & Operational Checklists
> - [ ] <action item 1>
> - [ ] <action item 2>

> [!check] Verification & Test Suite Status
> <content: golden test runs, schema version bumps, assertions>

Rules:
- You MUST produce all five callouts in the order shown above.
- ALL action items under [!warning] MUST be formatted as - [ ] checkboxes.
- Whenever mentioning tracked ecosystem projects by name, format as wikilinks: [[Projects/<Name>/Overview|<Name>]]
- Include Mermaid code blocks when system architecture changed.
- Write in Obsidian-flavored markdown. Be concise but thorough.`

// SystemPrompt returns the system prompt formatted with the given projects directory wikilink pattern.
// If projectsDir is empty or "Projects", it returns the exact v0.2.0 systemPrompt constant.
func SystemPrompt(projectsDir ...string) string {
	pDir := "Projects"
	if len(projectsDir) > 0 && projectsDir[0] != "" {
		pDir = projectsDir[0]
	}
	if pDir == "Projects" {
		return systemPrompt
	}
	return strings.Replace(
		systemPrompt,
		"[[Projects/<Name>/Overview|<Name>]]",
		fmt.Sprintf("[[%s/<Name>/Overview|<Name>]]", pDir),
		1,
	)
}

// BuildPrompt constructs the system and user prompts for the LLM. The system
// prompt enforces the Vault Callout v3 taxonomy. The user prompt injects real
// telemetry (branch, churn, top packages), commit history, agent session context,
// and a capped unified diff.
func BuildPrompt(meta *collector.RepoMetadata, agentCtx *collector.AgentContext, projectsDir ...string) (string, string) {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# Technical Journal — %s\n\n", meta.Name))
	b.WriteString(fmt.Sprintf("Repository: %s\n", meta.Path))
	b.WriteString(fmt.Sprintf("Active Branch: `%s`\n", meta.Branch))
	b.WriteString(fmt.Sprintf("Commits in window: %d\n", meta.CommitsCount))
	b.WriteString(fmt.Sprintf("Churn: %s\n", meta.Shortstat))
	b.WriteString(fmt.Sprintf("Top Packages: %s\n", formatTopPackages(meta.TopPackages)))

	b.WriteString("\n## Commit History\n\n")
	if len(meta.Commits) > 0 {
		for _, c := range meta.Commits {
			b.WriteString(fmt.Sprintf("- %s\n", c))
		}
	} else {
		b.WriteString("- *(no commits in window)*\n")
	}

	// Agent context
	if agentCtx != nil && (len(agentCtx.Goals) > 0 || len(agentCtx.Outlines) > 0 || len(agentCtx.Prompts) > 0) {
		b.WriteString("\n## Agent Session Context\n\n")
		for _, g := range agentCtx.Goals {
			b.WriteString(fmt.Sprintf("- **Goal:** %s\n", g))
		}
		for _, o := range agentCtx.Outlines {
			b.WriteString(fmt.Sprintf("- **Outcome:** %s\n", o))
		}
		for _, p := range agentCtx.Prompts {
			b.WriteString(fmt.Sprintf("- **Task:** %s\n", p))
		}
	}

	// Capped unified diff
	b.WriteString(fmt.Sprintf("\n## Code Diff (diff capped at %d chars)\n\n", collector.MaxDiffChars()))
	if meta.UnifiedDiff != "" {
		b.WriteString("```diff\n")
		b.WriteString(meta.UnifiedDiff)
		b.WriteString("\n```\n")
	} else {
		b.WriteString("*(no changes in window)*\n")
	}

	b.WriteString("\n## Instructions\n\n")
	b.WriteString("Write a technical devlog for " + meta.Name + " dated today.\n")
	b.WriteString("Active branch: `" + meta.Branch + "`. Churn: " + meta.Shortstat + ".\n")
	b.WriteString("Top packages: " + formatTopPackages(meta.TopPackages) + ".\n\n")
	b.WriteString("Output ONLY the five callout sections in the exact format specified in the system prompt:\n")
	b.WriteString("[!abstract] -> [!info] -> [!bug] -> [!warning] (with - [ ] checkboxes) -> [!check].\n")
	b.WriteString("Do NOT include YAML frontmatter, breadcrumb, title, or telemetry callout — they are already rendered.\n")
	b.WriteString("Do NOT use # ## ### headings inside callout bodies — use **bold inline headers** or bullet lists.\n")

	return SystemPrompt(projectsDir...), b.String()
}

func formatTopPackages(pkgs []string) string {
	if len(pkgs) == 0 {
		return "N/A"
	}
	return strings.Join(pkgs, ", ")
}

// BuildDayPrompt constructs the system and user prompts for day-based devlogs.
// It explicitly states that the note covers the local calendar day dateStr and
// whether the day is partial (not yet complete).
// Commit lines are capped at 200 followed by an omission summary line if exceeded.
func BuildDayPrompt(meta *collector.RepoMetadata, dateStr string, partial bool, projectsDir ...string) (string, string) {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# Technical Journal — %s\n\n", meta.Name))
	b.WriteString(fmt.Sprintf("Repository: %s\n", meta.Path))
	b.WriteString(fmt.Sprintf("Active Branch: `%s`\n", meta.Branch))
	b.WriteString(fmt.Sprintf("Date: %s\n", dateStr))
	if partial {
		b.WriteString("Status: Partial day (in progress)\n")
	} else {
		b.WriteString("Status: Complete day\n")
	}
	b.WriteString(fmt.Sprintf("Commits: %d\n", meta.CommitsCount))
	b.WriteString(fmt.Sprintf("Churn: %s\n", meta.Shortstat))
	b.WriteString(fmt.Sprintf("Top Packages: %s\n", formatTopPackages(meta.TopPackages)))

	b.WriteString("\n## Commit History\n\n")
	if len(meta.Commits) > 0 {
		commitLimit := 200
		if len(meta.Commits) <= commitLimit {
			for _, c := range meta.Commits {
				b.WriteString(fmt.Sprintf("- %s\n", c))
			}
		} else {
			for i := 0; i < commitLimit; i++ {
				b.WriteString(fmt.Sprintf("- %s\n", meta.Commits[i]))
			}
			omitted := len(meta.Commits) - commitLimit
			b.WriteString(fmt.Sprintf("... %d more commits omitted\n", omitted))
		}
	} else {
		b.WriteString("- *(no commits in day)*\n")
	}

	// Capped unified diff
	b.WriteString(fmt.Sprintf("\n## Code Diff (diff capped at %d chars)\n\n", collector.MaxDiffChars()))
	if meta.UnifiedDiff != "" {
		b.WriteString("```diff\n")
		b.WriteString(meta.UnifiedDiff)
		b.WriteString("\n```\n")
	} else {
		b.WriteString("*(no changes in day)*\n")
	}

	b.WriteString("\n## Instructions\n\n")
	if partial {
		b.WriteString(fmt.Sprintf("Write a technical devlog for %s covering local calendar day %s (partial, day not over).\n", meta.Name, dateStr))
	} else {
		b.WriteString(fmt.Sprintf("Write a technical devlog for %s covering local calendar day %s (complete day).\n", meta.Name, dateStr))
	}
	b.WriteString("Active branch: `" + meta.Branch + "`. Churn: " + meta.Shortstat + ".\n")
	b.WriteString("Top packages: " + formatTopPackages(meta.TopPackages) + ".\n\n")
	b.WriteString("Output ONLY the five callout sections in the exact format specified in the system prompt:\n")
	b.WriteString("[!abstract] -> [!info] -> [!bug] -> [!warning] (with - [ ] checkboxes) -> [!check].\n")
	b.WriteString("Do NOT include YAML frontmatter, breadcrumb, title, or telemetry callout — they are already rendered.\n")
	b.WriteString("Do NOT use # ## ### headings inside callout bodies — use **bold inline headers** or bullet lists.\n")

	return SystemPrompt(projectsDir...), b.String()
}
