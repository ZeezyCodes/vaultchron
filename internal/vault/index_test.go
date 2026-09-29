package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZeezyCodes/vaultchron/internal/config"
)

// TestUpdateIndex_PrependsNewEntry verifies that UpdateIndex prepends a new
// entry in the format [[Projects/<Project>/Devlog/<Date>|<Date>]] — <Abstract>
// without deleting existing entries.
func TestUpdateIndex_PrependsNewEntry(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "00-Dev-Index.md")

	mockIndex := `# Dev Vault — Index

**Recent Dev Logs**

[[Projects/sampleAuth/Devlog/2026-09-26|2026-09-26]] — Some existing summary
[[Projects/sampleCMS/Devlog/2026-09-25|2026-09-25]] — Another existing summary

## 🏗️ Architecture
`
	if err := os.WriteFile(indexPath, []byte(mockIndex), 0o644); err != nil {
		t.Fatal(err)
	}

	vaultCfg := config.VaultConfig{
		Path:      tmpDir,
		IndexFile: "00-Dev-Index.md",
	}

	data := &DevlogData{
		ProjectName:  "AcmeWidgets.com",
		Date:         "2026-09-27",
		Content:      "**Architectural Evolution**\n\nSome content here",
		CommitsCount: 14,
		Shortstat:    "8 files changed, 780 insertions(+), 20 deletions(-)",
	}

	if err := UpdateIndex(vaultCfg, data); err != nil {
		t.Fatalf("UpdateIndex failed: %v", err)
	}

	raw, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}

	content := string(raw)

	// Verify new entry was prepended in the correct format.
	expectedEntry := "[[Projects/AcmeWidgets.com/Devlog/2026-09-27|2026-09-27]] — Architectural Evolution"
	if !strings.Contains(content, expectedEntry) {
		t.Errorf("index should contain new entry %q", expectedEntry)
	}

	// Verify existing entries are preserved.
	if !strings.Contains(content, "[[Projects/sampleAuth/Devlog/2026-09-26|2026-09-26]] — Some existing summary") {
		t.Error("existing entry for sampleAuth should be preserved")
	}
	if !strings.Contains(content, "[[Projects/sampleCMS/Devlog/2026-09-25|2026-09-25]] — Another existing summary") {
		t.Error("existing entry for sampleCMS should be preserved")
	}

	// Verify new entry appears before existing entries.
	newEntryIdx := strings.Index(content, expectedEntry)
	existingEntryIdx := strings.Index(content, "[[Projects/sampleAuth/Devlog/2026-09-26|2026-09-26]]")
	if newEntryIdx < 0 || existingEntryIdx < 0 || newEntryIdx > existingEntryIdx {
		t.Error("new entry should appear before existing entries")
	}
}

// TestUpdateIndex_NoDuplicateConsecutiveEntries verifies that calling
// UpdateIndex twice with the same date/project does not create duplicate
// consecutive entries — the existing entry is updated in place.
func TestUpdateIndex_NoDuplicateConsecutiveEntries(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "00-Dev-Index.md")

	mockIndex := `# Dev Vault — Index

**Recent Dev Logs**

[[Projects/AcmeWidgets.com/Devlog/2026-09-27|2026-09-27]] — Old summary
[[Projects/sampleAuth/Devlog/2026-09-26|2026-09-26]] — Some existing summary
`
	if err := os.WriteFile(indexPath, []byte(mockIndex), 0o644); err != nil {
		t.Fatal(err)
	}

	vaultCfg := config.VaultConfig{
		Path:      tmpDir,
		IndexFile: "00-Dev-Index.md",
	}

	data := &DevlogData{
		ProjectName:  "AcmeWidgets.com",
		Date:         "2026-09-27",
		Content:      "**New Abstract**\n\nUpdated content",
		CommitsCount: 14,
		Shortstat:    "8 files changed, 780 insertions(+), 20 deletions(-)",
	}

	if err := UpdateIndex(vaultCfg, data); err != nil {
		t.Fatalf("UpdateIndex failed: %v", err)
	}

	raw, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}

	content := string(raw)

	// Count occurrences of the AcmeWidgets.com entry for 2026-09-27.
	count := strings.Count(content, "[[Projects/AcmeWidgets.com/Devlog/2026-09-27|2026-09-27]]")
	if count != 1 {
		t.Errorf("expected exactly 1 entry for AcmeWidgets.com on 2026-09-27, got %d", count)
	}

	// Verify the entry was updated (not duplicated).
	expectedEntry := "[[Projects/AcmeWidgets.com/Devlog/2026-09-27|2026-09-27]] — New Abstract"
	if !strings.Contains(content, expectedEntry) {
		t.Errorf("entry should be updated to %q", expectedEntry)
	}

	// Verify existing entries are preserved.
	if !strings.Contains(content, "[[Projects/sampleAuth/Devlog/2026-09-26|2026-09-26]] — Some existing summary") {
		t.Error("existing entry for sampleAuth should be preserved")
	}
}

