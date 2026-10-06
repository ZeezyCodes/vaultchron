package vault

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ZeezyCodes/vaultchron/internal/config"
)

const defaultIndexContent = "# Dev Index\n\n## Recent Dev Logs\n\n| Date | Notes |\n|---|---|\n"

// BootstrapIndex creates the root dev index file with minimal content if it does not already exist.
// If the index file already exists, it is never modified or overwritten.
func BootstrapIndex(vaultCfg config.VaultConfig) error {
	indexFile := vaultCfg.IndexFile
	if indexFile == "" {
		indexFile = "00-Dev-Index.md"
	}
	indexPath := filepath.Join(vaultCfg.Path, filepath.FromSlash(indexFile))
	_, err := os.Stat(indexPath)
	if err == nil {
		return nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("checking index file %s: %w", indexPath, err)
	}
	if err := writeFileAtomic(indexPath, []byte(defaultIndexContent), 0o644); err != nil {
		return fmt.Errorf("writing index file %s: %w", indexPath, err)
	}
	return nil
}

// firstBoldHeader extracts the first bold inline header (**text**) from the
// LLM-generated content. This serves as the brief abstract for index entries.
var firstBoldHeader = regexp.MustCompile(`\*\*([^*]+)\*\*`)

// entryLinkPattern matches devlog Obsidian wikilinks to extract project folder and date.
// Group 1: project folder name
// Group 2: date (YYYY-MM-DD)
var entryLinkPattern = regexp.MustCompile(`\[\[[^\]|]*?([^/\]|]+)/Devlog/(\d{4}-\d{2}-\d{2})(?:\|[^\]]*)?\]\]`)

// extractSummary pulls a one-line summary from the devlog content. It prefers
// the first bold inline header (the abstract title the LLM produces), falling
// back to a commit/churn summary derived from telemetry fields.
func extractSummary(content string, commits int, shortstat string) string {
	if m := firstBoldHeader.FindStringSubmatch(content); len(m) >= 2 && strings.TrimSpace(m[1]) != "" {
		return strings.TrimSpace(m[1])
	}
	return fmt.Sprintf("%d commits, %s", commits, shortstat)
}

// escapeTableCell replaces pipe characters in cell text to prevent breaking
// the markdown table layout.
func escapeTableCell(text string) string {
	return strings.ReplaceAll(text, "|", "\\|")
}

// isTableSeparator returns true if a line is a markdown table separator (e.g. |---|---|).
func isTableSeparator(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "|") || !strings.HasSuffix(trimmed, "|") {
		return false
	}
	clean := strings.ReplaceAll(trimmed, "|", "")
	clean = strings.ReplaceAll(clean, "-", "")
	clean = strings.ReplaceAll(clean, ":", "")
	clean = strings.TrimSpace(clean)
	return clean == "" && strings.Contains(trimmed, "-")
}

// parseTableCells splits a markdown table row into its trimmed cell values.
func parseTableCells(row string) []string {
	trimmed := strings.TrimSpace(row)
	trimmed = strings.TrimPrefix(trimmed, "|")
	trimmed = strings.TrimSuffix(trimmed, "|")
	raw := strings.Split(trimmed, "|")
	cells := make([]string, len(raw))
	for i, c := range raw {
		cells[i] = strings.TrimSpace(c)
	}
	return cells
}

// formatTableRow formats a markdown table row based on the number of header cells N.
// N == 2 (or < 2): returns "| link | summary |".
// N > 2: first cell = link, every header cell whose trimmed text equals "Project" (case-insensitive) gets the project display name,
// last cell = summary, any other cell empty.
func formatTableRow(data *DevlogData, link, summary string, headerCells []string) string {
	n := len(headerCells)
	if n <= 2 {
		return fmt.Sprintf("| %s | %s |", link, escapeTableCell(summary))
	}
	cells := make([]string, n)
	cells[0] = link
	cells[n-1] = escapeTableCell(summary)
	for i := 1; i < n-1; i++ {
		if strings.EqualFold(strings.TrimSpace(headerCells[i]), "Project") {
			cells[i] = data.ProjectName
		} else {
			cells[i] = ""
		}
	}
	return "| " + strings.Join(cells, " | ") + " |"
}

