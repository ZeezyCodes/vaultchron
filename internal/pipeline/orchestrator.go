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
	// Day mode flags
	Date    string // exact day (YYYY-MM-DD)
	From    string // inclusive start day (YYYY-MM-DD)
	To      string // inclusive end day (YYYY-MM-DD, defaults to yesterday)
	CatchUp int    // lookback in calendar days (> 0 overrides scan.catch_up_days)
	// MaxCalls is the max LLM calls per run. When Visited is nil, only MaxCalls > 0
	// overrides llm.max_calls_per_run; pass Visited with "max-calls": true to set explicit 0 (unlimited).
	MaxCalls int

	// Window is the git time window (e.g. "24.hours.ago") passed to
	// git log --since and used as HEAD@{<window>} for diff references.
	Window   string
	IsWindow bool // true if -window was explicitly visited on CLI

	// RepoFilter, when non-empty, restricts processing to a single
	// repository whose base directory name matches.
	RepoFilter string
	// Force overwrites existing devlogs in day mode, or processes repositories
	// with zero commits in legacy window mode.
	Force bool
	// DryRun skips LLM calls and disk writes.
	DryRun bool
	// Scan runs collector only without writing or LLM synthesis.
	Scan bool

	// Visited tracks explicitly set flags from flag.Visit.
	Visited map[string]bool

	// Clock and location injection (tests inject mock clock and fixed zone;
	// cmd/vaultchron injects time.Now and time.Local).
	Now func() time.Time
	Loc *time.Location
}

// ExecutionPlan represents the fully resolved and validated execution parameters.
type ExecutionPlan struct {
	IsLegacyWindow bool
	Window         string
	Days           []collector.DayWindow
	MaxCalls       int // 0 = unlimited
	RepoFilter     string
	Force          bool
	DryRun         bool
	Scan           bool
	Now            time.Time
	Loc            *time.Location
}