// TestUpdateIndex_TableMode_PrependsRowAfterSeparator verifies that when
// Recent Dev Logs contains a markdown table, UpdateIndex inserts a table row
// immediately after the separator row without corrupting table syntax.
func TestUpdateIndex_TableMode_PrependsRowAfterSeparator(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "00-Dev-Index.md")

	mockIndex := `# Dev Vault — Index

## 📅 Dev Logs

**Recent Dev Logs (Last 7 active entries):**

| Date | Summary |
|---|---|
| [2026-09-26](Daily-Rollups/2026-09-26.md) | 1 repo(s): AcmeWidgets.com |

> [!example]- 📦 Archive: September 2026 (19 logs)
> | Date | Summary |
> |---|---|
> | [2026-09-20](Daily-Rollups/2026-09-20.md) | 1 repo(s): AcmeWidgets.com |
`
	if err := os.WriteFile(indexPath, []byte(mockIndex), 0o644); err != nil {
		t.Fatal(err)
	}

	vaultCfg := config.VaultConfig{
		Path:      tmpDir,
		IndexFile: "00-Dev-Index.md",
	}

	data := &DevlogData{
		ProjectName:  "AcmeWidgets.com",
		Date:         "2026-09-27",
		Content:      "**Pipeline Reprocess Lifecycle (Version 8)**\n\nSome content",
		CommitsCount: 14,
		Shortstat:    "8 files changed, 780 insertions(+), 20 deletions(-)",
	}

	if err := UpdateIndex(vaultCfg, data); err != nil {
		t.Fatalf("UpdateIndex failed: %v", err)
	}

	raw, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}

	content := string(raw)

	// Verify table header and separator are intact and not separated by raw text.
	if !strings.Contains(content, "| Date | Summary |\n|---|---|\n| [[Projects/AcmeWidgets.com/Devlog/2026-09-27|2026-09-27]] | Pipeline Reprocess Lifecycle (Version 8) |") {
		t.Errorf("expected clean table row insertion after separator, got:\n%s", content)
	}

	// Verify existing table row is preserved below the new row.
	if !strings.Contains(content, "| [2026-09-26](Daily-Rollups/2026-09-26.md) | 1 repo(s): AcmeWidgets.com |") {
		t.Error("existing row should be preserved")
	}

	// Verify archive callout is unaffected.
	if !strings.Contains(content, "> [!example]- 📦 Archive: September 2026") {
		t.Error("archive callout should be preserved")
	}
}

// TestUpdateIndex_TableMode_UpdatesExistingRow verifies that calling UpdateIndex
// on an existing table row updates it in place rather than inserting duplicates.
func TestUpdateIndex_TableMode_UpdatesExistingRow(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "00-Dev-Index.md")

	mockIndex := `# Dev Vault — Index

**Recent Dev Logs**

| Date | Summary |
|---|---|
| [[Projects/AcmeWidgets.com/Devlog/2026-09-27|2026-09-27]] | Old Summary |
| [2026-09-26](Daily-Rollups/2026-09-26.md) | 1 repo(s): AcmeWidgets.com |
`
	if err := os.WriteFile(indexPath, []byte(mockIndex), 0o644); err != nil {
		t.Fatal(err)
	}

	vaultCfg := config.VaultConfig{
		Path:      tmpDir,
		IndexFile: "00-Dev-Index.md",
	}

	data := &DevlogData{
		ProjectName:  "AcmeWidgets.com",
		Date:         "2026-09-27",
		Content:      "**Updated Lifecycle Summary**\n\nContent",
		CommitsCount: 5,
		Shortstat:    "1 file changed",
	}

	if err := UpdateIndex(vaultCfg, data); err != nil {
		t.Fatalf("UpdateIndex failed: %v", err)
	}

	raw, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}

	content := string(raw)

	count := strings.Count(content, "[[Projects/AcmeWidgets.com/Devlog/2026-09-27|2026-09-27]]")
	if count != 1 {
		t.Errorf("expected exactly 1 entry for 2026-09-27, got %d", count)
	}

	expectedRow := "| [[Projects/AcmeWidgets.com/Devlog/2026-09-27|2026-09-27]] | Updated Lifecycle Summary |"
	if !strings.Contains(content, expectedRow) {
		t.Errorf("expected updated row %q in content:\n%s", expectedRow, content)
	}
}

