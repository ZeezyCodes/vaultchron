package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZeezyCodes/vaultchron/internal/config"
)

func sampleDevlogData(partial bool) DevlogData {
	return DevlogData{
		Date:         "2026-10-03",
		ProjectName:  "AcmeWidgets.com",
		Slug:         "acmewidgets",
		Lang:         "go",
		Branch:       "main",
		CommitsCount: 5,
		Shortstat:    "2 files changed, 10 insertions(+)",
		TopPackages:  []string{"internal/collector", "internal/vault"},
		Model:        "gemini-test",
		Now:          "2026-10-03T12:00:00Z",
		Content:      "> [!abstract] Architectural Evolution\nSample abstract content.",
		Partial:      partial,
	}
}

func TestRenderDevlog_WithAndWithoutPartial(t *testing.T) {
	// Without Partial (must match original output structure without partial: true)
	dataWithout := sampleDevlogData(false)
	renderedWithout, err := RenderDevlog(dataWithout)
	if err != nil {
		t.Fatalf("RenderDevlog without partial failed: %v", err)
	}
	if strings.Contains(renderedWithout, "partial:") {
		t.Errorf("rendered output without partial contains 'partial:':\n%s", renderedWithout)
	}
	expectedWithout := "generated: 2026-10-03T12:00:00Z\n---"
	if !strings.Contains(renderedWithout, expectedWithout) {
		t.Errorf("expected %q in rendered output without partial:\n%s", expectedWithout, renderedWithout)
	}

	// With Partial (must include partial: true right after generated:)
	dataWith := sampleDevlogData(true)
	renderedWith, err := RenderDevlog(dataWith)
	if err != nil {
		t.Fatalf("RenderDevlog with partial failed: %v", err)
	}
	expectedWith := "generated: 2026-10-03T12:00:00Z\npartial: true\n---"
	if !strings.Contains(renderedWith, expectedWith) {
		t.Errorf("expected %q in rendered output with partial:\n%s", expectedWith, renderedWith)
	}
}

func TestDevlogExists_NotExists(t *testing.T) {
	vaultPath := t.TempDir()
	exists, partial, err := DevlogExists(vaultPath, "AcmeWidgets.com", "2026-10-03")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Errorf("expected exists = false, got true")
	}
	if partial {
		t.Errorf("expected partial = false, got true")
	}
}

func TestDevlogExists_NonPartial(t *testing.T) {
	vaultPath := t.TempDir()
	data := sampleDevlogData(false)
	_, err := WriteDevlog(vaultPath, "AcmeWidgets.com", "2026-10-03", data)
	if err != nil {
		t.Fatalf("WriteDevlog failed: %v", err)
	}

	exists, partial, err := DevlogExists(vaultPath, "AcmeWidgets.com", "2026-10-03")
	if err != nil {
		t.Fatalf("DevlogExists failed: %v", err)
	}
	if !exists {
		t.Errorf("expected exists = true, got false")
	}
	if partial {
		t.Errorf("expected partial = false, got true")
	}
}

func TestDevlogExists_Partial(t *testing.T) {
	vaultPath := t.TempDir()
	data := sampleDevlogData(true)
	_, err := WriteDevlog(vaultPath, "AcmeWidgets.com", "2026-10-03", data)
	if err != nil {
		t.Fatalf("WriteDevlog failed: %v", err)
	}

	exists, partial, err := DevlogExists(vaultPath, "AcmeWidgets.com", "2026-10-03")
	if err != nil {
		t.Fatalf("DevlogExists failed: %v", err)
	}
	if !exists {
		t.Errorf("expected exists = true, got false")
	}
	if !partial {
		t.Errorf("expected partial = true, got false")
	}
}

