package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ZeezyCodes/vaultchron/internal/config"
)

// firstBoldHeader extracts the first bold inline header (**text**) from the
// LLM-generated content. This serves as the brief abstract for index entries.
var firstBoldHeader = regexp.MustCompile(`\*\*([^*]+)\*\*`)

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

// UpdateIndex prepends or updates today's devlog entry in the vault's
// 00-Dev-Index.md under the "Recent Dev Logs" or "Recent Activity" section.
// Existing manual notes and archive callouts are preserved untouched.
//
// Each entry links to the individual project devlog via an Obsidian wikilink:
//
//	[[Projects/<ProjectName>/Devlog/<YYYY-MM-DD>|YYYY-MM-DD]] — <Brief Abstract>
//
// If the section uses a markdown table, the row is formatted as:
//
//	| [[Projects/<ProjectName>/Devlog/<YYYY-MM-DD>|YYYY-MM-DD]] | <Escaped Summary> |
//
// and placed immediately after the table separator line, preserving valid table
// syntax. If an entry for the same date and project already exists, it is
// updated in place rather than duplicated.
func UpdateIndex(vaultCfg config.VaultConfig, data *DevlogData) error {
	indexPath := filepath.Join(vaultCfg.Path, vaultCfg.IndexFile)

	raw, err := os.ReadFile(indexPath)
	if err != nil {
		return fmt.Errorf("reading index file %s: %w", indexPath, err)
	}

	lines := strings.Split(string(raw), "\n")

	summary := extractSummary(data.Content, data.CommitsCount, data.Shortstat)
	link := fmt.Sprintf("[[Projects/%s/Devlog/%s|%s]]", data.ProjectName, data.Date, data.Date)

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
	// Section ends at the next section heading (## or ### of equal/higher rank),
	// an archive callout (> [!example]), or a horizontal rule (---).
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

	// Step 3: Check for existing entry for this project and date.
	for i := recentHeaderIdx + 1; i < sectionEndIdx; i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		if strings.Contains(line, data.Date) && strings.Contains(line, data.ProjectName) {
			if strings.HasPrefix(trimmed, "|") {
				lines[i] = fmt.Sprintf("| %s | %s |", link, escapeTableCell(summary))
			} else if strings.HasPrefix(trimmed, "- ") {
				lines[i] = fmt.Sprintf("- %s — %s", link, summary)
			} else {
				lines[i] = fmt.Sprintf("%s — %s", link, summary)
			}
			return writeFileAtomic(indexPath, []byte(strings.Join(lines, "\n")), 0o644)
		}
	}

	// Step 4: Detect whether recent entries are structured as a Markdown Table.
	// Look for a table separator line between recentHeaderIdx and sectionEndIdx,
	// ignoring blockquotes/callouts.
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

	if tableSeparatorIdx != -1 {
		// Table mode:
		// Insert immediately after the table separator row to maintain valid syntax.
		tableRow := fmt.Sprintf("| %s | %s |", link, escapeTableCell(summary))
		insertAt := tableSeparatorIdx + 1
		lines = append(lines[:insertAt], append([]string{tableRow}, lines[insertAt:]...)...)
		return writeFileAtomic(indexPath, []byte(strings.Join(lines, "\n")), 0o644)
	}

	// Step 5: List mode (no unquoted table found).
	// Check if existing list items use bullet style (- [[...]]).
	usesBullets := false
	firstItemIdx := -1

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

	var listEntry string
	if usesBullets {
		listEntry = fmt.Sprintf("- %s — %s", link, summary)
	} else {
		listEntry = fmt.Sprintf("%s — %s", link, summary)
	}

	if firstItemIdx != -1 {
		// Prepend before the first existing list item.
		lines = append(lines[:firstItemIdx], append([]string{listEntry}, lines[firstItemIdx:]...)...)
	} else {
		// No existing list items: insert after header (skipping immediate blanks).
		insertAt := recentHeaderIdx + 1
		for insertAt < sectionEndIdx && strings.TrimSpace(lines[insertAt]) == "" {
			insertAt++
		}
		lines = append(lines[:insertAt], append([]string{listEntry}, lines[insertAt:]...)...)
	}

	return writeFileAtomic(indexPath, []byte(strings.Join(lines, "\n")), 0o644)
}