// TestUpdateIndex_ListWithCollapsibleArchive verifies that when Recent Dev Logs
// is structured as a list followed by archive callouts, UpdateIndex prepends
// to the list without injecting content into or below the archive callout.
func TestUpdateIndex_ListWithCollapsibleArchive(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "00-Dev-Index.md")

	mockIndex := `# Dev Vault — Index

## 📅 Dev Logs

### Recent Activity

[[Projects/AcmeWidgets.com/Devlog/2026-09-26|2026-09-26]] — Deal Verification Engine

> [!example]- 📦 Archive: September 2026 (19 logs)
> | Date | Summary |
> |---|---|
> | [2026-09-20](Daily-Rollups/2026-09-20.md) | 1 repo(s): AcmeWidgets.com |
`
	if err := os.WriteFile(indexPath, []byte(mockIndex), 0o644); err != nil {
		t.Fatal(err)
	}

	vaultCfg := config.VaultConfig{
		Path:      tmpDir,
		IndexFile: "00-Dev-Index.md",
	}

	data := &DevlogData{
		ProjectName:  "vaultchron",
		Date:         "2026-09-27",
		Content:      "**Polyrepo Telemetry & Obsidian Pipeline Engine**\n\nContent",
		CommitsCount: 3,
		Shortstat:    "5 files changed",
	}

	if err := UpdateIndex(vaultCfg, data); err != nil {
		t.Fatalf("UpdateIndex failed: %v", err)
	}

	raw, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}

	content := string(raw)

	expectedEntry := "[[Projects/vaultchron/Devlog/2026-09-27|2026-09-27]] — Polyrepo Telemetry & Obsidian Pipeline Engine"
	if !strings.Contains(content, expectedEntry) {
		t.Errorf("expected entry %q in content", expectedEntry)
	}

	newEntryIdx := strings.Index(content, expectedEntry)
	oldEntryIdx := strings.Index(content, "[[Projects/AcmeWidgets.com/Devlog/2026-09-26|2026-09-26]]")
	archiveIdx := strings.Index(content, "> [!example]- 📦 Archive")

	if newEntryIdx >= oldEntryIdx || oldEntryIdx >= archiveIdx {
		t.Errorf("ordering invalid: new=%d, old=%d, archive=%d", newEntryIdx, oldEntryIdx, archiveIdx)
	}
}

// TestUpdateIndex_BulletListMode verifies that if existing list items use bullets,
// the newly prepended entry also adopts bullet formatting.
func TestUpdateIndex_BulletListMode(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "00-Dev-Index.md")

	mockIndex := `# Dev Vault — Index

### Recent Activity

- [[Projects/sampleAuth/Devlog/2026-09-26|2026-09-26]] — Auth refactoring

## 📁 Structure
`
	if err := os.WriteFile(indexPath, []byte(mockIndex), 0o644); err != nil {
		t.Fatal(err)
	}

	vaultCfg := config.VaultConfig{
		Path:      tmpDir,
		IndexFile: "00-Dev-Index.md",
	}

	data := &DevlogData{
		ProjectName:  "sampleCMS",
		Date:         "2026-09-27",
		Content:      "**Module DAG Ordering**\n\nContent",
		CommitsCount: 2,
		Shortstat:    "2 files changed",
	}

	if err := UpdateIndex(vaultCfg, data); err != nil {
		t.Fatalf("UpdateIndex failed: %v", err)
	}

	raw, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}

	content := string(raw)
	expectedEntry := "- [[Projects/sampleCMS/Devlog/2026-09-27|2026-09-27]] — Module DAG Ordering"
	if !strings.Contains(content, expectedEntry) {
		t.Errorf("expected bullet entry %q in content:\n%s", expectedEntry, content)
	}
}

