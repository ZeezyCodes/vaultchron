package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/ZeezyCodes/vaultchron/internal/collector"
	"github.com/ZeezyCodes/vaultchron/internal/config"
	"github.com/ZeezyCodes/vaultchron/internal/llm"
	"github.com/ZeezyCodes/vaultchron/internal/vault"
)

// harvestAgentContextFn is a package variable to allow testing agent log harvesting without disk I/O.
var harvestAgentContextFn = collector.HarvestAgentContext

// defaultLLMBaseURL is the OpenAI-compatible Gemini endpoint used when no
// explicit base URL is configured.
const defaultLLMBaseURL = "https://generativelanguage.googleapis.com/v1beta/openai"

// PipelineOptions controls the pipeline execution mode.
type PipelineOptions struct {
	// Window is the git time window (e.g. "24.hours.ago") passed to
	// git log --since and used as HEAD@{<window>} for diff references.
	Window string
	// RepoFilter, when non-empty, restricts processing to a single
	// repository whose base directory name matches.
	RepoFilter string
	// Force processes repositories even if they have zero commits in the
	// window.
	Force bool
	// DryRun skips LLM calls and disk writes. Instead, renders a populated
	// DevlogData preview to stdout.
	DryRun bool
}

// RepoResult records the outcome of processing a single repository in the
// execution summary.
type RepoResult struct {
	Name    string // project name (e.g. "AcmeWidgets.com")
	Status  string // "WRITTEN", "SKIPPED", "ERROR"
	Commits int    // number of commits in the window
	Output  string // output path or reason
}

// Run executes the full devlog generation pipeline: discover repositories,
// harvest git telemetry and agent context, then for each matching repo
// either render a dry-run preview or call the LLM and write a devlog.
//
// Error containment: if diff extraction, LLM synthesis, or disk I/O fails for
// a single repo, the error is logged with [ERROR] <repo>: <err>, recorded in
// the execution summary, and processing continues for remaining repositories.
//
// Returns a non-nil error only if all active repos fail or if critical
// configuration/scan errors occur.
func Run(cfg *config.Config, opts PipelineOptions) error {
	// Top-level context with a 10-minute timeout for the entire run to prevent
	// hung network calls from blocking background timers or cron schedules.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Phase 1: discover repositories.
	repos, err := collector.DiscoverRepositories(ctx, cfg.Scan.Roots, cfg.Scan.MaxDepth, cfg.Scan.Excludes)
	if err != nil {
		return fmt.Errorf("discovering repositories: %w", err)
	}

	// Phase 2: harvest agent context once for the active window if enabled.
	now := time.Now()
	var agentCtx *collector.AgentContext
	if cfg.AgentLogs.Enabled {
		since := now.Add(-24 * time.Hour)
		var harvestErr error
		agentCtx, harvestErr = harvestAgentContextFn(cfg.AgentLogs, since)
		if harvestErr != nil {
			fmt.Fprintf(os.Stderr, "[WARN] agent context harvest failed: %v\n", harvestErr)
			agentCtx = &collector.AgentContext{}
		}
	} else {
		agentCtx = &collector.AgentContext{}
	}

	var llmClient *llm.Client
	if !opts.DryRun {
		baseURL := cfg.LLM.BaseURL
		if baseURL == "" {
			baseURL = defaultLLMBaseURL
		}
		llmClient = llm.New(cfg.LLM.Waterfall, baseURL, cfg.LLM.APIKeyEnv, 120*time.Second)
	}

	today := now.Format("2006-01-02")
	nowStr := now.Format("2006-01-02 15:04:05")

	var results []RepoResult
	activeCount := 0
	failedCount := 0

	for _, repoPath := range repos {
		// Apply repo filter.
		if opts.RepoFilter != "" {
			if filepath.Base(repoPath) != opts.RepoFilter && repoPath != opts.RepoFilter {
				continue
			}
		}

		repoName := filepath.Base(repoPath)

		// Phase 2: collect git telemetry + diffs.
		meta, err := collector.HarvestRepoMetadata(ctx, repoPath, opts.Window)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] %s: %v\n", repoName, err)
			results = append(results, RepoResult{
				Name:    repoName,
				Status:  "ERROR",
				Commits: 0,
				Output:  err.Error(),
			})
			failedCount++
			continue
		}

		// Handle nil metadata (zero commits in window).
		if meta == nil {
			if !opts.Force {
				results = append(results, RepoResult{
					Name:    repoName,
					Status:  "SKIPPED",
					Commits: 0,
					Output:  "No commits in window",
				})
				continue
			}
			// Force mode: retrieve minimal metadata via ScanRepository.
			scanResult, scanErr := collector.ScanRepository(ctx, repoPath, opts.Window)
			if scanErr != nil || scanResult == nil {
				results = append(results, RepoResult{
					Name:    repoName,
					Status:  "SKIPPED",
					Commits: 0,
					Output:  "No commits in window",
				})
				continue
			}
			meta = &collector.RepoMetadata{
				Name:         scanResult.Name,
				Path:         scanResult.Path,
				Branch:       scanResult.Branch,
				Commits:      []string{},
				CommitsCount: 0,
				Shortstat:    "-",
				TopPackages:  scanResult.TopPackages,
				UnifiedDiff:  "",
			}
		}

		activeCount++
		slug, lang := getProjectTags(cfg, meta.Name)

		data := vault.DevlogData{
			Date:         today,
			ProjectName:  meta.Name,
			Slug:         slug,
			Lang:         lang,
			Branch:       meta.Branch,
			CommitsCount: meta.CommitsCount,
			Shortstat:    meta.Shortstat,
			TopPackages:  meta.TopPackages,
			Now:          nowStr,
		}

		if opts.DryRun {
			// Phase 4 (dry-run): render preview with placeholder content.
			data.Model = "dry-run"
			data.Content = buildDryRunContent(meta, opts.Window)
			rendered, renderErr := vault.RenderDevlog(data)
			if renderErr != nil {
				fmt.Fprintf(os.Stderr, "[ERROR] %s: render failed: %v\n", meta.Name, renderErr)
				results = append(results, RepoResult{
					Name:    meta.Name,
					Status:  "ERROR",
					Commits: meta.CommitsCount,
					Output:  fmt.Sprintf("render failed: %v", renderErr),
				})
				failedCount++
				continue
			}
			fmt.Println(rendered)
			results = append(results, RepoResult{
				Name:    meta.Name,
				Status:  "WRITTEN",
				Commits: meta.CommitsCount,
				Output:  fmt.Sprintf("Projects/%s/Devlog/%s.md", meta.Name, today),
			})
		} else {
			// Phase 4-6 (standard): build prompt, call LLM, write devlog.
			sysPrompt, userPrompt := BuildPrompt(meta, agentCtx)
			content, model, llmErr := llmClient.Call(ctx, sysPrompt, userPrompt)
			if llmErr != nil {
				fmt.Fprintf(os.Stderr, "[ERROR] %s: LLM generation failed: %v\n", meta.Name, llmErr)
				results = append(results, RepoResult{
					Name:    meta.Name,
					Status:  "ERROR",
					Commits: meta.CommitsCount,
					Output:  fmt.Sprintf("LLM generation failed: %v", llmErr),
				})
				failedCount++
				continue
			}
			data.Model = model
			data.Content = content
			path, writeErr := vault.WriteDevlog(cfg.Vault.Path, meta.Name, today, data)
			if writeErr != nil {
				fmt.Fprintf(os.Stderr, "[ERROR] %s: write failed: %v\n", meta.Name, writeErr)
				results = append(results, RepoResult{
					Name:    meta.Name,
					Status:  "ERROR",
					Commits: meta.CommitsCount,
					Output:  fmt.Sprintf("write failed: %v", writeErr),
				})
				failedCount++
				continue
			}
			fmt.Printf("  Devlog written: %s (%s)\n", meta.Name, model)

			relPath, _ := filepath.Rel(cfg.Vault.Path, path)
			results = append(results, RepoResult{
				Name:    meta.Name,
				Status:  "WRITTEN",
				Commits: meta.CommitsCount,
				Output:  relPath,
			})

			if idxErr := vault.UpdateIndex(cfg.Vault, &data); idxErr != nil {
				fmt.Fprintf(os.Stderr, "[ERROR] %s: index update failed: %v\n", meta.Name, idxErr)
			}
		}
	}

	// Emit structured terminal summary table.
	printSummaryTable(results)

	// Return non-zero exit code only if all active repos fail or if
	// critical configuration/scan errors occur.
	if activeCount > 0 && failedCount == activeCount {
		return fmt.Errorf("all %d active repository(ies) failed", activeCount)
	}

	return nil
}

