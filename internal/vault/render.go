package vault

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
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

// WriteDevlog renders the template and writes the devlog to
// <vault>/Projects/<project>/Devlog/<date>.md, creating directories as needed.
func WriteDevlog(vaultPath, projectName, date string, data DevlogData) (string, error) {
	devlogDir := filepath.Join(vaultPath, "Projects", projectName, "Devlog")
	if err := os.MkdirAll(devlogDir, 0o755); err != nil {
		return "", fmt.Errorf("creating devlog directory: %w", err)
	}
	devlogPath := filepath.Join(devlogDir, date+".md")

	rendered, err := RenderDevlog(data)
	if err != nil {
		return "", err
	}
	if err := writeFileAtomic(devlogPath, []byte(rendered), 0o644); err != nil {
		return "", fmt.Errorf("writing devlog file: %w", err)
	}
	return devlogPath, nil
}
