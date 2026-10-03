package vault

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

//go:embed templates/devlog.md.tmpl
var templatesFS embed.FS

// DevlogData is the data passed to the devlog.md.tmpl template.
type DevlogData struct {
	Date         string   // YYYY-MM-DD
	ProjectName  string   // e.g. "AcmeWidgets.com"
	Slug         string   // e.g. "acmewidgets"
	Lang         string   // e.g. "go"
	Branch       string   // active git branch
	CommitsCount int      // number of commits in window
	Shortstat    string   // e.g. "8 files changed, 780 insertions(+), 20 deletions(-)"
	TopPackages  []string // e.g. ["internal/ingest", "internal/db"]
	Model        string   // LLM model used
	Now          string   // generation timestamp
	Content      string   // LLM-generated callout body
	Partial      bool     // true if the note represents a partial day
}

// RenderDevlog executes the embedded devlog template and returns the formatted
// markdown string. Implements the v3 callout taxonomy (telemetry note,
// abstract, info, bug, warning, check sections).
func RenderDevlog(data DevlogData) (string, error) {
	tmplText, err := templatesFS.ReadFile("templates/devlog.md.tmpl")
	if err != nil {
		return "", fmt.Errorf("reading embedded template: %w", err)
	}
	tmpl, err := template.New("devlog").Parse(string(tmplText))
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}
	return buf.String(), nil
}

// devlogPath returns the filesystem path to a devlog note for the given vault, project, and date.
func devlogPath(vaultPath, projectName, date string) string {
	return filepath.Join(vaultPath, "Projects", projectName, "Devlog", date+".md")
}

// DevlogExists checks whether a devlog note exists for the given project and date in the vault,
// and whether it is marked as partial in its frontmatter.
//
// A missing note returns (false, false, nil). If the note exists, partial is true only if the note's
// frontmatter (the initial --- block, inspected within at most the first 50 lines) contains a line
// that is exactly "partial: true" (with trailing \r trimmed). Occurrences in the body are ignored.
func DevlogExists(vaultPath, project, date string) (exists bool, partial bool, err error) {
	path := devlogPath(vaultPath, project, date)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, false, nil
		}
		return false, false, fmt.Errorf("reading devlog at %s: %w", path, err)
	}

	exists = true
	lines := strings.Split(string(data), "\n")
	if len(lines) > 50 {
		lines = lines[:50]
	}

	inFrontmatter := false
	for i, line := range lines {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(line)
		if i == 0 {
			if trimmed == "---" {
				inFrontmatter = true
				continue
			}
			// File does not start with frontmatter delimiter ---
			break
		}
		if inFrontmatter {
			if trimmed == "---" {
				// Reached end of first frontmatter block
				break
			}
			if line == "partial: true" {
				partial = true
			}
		}
	}

	return exists, partial, nil
}

// WriteDevlog renders the template and writes the devlog to
// <vault>/Projects/<project>/Devlog/<date>.md, creating directories as needed.
func WriteDevlog(vaultPath, projectName, date string, data DevlogData) (string, error) {
	targetPath := devlogPath(vaultPath, projectName, date)
	devlogDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(devlogDir, 0o755); err != nil {
		return "", fmt.Errorf("creating devlog directory: %w", err)
	}

	rendered, err := RenderDevlog(data)
	if err != nil {
		return "", err
	}
	if err := writeFileAtomic(targetPath, []byte(rendered), 0o644); err != nil {
		return "", fmt.Errorf("writing devlog file: %w", err)
	}
	return targetPath, nil
}