// ResolvePlan resolves and validates CLI options, configuration, and time into
// an ExecutionPlan. Returns an error if flag combinations are invalid or dates are in the future.
func ResolvePlan(opts PipelineOptions, cfg *config.Config, now time.Time, loc *time.Location) (*ExecutionPlan, error) {
	if loc == nil {
		loc = time.Local
	}

	visited := opts.Visited
	if visited == nil {
		visited = make(map[string]bool)
		if opts.Date != "" {
			visited["date"] = true
		}
		if opts.From != "" {
			visited["from"] = true
		}
		if opts.To != "" {
			visited["to"] = true
		}
		if opts.IsWindow || opts.Window != "" {
			visited["window"] = true
		}
		if opts.CatchUp > 0 {
			visited["catch-up"] = true
		}
		if opts.MaxCalls > 0 {
			visited["max-calls"] = true
		}
	}

	// Validation rule: -date cannot be combined with -from, -to, -window, or -catch-up
	if visited["date"] && (visited["from"] || visited["to"] || visited["window"] || visited["catch-up"]) {
		return nil, fmt.Errorf("cannot combine -date with -from, -to, -window, or -catch-up")
	}

	// Validation rule: -from/-to cannot be combined with -window or -catch-up
	if (visited["from"] || visited["to"]) && (visited["window"] || visited["catch-up"]) {
		return nil, fmt.Errorf("cannot combine -from/-to with -window or -catch-up")
	}

	// Validation rule: -to without -from is an error
	if visited["to"] && !visited["from"] {
		return nil, fmt.Errorf("-to flag requires -from")
	}

	// Validation rule: -catch-up with -window is an error
	if visited["catch-up"] && visited["window"] {
		return nil, fmt.Errorf("cannot combine -catch-up with -window")
	}

	// Validation rule: -catch-up < 1 is an error
	if visited["catch-up"] && opts.CatchUp < 1 {
		return nil, fmt.Errorf("invalid -catch-up %d: must be at least 1", opts.CatchUp)
	}

	// Validation rule: -max-calls < 0 is an error
	if visited["max-calls"] && opts.MaxCalls < 0 {
		return nil, fmt.Errorf("invalid -max-calls %d: must be non-negative", opts.MaxCalls)
	}

	maxCalls := cfg.LLM.MaxCallsPerRunVal()
	if visited["max-calls"] {
		maxCalls = opts.MaxCalls
	}

	// Legacy window mode
	if visited["window"] {
		win := opts.Window
		if win == "" {
			win = "24.hours.ago"
		}
		return &ExecutionPlan{
			IsLegacyWindow: true,
			Window:         win,
			MaxCalls:       maxCalls,
			RepoFilter:     opts.RepoFilter,
			Force:          opts.Force,
			DryRun:         opts.DryRun,
			Scan:           opts.Scan,
			Now:            now,
			Loc:            loc,
		}, nil
	}

	// Day mode: -date D
	if visited["date"] {
		w, err := collector.NewDayWindow(opts.Date, loc, now)
		if err != nil {
			return nil, err
		}
		return &ExecutionPlan{
			IsLegacyWindow: false,
			Days:           []collector.DayWindow{w},
			MaxCalls:       maxCalls,
			RepoFilter:     opts.RepoFilter,
			Force:          opts.Force,
			DryRun:         opts.DryRun,
			Scan:           opts.Scan,
			Now:            now,
			Loc:            loc,
		}, nil
	}

	// Day mode: -from A [-to B]
	if visited["from"] {
		if _, err := collector.NewDayWindow(opts.From, loc, now); err != nil {
			return nil, err
		}

		toDate := opts.To
		if !visited["to"] {
			y, m, d := now.In(loc).Date()
			yesterday := time.Date(y, m, d, 0, 0, 0, 0, loc).AddDate(0, 0, -1)
			toDate = yesterday.Format("2006-01-02")
		}

		if _, err := collector.NewDayWindow(toDate, loc, now); err != nil {
			return nil, err
		}

		if opts.From > toDate {
			return nil, fmt.Errorf("invalid date range: -from %s is after -to %s", opts.From, toDate)
		}

		tFrom, _ := time.ParseInLocation("2006-01-02", opts.From, loc)
		tTo, _ := time.ParseInLocation("2006-01-02", toDate, loc)

		var days []collector.DayWindow
		for tCur := tFrom; !tCur.After(tTo); tCur = tCur.AddDate(0, 0, 1) {
			dStr := tCur.Format("2006-01-02")
			w, err := collector.NewDayWindow(dStr, loc, now)
			if err != nil {
				return nil, err
			}
			days = append(days, w)
		}

		return &ExecutionPlan{
			IsLegacyWindow: false,
			Days:           days,
			MaxCalls:       maxCalls,
			RepoFilter:     opts.RepoFilter,
			Force:          opts.Force,
			DryRun:         opts.DryRun,
			Scan:           opts.Scan,
			Now:            now,
			Loc:            loc,
		}, nil
	}

	// Day mode: Default (none of -date, -from, -to, -window)
	n := cfg.Scan.CatchUpDaysVal()
	if visited["catch-up"] {
		n = opts.CatchUp
	}

	y, m, d := now.In(loc).Date()
	todayMidnight := time.Date(y, m, d, 0, 0, 0, 0, loc)

	var days []collector.DayWindow
	for i := n; i >= 1; i-- {
		dayDate := todayMidnight.AddDate(0, 0, -i)
		dStr := dayDate.Format("2006-01-02")
		w, err := collector.NewDayWindow(dStr, loc, now)
		if err != nil {
			return nil, err
		}
		days = append(days, w)
	}

	return &ExecutionPlan{
		IsLegacyWindow: false,
		Days:           days,
		MaxCalls:       maxCalls,
		RepoFilter:     opts.RepoFilter,
		Force:          opts.Force,
		DryRun:         opts.DryRun,
		Scan:           opts.Scan,
		Now:            now,
		Loc:            loc,
	}, nil
}

// RepoResult records the outcome of processing a single repository in legacy window mode.
type RepoResult struct {
	Name    string // project name (e.g. "AcmeWidgets.com")
	Status  string // "WRITTEN", "SKIPPED", "ERROR"
	Commits int    // number of commits in the window
	Output  string // output path or reason
}

// DayResultRow represents one row in the day-mode summary table.
type DayResultRow struct {
	Repo    string
	Date    string
	Status  string
	Commits string
	Output  string
}

// DaySummaryCounts summarizes counts for day mode.
type DaySummaryCounts struct {
	Written      int
	Skipped      int
	Pending      int
	CapPending   int
	IndexPending int
	Errors       int
	ZeroCommits  int
}

