package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