// UpdateIndex inserts or updates a devlog entry in the vault's 00-Dev-Index.md
// under the "Recent Dev Logs" or "Recent Activity" section.
//
// Entries are sorted by date descending (newest first), with equal dates sorted
// by project name ascending (case-insensitive). Existing entries for the same date
// and project are updated in place. Unparseable rows (without a matching devlog link)
// are never reordered, removed, or modified.
func UpdateIndex(vaultCfg config.VaultConfig, data *DevlogData) error {
	indexFile := vaultCfg.IndexFile
	if indexFile == "" {
		indexFile = "00-Dev-Index.md"
	}
	indexPath := filepath.Join(vaultCfg.Path, filepath.FromSlash(indexFile))

	raw, err := os.ReadFile(indexPath)
	if err != nil {
		return fmt.Errorf("reading index file %s: %w", indexPath, err)
	}

	isCRLF := strings.Contains(string(raw), "\r\n")
	lineEnding := "\n"
	if isCRLF {
		lineEnding = "\r\n"
	}

	content := strings.ReplaceAll(string(raw), "\r\n", "\n")
	lines := strings.Split(content, "\n")

	summary := extractSummary(data.Content, data.CommitsCount, data.Shortstat)
	projectsDir := vaultCfg.ProjectsDir
	if projectsDir == "" {
		projectsDir = "Projects"
	}
	link := fmt.Sprintf("[[%s/%s/Devlog/%s|%s]]", projectsDir, data.ProjectName, data.Date, data.Date)

	// Step 1: Locate the recent logs section header.
	recentHeaderIdx := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "recent dev logs") ||
			strings.Contains(lower, "recent devlogs") ||
			strings.Contains(lower, "recent activity") {
			recentHeaderIdx = i
			break
		}
	}

	// Fallback to "## 📅 Dev Logs" or "## Dev Logs" if specific recent header not found.
	if recentHeaderIdx == -1 {
		for i, line := range lines {
			lower := strings.ToLower(strings.TrimSpace(line))
			if strings.HasPrefix(lower, "##") && (strings.Contains(lower, "dev logs") || strings.Contains(lower, "devlogs")) {
				recentHeaderIdx = i
				break
			}
		}
	}

	if recentHeaderIdx == -1 {
		return fmt.Errorf("recent Dev Logs section not found in index file")
	}

	// Step 2: Determine section boundary.
	sectionEndIdx := len(lines)
	isHeaderH3 := strings.HasPrefix(strings.TrimSpace(lines[recentHeaderIdx]), "###")

	for i := recentHeaderIdx + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "## ") || (isHeaderH3 && strings.HasPrefix(trimmed, "### ")) {
			sectionEndIdx = i
			break
		}
		if strings.HasPrefix(trimmed, "> [!example]") || strings.HasPrefix(trimmed, "> [!archive]") {
			sectionEndIdx = i
			break
		}
		if trimmed == "---" {
			sectionEndIdx = i
			break
		}
	}

	// Step 3: Detect Table vs List mode and prepare new entry.
	tableSeparatorIdx := -1
	for i := recentHeaderIdx + 1; i < sectionEndIdx; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, ">") {
			continue
		}
		if isTableSeparator(trimmed) {
			tableSeparatorIdx = i
			break
		}
	}

	var newEntry string
	var startScanIdx int
	usesBullets := false
	firstItemIdx := -1

	if tableSeparatorIdx != -1 {
		// Table mode
		startScanIdx = tableSeparatorIdx + 1
		headerRow := ""
		if tableSeparatorIdx > 0 {
			headerRow = lines[tableSeparatorIdx-1]
		}
		headerCells := parseTableCells(headerRow)
		newEntry = formatTableRow(data, link, summary, headerCells)
	} else {
		// List mode
		startScanIdx = recentHeaderIdx + 1
		for i := recentHeaderIdx + 1; i < sectionEndIdx; i++ {
			trimmed := strings.TrimSpace(lines[i])
			if trimmed == "" || strings.HasPrefix(trimmed, ">") {
				continue
			}
			if strings.HasPrefix(trimmed, "- [[") || strings.HasPrefix(trimmed, "* [[") {
				usesBullets = true
				if firstItemIdx == -1 {
					firstItemIdx = i
				}
			} else if strings.HasPrefix(trimmed, "[[") {
				if firstItemIdx == -1 {
					firstItemIdx = i
				}
			}
		}

		if usesBullets {
			newEntry = fmt.Sprintf("- %s — %s", link, summary)
		} else {
			newEntry = fmt.Sprintf("%s — %s", link, summary)
		}
	}

	// Step 4: Check if same date + same project already exists.
	// Replace in place (unchanged behavior). Re-running with identical data leaves the file byte-identical.
	for i := startScanIdx; i < sectionEndIdx; i++ {
		m := entryLinkPattern.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		proj := m[1]
		date := m[2]
		if date == data.Date && strings.EqualFold(proj, data.ProjectName) {
			lines[i] = newEntry
			return writeFileAtomic(indexPath, []byte(strings.Join(lines, lineEnding)), 0o644)
		}
	}

	// Step 5: Sorted insertion.
	// Order: dates DESCENDING (newest first); equal dates ordered by project ascending (case-insensitive).
	// Insert before the first parseable entry whose date is older than the new date,
	// or has the same date and a project that sorts after the new project.
	// If none, insert after the last parseable entry.
	// If there are no parseable entries keep today's behavior.
	insertAt := -1
	lastParseableIdx := -1
	newProjLower := strings.ToLower(data.ProjectName)

	for i := startScanIdx; i < sectionEndIdx; i++ {
		m := entryLinkPattern.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		lastParseableIdx = i
		entryProj := m[1]
		entryDate := m[2]
		entryProjLower := strings.ToLower(entryProj)

		if entryDate < data.Date || (entryDate == data.Date && entryProjLower > newProjLower) {
			insertAt = i
			break
		}
	}

	if insertAt == -1 {
		if lastParseableIdx != -1 {
			insertAt = lastParseableIdx + 1
		} else {
			// No parseable entries: keep today's behavior.
			if tableSeparatorIdx != -1 {
				insertAt = tableSeparatorIdx + 1
			} else if firstItemIdx != -1 {
				insertAt = firstItemIdx
			} else {
				insertAt = recentHeaderIdx + 1
				for insertAt < sectionEndIdx && strings.TrimSpace(lines[insertAt]) == "" {
					insertAt++
				}
			}
		}
	}

	lines = append(lines[:insertAt], append([]string{newEntry}, lines[insertAt:]...)...)
	return writeFileAtomic(indexPath, []byte(strings.Join(lines, lineEnding)), 0o644)
}
