package collector

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	// pemPrivateKeyPattern matches PEM-encoded private key blocks.
	pemPrivateKeyPattern = regexp.MustCompile(`(?s)-----BEGIN[^\r\n]*PRIVATE KEY-----.*?-----END[^\r\n]*(?:-----)?`)

	// awsAccessKeyPattern matches standard 20-character AWS Access Key IDs.
	awsAccessKeyPattern = regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)

	// authBearerPattern matches HTTP Authorization: Bearer headers with optional surrounding quotes.
	authBearerPattern = regexp.MustCompile(`(?i)(Authorization["']?\s*:\s*(["']?)Bearer\s+(["']?))([^"'\r\n\s]+)(["']?)`)

	// assignmentSecretPattern matches assignments or key-value mappings where the key
	// contains api_key, apikey, api-key, secret, token, or password.
	assignmentSecretPattern = regexp.MustCompile(
		`(?i)(^|[^\w.-])` +
			`(["']?([a-zA-Z0-9_.-]*(?:api[_-]?key|secret|token|password)[a-zA-Z0-9_.-]*)["']?\s*(?::=|=|:)\s*)` +
			`(?:("([^"\r\n]*)")|('([^'\r\n]*)')|([^\s\r\n,;#]+))`,
	)
)

// pathspecExcludes are the git pathspec excludes applied to all diff operations.
// Excludes lockfiles, dependencies, and minified bundles.
var pathspecExcludes = []string{
	":(exclude)*.lock",
	":(exclude)*.sum",
	":(exclude)*.min.*",
	":(exclude)vendor/",
	":(exclude)node_modules/",
}

// maxDiffChars is the character ceiling for unified diff content to protect
// LLM context windows.
const maxDiffChars = 25000

// ScanResult is a lightweight summary used by the -scan mode. It is always
// populated (even for inactive repos with zero commits in the window) so the
// scan table can report every discovered repository.
type ScanResult struct {
	Name        string
	Path        string
	Branch      string
	Commits     int
	Shortstat   string
	TopPackages []string
}

// DiscoverRepositories walks the given roots up to maxDepth, identifying valid git
// repositories while respecting directory excludes. Returns absolute repo paths.
//
// Repository discovery criteria:
//   - Uses git rev-parse --absolute-git-dir to validate real git repos.
//   - Skips repos with index.lock (in use).
//   - Skips unborn branches (initialized but no commits) via git rev-parse HEAD.
//   - Skips vault sub-repo mirrors (project-nexus-*).
func DiscoverRepositories(ctx context.Context, roots []string, maxDepth int, excludes []string) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	excludeSet := make(map[string]bool)
	for _, ex := range excludes {
		excludeSet[ex] = true
	}

	var repos []string

	for _, root := range roots {
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}

		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil // skip directories we can't access
			}

			// Enforce maxDepth: count path components from root.
			rel, _ := filepath.Rel(root, path)
			depth := 0
			if rel != "." && rel != ".." {
				depth = strings.Count(rel, string(filepath.Separator)) + 1
			}
			if depth > maxDepth {
				return filepath.SkipDir
			}

			// Skip excluded directory names.
			dirname := d.Name()
			if excludeSet[dirname] {
				return filepath.SkipDir
			}

			// Look for .git entry (directory or file for submodules).
			gitEntry := filepath.Join(path, ".git")
			_, err = os.Lstat(gitEntry)
			if err != nil {
				return nil // not a git repo, keep walking
			}

			// Validate: must be a real git repo.
			cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			out, err := exec.CommandContext(cmdCtx, "git", "-C", path, "rev-parse", "--absolute-git-dir").Output()
			cancel()
			if err != nil {
				return nil // not a valid git repo
			}
			gitDir := filepath.Clean(filepath.FromSlash(strings.TrimSpace(string(out))))
			if gitDir == "" {
				return nil
			}
			// Check for index.lock (repo in use).
			if _, err := os.Stat(filepath.Join(gitDir, "index.lock")); err == nil {
				return filepath.SkipDir
			}

			// Check for unborn branch (initialized but no commits).
			// rev-parse HEAD fails for unborn branches.
			headCtx, headCancel := context.WithTimeout(ctx, 30*time.Second)
			_, headErr := exec.CommandContext(headCtx, "git", "-C", path, "rev-parse", "HEAD").Output()
			headCancel()
			if headErr != nil {
				return filepath.SkipDir
			}

			// Skip vault sub-repo mirrors.
			basename := filepath.Base(path)
			if strings.HasPrefix(basename, "project-nexus-") {
				return filepath.SkipDir
			}

			// Check if already discovered.
			for _, r := range repos {
				if r == path {
					return filepath.SkipDir
				}
			}

			repos = append(repos, path)
			return filepath.SkipDir // don't recurse into the repo itself
		})
		if err != nil {
			return nil, fmt.Errorf("walking %s: %w", root, err)
		}
	}

	return repos, nil
}