// FormatDaySummaryLine formats the summary line text for day mode.
func FormatDaySummaryLine(counts DaySummaryCounts, dryRun bool) string {
	writtenWord := "written"
	if dryRun {
		writtenWord = "would be written"
	}
	var suffixes string
	if counts.CapPending > 0 {
		suffixes += " (call cap reached; run again to continue)"
	}
	if counts.IndexPending > 0 {
		suffixes += " (index error; fix the index and run again)"
	}
	return fmt.Sprintf("%d %s, %d skipped, %d pending%s, %d errors",
		counts.Written, writtenWord, counts.Skipped, counts.Pending, suffixes, counts.Errors)
}

// Run executes the devlog generation pipeline.
func Run(cfg *config.Config, opts PipelineOptions) error {
	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	now := nowFn()
	loc := opts.Loc
	if loc == nil {
		loc = time.Local
	}

	plan, err := ResolvePlan(opts, cfg, now, loc)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	_, _, err = RunPlan(ctx, cfg, plan)
	return err
}

// RunPlan executes a resolved execution plan, returning row details, summary counts,
// and an error if any step failed.
func RunPlan(ctx context.Context, cfg *config.Config, plan *ExecutionPlan) ([]DayResultRow, DaySummaryCounts, error) {
	// Discover repositories across scan roots.
	repos, err := collector.DiscoverRepositories(ctx, cfg.Scan.Roots, cfg.Scan.MaxDepth, cfg.Scan.Excludes)
	if err != nil {
		return nil, DaySummaryCounts{}, fmt.Errorf("discovering repositories: %w", err)
	}

	// Filter repositories if requested.
	if plan.RepoFilter != "" {
		var filtered []string
		for _, r := range repos {
			if filepath.Base(r) == plan.RepoFilter || r == plan.RepoFilter {
				filtered = append(filtered, r)
			}
		}
		repos = filtered
	}

	if plan.IsLegacyWindow {
		return runLegacyPlan(ctx, cfg, plan, repos)
	}

	return runDayPlan(ctx, cfg, plan, repos)
}