// TestUpdateIndex_RealEcosystemHubIndex verifies that UpdateIndex works seamlessly
// with the rewritten 00-Dev-Index.md containing Callout v3 headers, Mermaid graphs,
// Active Projects tables, and collapsible monthly archives.
func TestUpdateIndex_RealEcosystemHubIndex(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "00-Dev-Index.md")

	hubContent := `# Dev Vault — Ecosystem Hub

> [!abstract] Engineering Ecosystem & System Catalog
> Central navigation hub for the developer's infrastructure and software engineering projects.

## 🏗️ System Topology & Architecture

` + "```mermaid\ngraph TD\n    HL --> REV_PROXY\n```" + `

## 📋 Active Projects

| Project | Runtime | Remote | Last Active / Status |
|---|---|---|---|
| [AcmeWidgets.com](Projects/AcmeWidgets.com/Overview.md) | Go 1.26, HTMX, SQLite | Forgejo | Active |

## 📅 Dev Logs

Daily rollup notes link to per-project devlogs.

### Recent Activity

[[Projects/vaultchron/Devlog/2026-09-27|2026-09-27]] — Polyrepo Telemetry & Obsidian Pipeline Engine
[[Projects/AcmeWidgets.com/Devlog/2026-09-27|2026-09-27]] — Pipeline Reprocess Lifecycle (Version 8)

> [!example]- 📦 Archive: September 2026 (19 logs)
> | Date | Summary |
> |---|---|
> | [2026-09-20](Daily-Rollups/2026-09-20.md) | 1 repo(s): AcmeWidgets.com |
`
	if err := os.WriteFile(indexPath, []byte(hubContent), 0o644); err != nil {
		t.Fatal(err)
	}

	vaultCfg := config.VaultConfig{
		Path:      tmpDir,
		IndexFile: "00-Dev-Index.md",
	}

	// 1. Prepend a new day's log (2026-09-28)
	newData := &DevlogData{
		ProjectName:  "AcmeWidgets.com",
		Date:         "2026-09-28",
		Content:      "**Cockpit Modernization & Index Engine Hardening**\n\nContent",
		CommitsCount: 4,
		Shortstat:    "3 files changed",
	}

	if err := UpdateIndex(vaultCfg, newData); err != nil {
		t.Fatalf("UpdateIndex failed: %v", err)
	}

	raw, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}

	content := string(raw)

	expectedEntry := "[[Projects/AcmeWidgets.com/Devlog/2026-09-28|2026-09-28]] — Cockpit Modernization & Index Engine Hardening"
	if !strings.Contains(content, expectedEntry) {
		t.Errorf("expected new entry %q in content:\n%s", expectedEntry, content)
	}

	newIdx := strings.Index(content, expectedEntry)
	vaultchronIdx := strings.Index(content, "[[Projects/vaultchron/Devlog/2026-09-27|2026-09-27]]")
	archiveIdx := strings.Index(content, "> [!example]- 📦 Archive: September 2026")

	if newIdx >= vaultchronIdx || vaultchronIdx >= archiveIdx {
		t.Errorf("ordering invalid: new=%d, vaultchron=%d, archive=%d", newIdx, vaultchronIdx, archiveIdx)
	}

	// 2. Update existing entry in place without duplicating
	updatedData := &DevlogData{
		ProjectName:  "AcmeWidgets.com",
		Date:         "2026-09-28",
		Content:      "**Updated Modernization Title**\n\nContent",
		CommitsCount: 5,
		Shortstat:    "4 files changed",
	}

	if err := UpdateIndex(vaultCfg, updatedData); err != nil {
		t.Fatalf("UpdateIndex update failed: %v", err)
	}

	rawUpdated, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}

	contentUpdated := string(rawUpdated)
	count := strings.Count(contentUpdated, "[[Projects/AcmeWidgets.com/Devlog/2026-09-28|2026-09-28]]")
	if count != 1 {
		t.Errorf("expected exactly 1 entry for 2026-09-28, got %d", count)
	}
	if !strings.Contains(contentUpdated, "Updated Modernization Title") {
		t.Error("expected updated title in content")
	}
}