// emptyTreeSHA is the universal Git empty tree hash (git hash-object -t tree /dev/null).
const emptyTreeSHA = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

// resolveBaseRef finds the commit SHA before the time window to diff against.
// If no commit exists before the window (e.g. fresh repository or repo younger
// than the window), it falls back to the universal empty tree hash so that all
// changes within the window are captured without relying on the local reflog.
func resolveBaseRef(ctx context.Context, repoPath, sinceWindow string) (string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	out, err := exec.CommandContext(cmdCtx, "git", "-C", repoPath, "rev-list", "-1",
		fmt.Sprintf("--before=%s", sinceWindow), "HEAD").Output()
	cancel()
	if err != nil {
		return "", fmt.Errorf("resolving base commit for %s: %w", repoPath, err)
	}
	sha := strings.TrimSpace(string(out))
	if sha == "" {
		return emptyTreeSHA, nil
	}
	return sha, nil
}

// ScanRepository is a convenience wrapper for -scan mode. It always returns a
// ScanResult (even for inactive repos with zero commits), so the scan table can
// display every discovered repository without errors.
func ScanRepository(ctx context.Context, repoPath, sinceWindow string) (*ScanResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	meta, err := HarvestRepoMetadata(ctx, repoPath, sinceWindow)
	if err != nil {
		return nil, err
	}
	if meta != nil {
		return &ScanResult{
			Name:        meta.Name,
			Path:        meta.Path,
			Branch:      meta.Branch,
			Commits:     meta.CommitsCount,
			Shortstat:   meta.Shortstat,
			TopPackages: meta.TopPackages,
		}, nil
	}
	// Inactive repo (0 commits in window) — still report branch.
	branch, _ := getActiveBranch(ctx, repoPath)
	return &ScanResult{
		Name:    repoToProjectName(repoPath),
		Path:    repoPath,
		Branch:  branch,
		Commits: 0,
	}, nil
}

// HarvestRepoMetadata collects git telemetry and diff content for a single
// repository within the specified time window. If the repository has zero
// commits in the window, returns (nil, nil) to signal the caller to skip it.
//
// The sinceWindow parameter is used as the git log --since date and to resolve
// the base commit reference without relying on the local reflog.
//
// Note: repoPath values are guaranteed to originate from DiscoverRepositories'
// verified filesystem walk, never from unsanitized user input.
func HarvestRepoMetadata(ctx context.Context, repoPath string, sinceWindow string) (*RepoMetadata, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	// Query commits in window.
	commits, err := collectCommitsInWindow(ctx, repoPath, sinceWindow)
	if err != nil {
		return nil, fmt.Errorf("collecting commits for %s: %w", repoPath, err)
	}
	if len(commits) == 0 {
		return nil, nil // inactive repo, skip
	}

	projectName := repoToProjectName(repoPath)
	branch, err := getActiveBranch(ctx, repoPath)
	if err != nil {
		return nil, fmt.Errorf("getting branch for %s: %w", repoPath, err)
	}

	baseRef, err := resolveBaseRef(ctx, repoPath, sinceWindow)
	if err != nil {
		return nil, fmt.Errorf("resolving base commit for %s: %w", repoPath, err)
	}

	shortstat, err := getDiffShortstat(ctx, repoPath, baseRef)
	if err != nil {
		return nil, fmt.Errorf("getting shortstat for %s: %w", repoPath, err)
	}

	topPackages, err := getTopModifiedPackages(ctx, repoPath, baseRef)
	if err != nil {
		return nil, fmt.Errorf("getting top packages for %s: %w", repoPath, err)
	}

	unifiedDiff, err := getUnifiedDiff(ctx, repoPath, baseRef)
	if err != nil {
		return nil, fmt.Errorf("getting unified diff for %s: %w", repoPath, err)
	}

	return &RepoMetadata{
		Name:         projectName,
		Path:         repoPath,
		Branch:       branch,
		Commits:      commits,
		CommitsCount: len(commits),
		Shortstat:    shortstat,
		TopPackages:  topPackages,
		UnifiedDiff:  unifiedDiff,
	}, nil
}