func runLegacyPlan(ctx context.Context, cfg *config.Config, plan *ExecutionPlan, repos []string) ([]DayResultRow, DaySummaryCounts, error) {
	var agentCtx *collector.AgentContext
	if cfg.AgentLogs.Enabled {
		since := plan.Now.Add(-24 * time.Hour)
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
	if !plan.DryRun {
		baseURL := cfg.LLM.BaseURL
		if baseURL == "" {
			baseURL = defaultLLMBaseURL
		}
		llmClient = llm.New(cfg.LLM.Waterfall, baseURL, cfg.LLM.APIKeyEnv, 120*time.Second)
	}

	today := plan.Now.Format("2006-01-02")
	nowStr := plan.Now.Format("2006-01-02 15:04:05")

	var results []RepoResult
	activeCount := 0
	failedCount := 0

	for _, repoPath := range repos {
		repoName := filepath.Base(repoPath)
		projName := collector.ProjectName(repoPath)

		// Skip-existing check in legacy mode unless -force
		exists, _, err := vault.DevlogExists(cfg.Vault.Path, projName, today)
		if err == nil && exists && !plan.Force {
			results = append(results, RepoResult{
				Name:    projName,
				Status:  "SKIPPED",
				Commits: 0,
				Output:  "note exists",
			})
			continue
		}

		meta, err := collector.HarvestRepoMetadata(ctx, repoPath, plan.Window)
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

		if meta == nil {
			if !plan.Force {
				results = append(results, RepoResult{
					Name:    repoName,
					Status:  "SKIPPED",
					Commits: 0,
					Output:  "No commits in window",
				})
				continue
			}
			scanResult, scanErr := collector.ScanRepository(ctx, repoPath, plan.Window)
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

		if plan.DryRun {
			data.Model = "dry-run"
			data.Content = buildDryRunContent(meta, plan.Window)
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

			if stubErr := vault.EnsureOverviewStub(cfg.Vault.Path, meta.Name, data.Slug); stubErr != nil {
				fmt.Fprintf(os.Stderr, "[WARN] %s: overview stub creation failed: %v\n", meta.Name, stubErr)
			}

			if idxErr := vault.BootstrapIndex(cfg.Vault); idxErr != nil {
				fmt.Fprintf(os.Stderr, "[ERROR] %s: index update failed: %v\n", meta.Name, idxErr)
			} else if idxErr := vault.UpdateIndex(cfg.Vault, &data); idxErr != nil {
				fmt.Fprintf(os.Stderr, "[ERROR] %s: index update failed: %v\n", meta.Name, idxErr)
			}
		}
	}

	printSummaryTable(results)

	if activeCount > 0 && failedCount == activeCount {
		return nil, DaySummaryCounts{}, fmt.Errorf("all %d active repository(ies) failed", activeCount)
	}

	return nil, DaySummaryCounts{}, nil
}

func runDayPlan(ctx context.Context, cfg *config.Config, plan *ExecutionPlan, repos []string) ([]DayResultRow, DaySummaryCounts, error) {
	// Scan-only mode
	if plan.Scan {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "Repository\tDate\tCommits")
		fmt.Fprintln(w, "----------\t----\t-------")
		for _, dayWindow := range plan.Days {
			for _, repoPath := range repos {
				meta, err := collector.ScanRepositoryDay(ctx, repoPath, dayWindow)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  [WARN] %s (%s): %v\n", repoPath, dayWindow.Date, err)
					continue
				}
				if meta != nil && meta.CommitsCount > 0 {
					fmt.Fprintf(w, "%s\t%s\t%d\n", meta.Name, dayWindow.Date, meta.CommitsCount)
				}
			}
		}
		w.Flush()
		return nil, DaySummaryCounts{}, nil
	}

	var llmClient *llm.Client
	if !plan.DryRun {
		baseURL := cfg.LLM.BaseURL
		if baseURL == "" {
			baseURL = defaultLLMBaseURL
		}
		llmClient = llm.New(cfg.LLM.Waterfall, baseURL, cfg.LLM.APIKeyEnv, 120*time.Second)
	}

	var rows []DayResultRow
	var counts DaySummaryCounts
	llmCallsCount := 0
	indexBroken := false

	// Day-major processing: for each day, for each repo
	for _, dayWindow := range plan.Days {
		day := dayWindow.Date
		for _, repoPath := range repos {
			projName := collector.ProjectName(repoPath)

			// Step 1: Check note existence
			exists, notePartial, err := vault.DevlogExists(cfg.Vault.Path, projName, day)
			if err != nil {
				rows = append(rows, DayResultRow{
					Repo:    projName,
					Date:    day,
					Status:  "ERROR",
					Commits: "-",
					Output:  fmt.Sprintf("checking note: %v", err),
				})
				counts.Errors++
				continue
			}

			isRegeneratingPartial := notePartial && !dayWindow.Partial
			if exists && !plan.Force && !isRegeneratingPartial {
				rows = append(rows, DayResultRow{
					Repo:    projName,
					Date:    day,
					Status:  "SKIPPED",
					Commits: "-",
					Output:  "note exists",
				})
				counts.Skipped++
				continue
			}

			// Step 2: Scan repository day
			meta, err := collector.ScanRepositoryDay(ctx, repoPath, dayWindow)
			if err != nil {
				rows = append(rows, DayResultRow{
					Repo:    projName,
					Date:    day,
					Status:  "ERROR",
					Commits: "-",
					Output:  fmt.Sprintf("scan failed: %v", err),
				})
				counts.Errors++
				continue
			}

			// nil result (zero commits) -> not listed in the table, counted separately; never write empty note even with -force
			if meta == nil || meta.CommitsCount == 0 {
				counts.ZeroCommits++
				continue
			}

			commitsStr := fmt.Sprintf("%d", meta.CommitsCount)
			expectedRelPath := fmt.Sprintf("Projects/%s/Devlog/%s.md", projName, day)

			// Step 3: Check Call Cap
			if plan.MaxCalls > 0 && llmCallsCount >= plan.MaxCalls {
				rows = append(rows, DayResultRow{
					Repo:    projName,
					Date:    day,
					Status:  "PENDING",
					Commits: commitsStr,
					Output:  "call cap reached",
				})
				counts.Pending++
				counts.CapPending++
				continue
			}

			if plan.DryRun {
				var status string
				if exists && plan.Force {
					status = "WOULD OVERWRITE"
				} else if isRegeneratingPartial {
					status = "WOULD REGENERATE"
				} else {
					status = "WOULD WRITE"
				}
				rows = append(rows, DayResultRow{
					Repo:    projName,
					Date:    day,
					Status:  status,
					Commits: commitsStr,
					Output:  expectedRelPath,
				})
				counts.Written++
				llmCallsCount++ // cap applies to dry-run plan
				continue
			}

			if indexBroken {
				rows = append(rows, DayResultRow{
					Repo:    projName,
					Date:    day,
					Status:  "PENDING",
					Commits: commitsStr,
					Output:  "not attempted: index update failed earlier in this run",
				})
				counts.Pending++
				counts.IndexPending++
				continue
			}

			// Real run: call LLM
			llmCallsCount++ // every attempt counts, including failures
			sysPrompt, userPrompt := BuildDayPrompt(meta, day, dayWindow.Partial)
			content, model, llmErr := llmClient.Call(ctx, sysPrompt, userPrompt)
			if llmErr != nil {
				fmt.Fprintf(os.Stderr, "[ERROR] %s (%s): LLM generation failed: %v\n", projName, day, llmErr)
				rows = append(rows, DayResultRow{
					Repo:    projName,
					Date:    day,
					Status:  "ERROR",
					Commits: commitsStr,
					Output:  fmt.Sprintf("LLM generation failed: %v", llmErr),
				})
				counts.Errors++
				continue
			}

			slug, lang := getProjectTags(cfg, meta.Name)
			nowStr := plan.Now.Format("2006-01-02 15:04:05")
			devlogData := vault.DevlogData{
				Date:         day,
				ProjectName:  meta.Name,
				Slug:         slug,
				Lang:         lang,
				Branch:       meta.Branch,
				CommitsCount: meta.CommitsCount,
				Shortstat:    meta.Shortstat,
				TopPackages:  meta.TopPackages,
				Now:          nowStr,
				Model:        model,
				Content:      content,
				Partial:      dayWindow.Partial,
			}

			if idxErr := vault.BootstrapIndex(cfg.Vault); idxErr != nil {
				fmt.Fprintf(os.Stderr, "[ERROR] %s (%s): index update failed: %v\n", projName, day, idxErr)
				rows = append(rows, DayResultRow{
					Repo:    projName,
					Date:    day,
					Status:  "ERROR",
					Commits: commitsStr,
					Output:  fmt.Sprintf("index update failed: %v; note not written", idxErr),
				})
				counts.Errors++
				indexBroken = true
				continue
			}

			if idxErr := vault.UpdateIndex(cfg.Vault, &devlogData); idxErr != nil {
				fmt.Fprintf(os.Stderr, "[ERROR] %s (%s): index update failed: %v\n", projName, day, idxErr)
				rows = append(rows, DayResultRow{
					Repo:    projName,
					Date:    day,
					Status:  "ERROR",
					Commits: commitsStr,
					Output:  fmt.Sprintf("index update failed: %v; note not written", idxErr),
				})
				counts.Errors++
				indexBroken = true
				continue
			}

			notePath, writeErr := vault.WriteDevlog(cfg.Vault.Path, meta.Name, day, devlogData)
			if writeErr != nil {
				fmt.Fprintf(os.Stderr, "[ERROR] %s (%s): write failed: %v\n", projName, day, writeErr)
				rows = append(rows, DayResultRow{
					Repo:    projName,
					Date:    day,
					Status:  "ERROR",
					Commits: commitsStr,
					Output:  fmt.Sprintf("write failed: %v", writeErr),
				})
				counts.Errors++
				continue
			}

			if stubErr := vault.EnsureOverviewStub(cfg.Vault.Path, meta.Name, devlogData.Slug); stubErr != nil {
				fmt.Fprintf(os.Stderr, "[WARN] %s: overview stub creation failed: %v\n", meta.Name, stubErr)
			}

			relPath, _ := filepath.Rel(cfg.Vault.Path, notePath)
			if relPath == "" {
				relPath = expectedRelPath
			}

			rows = append(rows, DayResultRow{
				Repo:    projName,
				Date:    day,
				Status:  "WRITTEN",
				Commits: commitsStr,
				Output:  relPath,
			})
			counts.Written++
		}
	}

	printDaySummaryTable(rows)

	fmt.Printf("\n%s\n", FormatDaySummaryLine(counts, plan.DryRun))
	fmt.Printf("%d repo-days had no commits\n", counts.ZeroCommits)

	if counts.Errors > 0 {
		return rows, counts, fmt.Errorf("%d error(s) occurred during run", counts.Errors)
	}

	return rows, counts, nil
}

func printDaySummaryTable(rows []DayResultRow) {
	if len(rows) == 0 {
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "Repository\tDate\tStatus\tCommits\tOutput / Reason")
	fmt.Fprintln(w, "----------\t----\t------\t-------\t---------------")
	for _, r := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r.Repo, r.Date, r.Status, r.Commits, r.Output)
	}
	w.Flush()
}

// printSummaryTable renders a structured terminal summary of all repository
// processing results for legacy window mode.
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