// printSummaryTable renders a structured terminal summary of all repository
// processing results.
func printSummaryTable(results []RepoResult) {
	if len(results) == 0 {
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "Repository\tStatus\tCommits\tOutput / Reason")
	fmt.Fprintln(w, "----------\t------\t-------\t---------------")

	for _, r := range results {
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", r.Name, r.Status, r.Commits, r.Output)
	}
	w.Flush()
}

// getProjectTags maps a project name to its slug and language from optional
// config mappings, falling back to a slugified name and "go" language for
// unmapped projects.
func getProjectTags(cfg *config.Config, projectName string) (slug, lang string) {
	if cfg != nil && cfg.ProjectTags != nil {
		if tag, ok := cfg.ProjectTags[projectName]; ok {
			return tag.Slug, tag.Lang
		}
	}
	slug = vault.Slugify(projectName)
	lang = "go"
	return slug, lang
}

// buildDryRunContent constructs placeholder v3 callout content for dry-run
// mode, including the commit history, diff, and empty callout sections so the
// user can preview the full devlog structure without an LLM call.
func buildDryRunContent(meta *collector.RepoMetadata, window string) string {
	var b strings.Builder

	// Commit history.
	b.WriteString("**Commit History**\n\n")
	if len(meta.Commits) > 0 {
		for _, c := range meta.Commits {
			b.WriteString(fmt.Sprintf("- %s\n", c))
		}
	} else {
		b.WriteString("- *(no commits in window)*\n")
	}
	b.WriteString("\n")

	// Code diff.
	b.WriteString(fmt.Sprintf("**Code Diff (window: %s)**\n\n", window))
	if meta.UnifiedDiff != "" {
		b.WriteString("```diff\n")
		b.WriteString(meta.UnifiedDiff)
		b.WriteString("\n```\n\n")
	} else {
		b.WriteString("*(no changes in window)*\n\n")
	}

	// v3 callout placeholders.
	b.WriteString("> [!abstract] Architectural Evolution & Design Decisions\n")
	b.WriteString("> *(dry-run: awaiting LLM synthesis)*\n\n")

	b.WriteString("> [!info] Data Contracts & Interface Shifts\n")
	b.WriteString("> *(dry-run: awaiting LLM synthesis)*\n\n")

	b.WriteString("> [!bug] Regressions & Defect Remediations\n")
	b.WriteString("> *(dry-run: awaiting LLM synthesis)*\n\n")

	b.WriteString("> [!warning] Immediate Action Items & Operational Checklists\n")
	b.WriteString("> - [ ] *(dry-run: no LLM call made)*\n\n")

	b.WriteString("> [!check] Verification & Test Suite Status\n")
	b.WriteString("> *(dry-run: awaiting LLM synthesis)*\n")

	return b.String()
}