// collectCommitsInWindow queries git log with --since to get oneline commit
// messages within the time window.
func collectCommitsInWindow(ctx context.Context, repoPath, sinceWindow string) ([]string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	out, err := exec.CommandContext(cmdCtx, "git", "-C", repoPath, "log",
		fmt.Sprintf("--since=%s", sinceWindow),
		"--oneline").Output()
	cancel()
	if err != nil {
		return nil, err
	}
	return parseCommitLines(string(out)), nil
}

// parseCommitLines splits raw git log --oneline output into a slice of
// non-empty, trimmed commit message lines. Extracted as a pure function for
// unit testing without requiring a real git repository.
func parseCommitLines(output string) []string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var commits []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			commits = append(commits, line)
		}
	}
	return commits
}

// getActiveBranch returns the active branch name using git rev-parse
// --abbrev-ref HEAD.
func getActiveBranch(ctx context.Context, repoPath string) (string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	out, err := exec.CommandContext(cmdCtx, "git", "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD").Output()
	cancel()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// getDiffShortstat returns the git diff --shortstat summary line for the
// baseRef..HEAD range.
func getDiffShortstat(ctx context.Context, repoPath, baseRef string) (string, error) {
	ref := fmt.Sprintf("%s..HEAD", baseRef)
	if strings.Contains(baseRef, "..") {
		ref = baseRef
	}
	args := append([]string{"-C", repoPath, "diff", "--shortstat",
		ref, "--", "."}, pathspecExcludes...)
	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	out, err := exec.CommandContext(cmdCtx, "git", args...).Output()
	cancel()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// getTopModifiedPackages analyzes git diff --name-only and returns the top
// 4 most-modified directory/package prefixes (first two path components for
// Go packages).
func getTopModifiedPackages(ctx context.Context, repoPath, baseRef string) ([]string, error) {
	ref := fmt.Sprintf("%s..HEAD", baseRef)
	if strings.Contains(baseRef, "..") {
		ref = baseRef
	}
	args := append([]string{"-C", repoPath, "diff", "--name-only",
		ref, "--", "."}, pathspecExcludes...)
	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	out, err := exec.CommandContext(cmdCtx, "git", args...).Output()
	cancel()
	if err != nil {
		return nil, err
	}
	return rankPackages(string(out)), nil
}

// rankPackages parses git diff --name-only output and returns the top 4
// most-modified directory/package prefixes (first two path components for
// Go packages). Extracted as a pure function for unit testing without
// requiring a real git repository.
func rankPackages(nameOnlyOutput string) []string {
	dirCounts := make(map[string]int)
	for _, line := range strings.Split(strings.TrimSpace(nameOnlyOutput), "\n") {
		fp := strings.TrimSpace(line)
		if fp == "" {
			continue
		}
		fp = filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.ReplaceAll(fp, "\\", "/"))))
		parts := strings.Split(fp, "/")
		var pkg string
		if len(parts) >= 2 {
			pkg = strings.Join(parts[:2], "/")
		} else if len(parts) == 1 {
			pkg = parts[0]
		} else {
			continue
		}
		dirCounts[pkg]++
	}

	// Sort by count descending, take top 4.
	type dirCount struct {
		dir   string
		count int
	}
	var sorted []dirCount
	for d, c := range dirCounts {
		sorted = append(sorted, dirCount{d, c})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})

	var top []string
	for i, dc := range sorted {
		if i >= 4 {
			break
		}
		top = append(top, dc.dir)
	}
	return top
}

// getUnifiedDiff returns the truncated unified diff for baseRef..HEAD with
// secrets redacted. If the diff exceeds maxDiffChars, head + tail truncation is applied.
func getUnifiedDiff(ctx context.Context, repoPath, baseRef string) (string, error) {
	ref := fmt.Sprintf("%s..HEAD", baseRef)
	if strings.Contains(baseRef, "..") {
		ref = baseRef
	}
	args := append([]string{"-C", repoPath, "diff",
		ref, "--", "."}, pathspecExcludes...)
	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	out, err := exec.CommandContext(cmdCtx, "git", args...).Output()
	cancel()
	if err != nil {
		return "", err
	}
	return redactSecrets(truncateDiff(string(out))), nil
}