func TestDevlogExists_PartialInBodyIgnored(t *testing.T) {
	vaultPath := t.TempDir()
	dir := filepath.Join(vaultPath, "Projects", "AcmeWidgets.com", "Devlog")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	// Frontmatter does NOT have partial: true, but body does
	content := "---\ndate: 2026-10-03\nproject: AcmeWidgets.com\ngenerated: 2026-10-03T12:00:00Z\n---\n\n# Technical Journal\npartial: true\nSome more text\n"
	notePath := filepath.Join(dir, "2026-10-03.md")
	if err := os.WriteFile(notePath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	exists, partial, err := DevlogExists(vaultPath, "AcmeWidgets.com", "2026-10-03")
	if err != nil {
		t.Fatalf("DevlogExists failed: %v", err)
	}
	if !exists {
		t.Errorf("expected exists = true, got false")
	}
	if partial {
		t.Errorf("expected partial = false for body partial line, got true")
	}
}

func TestDevlogExists_CRLF(t *testing.T) {
	vaultPath := t.TempDir()
	dir := filepath.Join(vaultPath, "Projects", "AcmeWidgets.com", "Devlog")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	// CRLF note with partial: true
	crlfPartial := "---\r\ndate: 2026-10-03\r\nproject: AcmeWidgets.com\r\ngenerated: 2026-10-03T12:00:00Z\r\npartial: true\r\n---\r\n\r\n# Technical Journal\r\n"
	notePartialPath := filepath.Join(dir, "2026-10-03.md")
	if err := os.WriteFile(notePartialPath, []byte(crlfPartial), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	exists, partial, err := DevlogExists(vaultPath, "AcmeWidgets.com", "2026-10-03")
	if err != nil {
		t.Fatalf("DevlogExists CRLF partial failed: %v", err)
	}
	if !exists {
		t.Errorf("expected exists = true, got false")
	}
	if !partial {
		t.Errorf("expected partial = true, got false")
	}

	// CRLF note without partial
	crlfNonPartial := "---\r\ndate: 2026-10-04\r\nproject: AcmeWidgets.com\r\ngenerated: 2026-10-04T12:00:00Z\r\n---\r\n\r\n# Technical Journal\r\n"
	noteNonPartialPath := filepath.Join(dir, "2026-10-04.md")
	if err := os.WriteFile(noteNonPartialPath, []byte(crlfNonPartial), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	exists, partial, err = DevlogExists(vaultPath, "AcmeWidgets.com", "2026-10-04")
	if err != nil {
		t.Fatalf("DevlogExists CRLF non-partial failed: %v", err)
	}
	if !exists {
		t.Errorf("expected exists = true, got false")
	}
	if partial {
		t.Errorf("expected partial = false, got true")
	}
}

func TestRenderTemplate_CRLFTemplateRendersLF(t *testing.T) {
	raw, err := templatesFS.ReadFile("templates/devlog.md.tmpl")
	if err != nil {
		t.Fatalf("reading embedded template: %v", err)
	}

	lf := strings.ReplaceAll(string(raw), "\r\n", "\n")
	crlf := strings.ReplaceAll(lf, "\n", "\r\n")

	for _, partial := range []bool{false, true} {
		data := sampleDevlogData(partial)
		renderedLF, err := renderTemplate([]byte(lf), data)
		if err != nil {
			t.Fatalf("renderTemplate with LF template (partial=%v) failed: %v", partial, err)
		}
		renderedCRLF, err := renderTemplate([]byte(crlf), data)
		if err != nil {
			t.Fatalf("renderTemplate with CRLF template (partial=%v) failed: %v", partial, err)
		}
		if renderedCRLF != renderedLF {
			t.Errorf("rendered output with CRLF template differs from LF template (partial=%v)", partial)
		}
		if strings.Contains(renderedCRLF, "\r") {
			t.Errorf("rendered output with CRLF template contains carriage return (partial=%v)", partial)
		}
	}
}

func TestRenderOverview(t *testing.T) {
	data := OverviewData{
		ProjectName: "AcmeWidgets.com",
		Slug:        "acmewidgets",
	}

	rendered, err := RenderOverview(data)
	if err != nil {
		t.Fatalf("RenderOverview failed: %v", err)
	}

	// Verify frontmatter
	if !strings.Contains(rendered, "project: AcmeWidgets.com") {
		t.Errorf("expected frontmatter with project: AcmeWidgets.com, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "type/overview") {
		t.Errorf("expected tag type/overview, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "project/acmewidgets") {
		t.Errorf("expected tag project/acmewidgets, got:\n%s", rendered)
	}

	// Verify breadcrumb
	if !strings.Contains(rendered, "[[00-Dev-Index|🏠 Index]] / AcmeWidgets.com") {
		t.Errorf("expected breadcrumb, got:\n%s", rendered)
	}

	// Verify heading
	if !strings.Contains(rendered, "# AcmeWidgets.com") {
		t.Errorf("expected heading # AcmeWidgets.com, got:\n%s", rendered)
	}

	// Verify callout
	if !strings.Contains(rendered, "> [!note]") {
		t.Errorf("expected [!note] callout, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "placeholder created by VaultChron") {
		t.Errorf("expected placeholder text in callout, got:\n%s", rendered)
	}
}

func TestEnsureOverviewStub(t *testing.T) {
	vaultPath := t.TempDir()
	projName := "AcmeWidgets.com"
	slug := "acmewidgets"

	// 1. Initial creation
	if err := EnsureOverviewStub(vaultPath, projName, slug); err != nil {
		t.Fatalf("EnsureOverviewStub failed: %v", err)
	}

	stubPath := filepath.Join(vaultPath, "Projects", projName, "Overview.md")
	content, err := os.ReadFile(stubPath)
	if err != nil {
		t.Fatalf("reading overview stub: %v", err)
	}
	if !strings.Contains(string(content), "type/overview") {
		t.Errorf("expected overview content, got:\n%s", string(content))
	}

	// 2. Modify content to custom user content
	customContent := "# Custom Overview\n\nUser written documentation\n"
	if err := os.WriteFile(stubPath, []byte(customContent), 0o644); err != nil {
		t.Fatalf("writing custom overview: %v", err)
	}

	// 3. EnsureOverviewStub again: should never overwrite
	if err := EnsureOverviewStub(vaultPath, projName, slug); err != nil {
		t.Fatalf("EnsureOverviewStub rerun failed: %v", err)
	}

	contentAfter, err := os.ReadFile(stubPath)
	if err != nil {
		t.Fatalf("reading overview stub after rerun: %v", err)
	}
	if string(contentAfter) != customContent {
		t.Errorf("overview stub was overwritten; expected %q, got %q", customContent, string(contentAfter))
	}
}

func TestCustomVaultPaths(t *testing.T) {
	vaultPath := t.TempDir()
	customProjects := "Dev/Projects"
	customIndex := "Meta/Dev-Index.md"
	vCfg := config.VaultConfig{
		Path:        vaultPath,
		ProjectsDir: customProjects,
		IndexFile:   customIndex,
	}

	data := sampleDevlogData(false)
	data.ProjectsDir = customProjects
	data.IndexFile = customIndex

	// 1. Write devlog under custom projects dir
	writtenPath, err := WriteDevlog(vaultPath, data.ProjectName, data.Date, data)
	if err != nil {
		t.Fatalf("WriteDevlog failed: %v", err)
	}

	expectedPath := filepath.Join(vaultPath, "Dev", "Projects", data.ProjectName, "Devlog", data.Date+".md")
	if writtenPath != expectedPath {
		t.Errorf("expected devlog written to %s, got %s", expectedPath, writtenPath)
	}

	// 2. DevlogExists finds it with custom projects dir, not with default
	exists, _, err := DevlogExists(vaultPath, data.ProjectName, data.Date, customProjects)
	if err != nil || !exists {
		t.Fatalf("DevlogExists(custom) failed: exists=%v, err=%v", exists, err)
	}
	defaultExists, _, _ := DevlogExists(vaultPath, data.ProjectName, data.Date)
	if defaultExists {
		t.Errorf("DevlogExists(default) should not find note written under custom projects dir")
	}

	// 3. Devlog breadcrumb uses custom links
	noteBytes, err := os.ReadFile(writtenPath)
	if err != nil {
		t.Fatalf("reading written note: %v", err)
	}
	expectedDevlogBreadcrumb := "[[Meta/Dev-Index|🏠 Index]] / [[Dev/Projects/AcmeWidgets.com/Overview|AcmeWidgets.com]] / [[Daily-Rollups/2026-10-03|📅 Rollup]]"
	if !strings.Contains(string(noteBytes), expectedDevlogBreadcrumb) {
		t.Errorf("devlog breadcrumb missing expected custom link:\n%s", string(noteBytes))
	}

	// 4. EnsureOverviewStub creates stub in custom projects dir with custom index link
	if err := EnsureOverviewStub(vaultPath, data.ProjectName, data.Slug, vCfg); err != nil {
		t.Fatalf("EnsureOverviewStub failed: %v", err)
	}
	stubPath := filepath.Join(vaultPath, "Dev", "Projects", data.ProjectName, "Overview.md")
	stubBytes, err := os.ReadFile(stubPath)
	if err != nil {
		t.Fatalf("reading overview stub: %v", err)
	}
	expectedStubBreadcrumb := "[[Meta/Dev-Index|🏠 Index]] / AcmeWidgets.com"
	if !strings.Contains(string(stubBytes), expectedStubBreadcrumb) {
		t.Errorf("overview stub missing expected custom link:\n%s", string(stubBytes))
	}
}
