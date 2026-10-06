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

	"github.com/ZeezyCodes/vaultchron/internal/config"
)

//go:embed templates/devlog.md.tmpl templates/overview.md.tmpl
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
	IndexFile    string   // configured index_file (or IndexLink)
	ProjectsDir  string   // configured projects_dir
	IndexLink    string   // computed or explicit index link target
}

// RenderDevlog executes the embedded devlog template and returns the formatted
// markdown string. Implements the v3 callout taxonomy (telemetry note,
// abstract, info, bug, warning, check sections). A CRLF checkout of the
// template is normalized to LF.
func RenderDevlog(data DevlogData) (string, error) {
	if data.IndexLink == "" {
		if data.IndexFile != "" {
			data.IndexLink = strings.TrimSuffix(data.IndexFile, ".md")
		} else {
			data.IndexLink = "00-Dev-Index"
		}
	}
	if data.ProjectsDir == "" {
		data.ProjectsDir = "Projects"
	}
	tmplText, err := templatesFS.ReadFile("templates/devlog.md.tmpl")
	if err != nil {
		return "", fmt.Errorf("reading embedded template: %w", err)
	}
	return renderTemplate(tmplText, data)
}

func renderTemplate(tmplText []byte, data DevlogData) (string, error) {
	normalized := strings.ReplaceAll(string(tmplText), "\r\n", "\n")
	tmpl, err := template.New("devlog").Parse(normalized)
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}
	return buf.String(), nil
}

// devlogPath returns the filesystem path to a devlog note for the given vault, projectsDir, project, and date.
func devlogPath(vaultPath, projectsDir, projectName, date string) string {
	if projectsDir == "" {
		projectsDir = "Projects"
	}
	return filepath.Join(vaultPath, filepath.FromSlash(projectsDir), projectName, "Devlog", date+".md")
}

// DevlogExists checks whether a devlog note exists for the given project and date in the vault,
// and whether it is marked as partial in its frontmatter.
//
// A missing note returns (false, false, nil). If the note exists, partial is true only if the note's
// frontmatter (the initial --- block, inspected within at most the first 50 lines) contains a line
// that is exactly "partial: true" (with trailing \r trimmed). Occurrences in the body are ignored.
func DevlogExists(vaultPath, project, date string, projectsDir ...string) (exists bool, partial bool, err error) {
	pDir := "Projects"
	if len(projectsDir) > 0 && projectsDir[0] != "" {
		pDir = projectsDir[0]
	}
	path := devlogPath(vaultPath, pDir, project, date)
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
// <vault>/<projects_dir>/<project>/Devlog/<date>.md, creating directories as needed.
func WriteDevlog(vaultPath, projectName, date string, data DevlogData) (string, error) {
	targetPath := devlogPath(vaultPath, data.ProjectsDir, projectName, date)
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

// OverviewData is the data passed to the overview.md.tmpl template.
type OverviewData struct {
	ProjectName string // e.g. "AcmeWidgets.com"
	Slug        string // e.g. "acmewidgets"
	IndexFile   string // e.g. "00-Dev-Index.md"
	IndexLink   string // computed or explicit index link target
}

// RenderOverview executes the embedded overview template and returns the formatted
// markdown string. A CRLF checkout of the template is normalized to LF.
func RenderOverview(data OverviewData) (string, error) {
	if data.IndexLink == "" {
		if data.IndexFile != "" {
			data.IndexLink = strings.TrimSuffix(data.IndexFile, ".md")
		} else {
			data.IndexLink = "00-Dev-Index"
		}
	}
	tmplText, err := templatesFS.ReadFile("templates/overview.md.tmpl")
	if err != nil {
		return "", fmt.Errorf("reading embedded template: %w", err)
	}
	normalized := strings.ReplaceAll(string(tmplText), "\r\n", "\n")
	tmpl, err := template.New("overview").Parse(normalized)
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}
	return buf.String(), nil
}

// overviewPath returns the filesystem path to an overview note for the given vault, projectsDir, and project.
func overviewPath(vaultPath, projectsDir, projectName string) string {
	if projectsDir == "" {
		projectsDir = "Projects"
	}
	return filepath.Join(vaultPath, filepath.FromSlash(projectsDir), projectName, "Overview.md")
}

// EnsureOverviewStub creates <vault>/<projects_dir>/<projectName>/Overview.md if it does
// not already exist. It never overwrites an existing file.
func EnsureOverviewStub(vaultPath, projectName, slug string, vaultCfg ...config.VaultConfig) error {
	projectsDir := "Projects"
	indexFile := "00-Dev-Index.md"
	if len(vaultCfg) > 0 {
		if vaultCfg[0].ProjectsDir != "" {
			projectsDir = vaultCfg[0].ProjectsDir
		}
		if vaultCfg[0].IndexFile != "" {
			indexFile = vaultCfg[0].IndexFile
		}
	}
	targetPath := overviewPath(vaultPath, projectsDir, projectName)
	_, err := os.Stat(targetPath)
	if err == nil {
		return nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("checking overview at %s: %w", targetPath, err)
	}

	rendered, err := RenderOverview(OverviewData{
		ProjectName: projectName,
		Slug:        slug,
		IndexFile:   indexFile,
	})
	if err != nil {
		return err
	}
	if err := writeFileAtomic(targetPath, []byte(rendered), 0o644); err != nil {
		return fmt.Errorf("writing overview file: %w", err)
	}
	return nil
}