// redactSecrets applies best-effort sanitization to git diff output by replacing
// detected credentials and sensitive tokens with [REDACTED].
//
// Patterns covered include:
//   - AWS access keys (AKIA[0-9A-Z]{16})
//   - Private key blocks (PEM-encoded BEGIN ... PRIVATE KEY ... END)
//   - HTTP Authorization Bearer tokens
//   - Key-value / assignment pairs for api_key, secret, token, password
//     in code, YAML, JSON, and .env files.
//
// Note: This redaction is best-effort and heuristic-based; it is not a guarantee
// that all sensitive material will be identified. Users should ensure their
// repositories do not contain plaintext secrets.
func redactSecrets(diff string) string {
	if diff == "" {
		return ""
	}

	// 1. Redact PEM private key blocks.
	diff = pemPrivateKeyPattern.ReplaceAllString(diff, "[REDACTED]")

	// 2. Redact AWS access keys.
	diff = awsAccessKeyPattern.ReplaceAllString(diff, "[REDACTED]")

	// 3. Redact Authorization: Bearer tokens.
	diff = authBearerPattern.ReplaceAllStringFunc(diff, func(m string) string {
		sub := authBearerPattern.FindStringSubmatch(m)
		if len(sub) == 0 {
			return m
		}
		prefix := sub[1]
		val := sub[4]
		closeQuote := sub[5]
		if val == "[REDACTED]" {
			return m
		}
		return prefix + "[REDACTED]" + closeQuote
	})

	// 4. Redact assignment / mapping secrets.
	diff = assignmentSecretPattern.ReplaceAllStringFunc(diff, func(m string) string {
		sub := assignmentSecretPattern.FindStringSubmatch(m)
		if len(sub) == 0 {
			return m
		}
		lead := sub[1]
		keyOp := sub[2]

		// Guard against comparison operators like !=, <=, >=
		if strings.HasSuffix(lead, "!") || strings.HasSuffix(lead, "<") || strings.HasSuffix(lead, ">") {
			return m
		}

		if sub[4] != "" { // double-quoted value
			val := sub[5]
			if val == "" || val == "[REDACTED]" {
				return m
			}
			return lead + keyOp + `"[REDACTED]"`
		}
		if sub[6] != "" { // single-quoted value
			val := sub[7]
			if val == "" || val == "[REDACTED]" {
				return m
			}
			return lead + keyOp + `'[REDACTED]'`
		}

		// Unquoted value
		val := sub[8]
		if val == "" || val == "[REDACTED]" {
			return m
		}
		// Guard against equality comparison == (val starts with =)
		if strings.HasPrefix(val, "=") {
			return m
		}
		// Guard against function calls / invocations
		if strings.Contains(val, "(") {
			return m
		}
		// Guard against common non-secret language keywords
		lower := strings.ToLower(val)
		if lower == "true" || lower == "false" || lower == "null" || lower == "nil" || lower == "none" || lower == "undefined" {
			return m
		}
		return lead + keyOp + "[REDACTED]"
	})

	return diff
}

// safeTruncate truncates s to at most maxBytes, backing off to the nearest
// preceding valid UTF-8 rune boundary so that multi-byte code points are never split.
func safeTruncate(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}
	return s[:maxBytes]
}

// truncateDiff truncates a diff string to head + tail within maxDiffChars,
// preserving valid UTF-8 rune boundaries on both the head and tail slices.
func truncateDiff(diffText string) string {
	if len(diffText) <= maxDiffChars {
		return diffText
	}
	half := maxDiffChars / 2
	head := safeTruncate(diffText, half)

	tailStart := len(diffText) - half
	for tailStart < len(diffText) && !utf8.RuneStart(diffText[tailStart]) {
		tailStart++
	}
	tail := diffText[tailStart:]

	return fmt.Sprintf("%s\n\n... [TRUNCATED — %d chars total] ...\n\n%s", head, len(diffText), tail)
}

// repoToProjectName maps a repo path to its vault project directory name.
func repoToProjectName(repoPath string) string {
	base := filepath.Base(repoPath)
	if repoPath == "/srv" || base == "srv" {
		return "Homelab"
	}
	return base
}

// MaxDiffChars returns the maximum diff character ceiling.
func MaxDiffChars() int {
	return maxDiffChars
}
