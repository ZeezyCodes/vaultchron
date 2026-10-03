package vault

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZeezyCodes/vaultchron/internal/config"
)

func TestUpdateIndex_SortedTableDriven(t *testing.T) {
	tests := []struct {
		name       string
		setupIndex string
		updates    []*DevlogData
		verify     func(t *testing.T, indexPath string, raw []byte)
	}{
		{
			name: "empty index with table header",
			setupIndex: `# Index
## Recent Dev Logs
| Date | Notes |
|---|---|
`,
			updates: []*DevlogData{
				{
					ProjectName:  "AcmeWidgets.com",
					Date:         "2026-10-03",
					Content:      "**First Entry**",
					CommitsCount: 1,
				},
			},
			verify: func(t *testing.T, indexPath string, raw []byte) {
				content := string(raw)
				expected := "| [[Projects/AcmeWidgets.com/Devlog/2026-10-03|2026-10-03]] | First Entry |"
				if !strings.Contains(content, expected) {
					t.Errorf("expected %q in content:\n%s", expected, content)
				}
			},
		},
		{
			name: "empty index with list header",
			setupIndex: `# Index
## Recent Activity
`,
			updates: []*DevlogData{
				{
					ProjectName:  "AcmeWidgets.com",
					Date:         "2026-10-03",
					Content:      "**First List Entry**",
					CommitsCount: 1,
				},
			},
			verify: func(t *testing.T, indexPath string, raw []byte) {
				content := string(raw)
				expected := "[[Projects/AcmeWidgets.com/Devlog/2026-10-03|2026-10-03]] — First List Entry"
				if !strings.Contains(content, expected) {
					t.Errorf("expected %q in content:\n%s", expected, content)
				}
			},
		},
		{
			name: "older date written after a newer one (lands below it)",
			setupIndex: `# Index
## Recent Dev Logs
| Date | Notes |
|---|---|
| [[Projects/AcmeWidgets.com/Devlog/2026-10-05|2026-10-05]] | Newer |
`,
			updates: []*DevlogData{
				{
					ProjectName:  "AcmeWidgets.com",
					Date:         "2026-10-02",
					Content:      "**Older**",
					CommitsCount: 1,
				},
			},
			verify: func(t *testing.T, indexPath string, raw []byte) {
				content := string(raw)
				idxNewer := strings.Index(content, "2026-10-05")
				idxOlder := strings.Index(content, "2026-10-02")
				if idxNewer == -1 || idxOlder == -1 || idxOlder <= idxNewer {
					t.Errorf("expected 2026-10-02 to land below 2026-10-05, got content:\n%s", content)
				}
			},
		},
		{
			name: "newer date lands above",
			setupIndex: `# Index
## Recent Dev Logs
| Date | Notes |
|---|---|
| [[Projects/AcmeWidgets.com/Devlog/2026-10-02|2026-10-02]] | Older |
`,
			updates: []*DevlogData{
				{
					ProjectName:  "AcmeWidgets.com",
					Date:         "2026-10-05",
					Content:      "**Newer**",
					CommitsCount: 1,
				},
			},
			verify: func(t *testing.T, indexPath string, raw []byte) {
				content := string(raw)
				idxNewer := strings.Index(content, "2026-10-05")
				idxOlder := strings.Index(content, "2026-10-02")
				if idxNewer == -1 || idxOlder == -1 || idxNewer >= idxOlder {
					t.Errorf("expected 2026-10-05 to land above 2026-10-02, got content:\n%s", content)
				}
			},
		},
		{
			name: "same date, two projects (alphabetical)",
			setupIndex: `# Index
## Recent Dev Logs
| Date | Notes |
|---|---|
| [[Projects/ZooProject/Devlog/2026-10-03|2026-10-03]] | Zoo |
`,
			updates: []*DevlogData{
				{
					ProjectName:  "AlphaProject",
					Date:         "2026-10-03",
					Content:      "**Alpha**",
					CommitsCount: 1,
				},
			},
			verify: func(t *testing.T, indexPath string, raw []byte) {
				content := string(raw)
				idxAlpha := strings.Index(content, "AlphaProject")
				idxZoo := strings.Index(content, "ZooProject")
				if idxAlpha == -1 || idxZoo == -1 || idxAlpha >= idxZoo {
					t.Errorf("expected AlphaProject to land before ZooProject on same date, got content:\n%s", content)
				}
			},
		},
		{
			name: "user-written 3-column table (| Date | Project | Notes |) gets correct 3-cell row",
			setupIndex: `# Index
## Recent Dev Logs
| Date | Project | Notes |
|---|---|---|
`,
			updates: []*DevlogData{
				{
					ProjectName:  "AcmeWidgets.com",
					Date:         "2026-10-03",
					Content:      "**Three Column Test**",
					CommitsCount: 1,
				},
			},
			verify: func(t *testing.T, indexPath string, raw []byte) {
				content := string(raw)
				expected := "| [[Projects/AcmeWidgets.com/Devlog/2026-10-03|2026-10-03]] | AcmeWidgets.com | Three Column Test |"
				if !strings.Contains(content, expected) {
					t.Errorf("expected 3-cell row %q, got:\n%s", expected, content)
				}
			},
		},
		{
			name: "2-column table unchanged",
			setupIndex: `# Index
## Recent Dev Logs
| Date | Notes |
|---|---|
`,
			updates: []*DevlogData{
				{
					ProjectName:  "AcmeWidgets.com",
					Date:         "2026-10-03",
					Content:      "**Two Column Test**",
					CommitsCount: 1,
				},
			},
			verify: func(t *testing.T, indexPath string, raw []byte) {
				content := string(raw)
				expected := "| [[Projects/AcmeWidgets.com/Devlog/2026-10-03|2026-10-03]] | Two Column Test |"
				if !strings.Contains(content, expected) {
					t.Errorf("expected 2-cell row %q, got:\n%s", expected, content)
				}
			},
		},
		{
			name: "list mode",
			setupIndex: `# Index
## Recent Activity
- [[Projects/Beta/Devlog/2026-10-02|2026-10-02]] — Beta note
`,
			updates: []*DevlogData{
				{
					ProjectName:  "Alpha",
					Date:         "2026-10-04",
					Content:      "**Alpha note**",
					CommitsCount: 1,
				},
				{
					ProjectName:  "Gamma",
					Date:         "2026-10-01",
					Content:      "**Gamma note**",
					CommitsCount: 1,
				},
			},
			verify: func(t *testing.T, indexPath string, raw []byte) {
				content := string(raw)
				idxAlpha := strings.Index(content, "2026-10-04")
				idxBeta := strings.Index(content, "2026-10-02")
				idxGamma := strings.Index(content, "2026-10-01")
				if idxAlpha == -1 || idxBeta == -1 || idxGamma == -1 {
					t.Fatalf("missing entries in list mode:\n%s", content)
				}
				if !(idxAlpha < idxBeta && idxBeta < idxGamma) {
					t.Errorf("expected order Alpha < Beta < Gamma in list mode:\n%s", content)
				}
			},
		},
		{
			name: "unparseable user rows interleaved stay in place",
			setupIndex: `# Index
## Recent Dev Logs
| Date | Notes |
|---|---|
| [[Projects/Foo/Devlog/2026-10-05|2026-10-05]] | Foo 5 |
| User manual unparseable row 1 |
| [[Projects/Foo/Devlog/2026-10-01|2026-10-01]] | Foo 1 |
| User manual unparseable row 2 |
`,
			updates: []*DevlogData{
				{
					ProjectName:  "Foo",
					Date:         "2026-10-03",
					Content:      "**Foo 3**",
					CommitsCount: 1,
				},
			},
			verify: func(t *testing.T, indexPath string, raw []byte) {
				content := string(raw)
				idx5 := strings.Index(content, "2026-10-05")
				idxUser1 := strings.Index(content, "User manual unparseable row 1")
				idx3 := strings.Index(content, "2026-10-03")
				idx1 := strings.Index(content, "2026-10-01")
				idxUser2 := strings.Index(content, "User manual unparseable row 2")

				if idx5 == -1 || idxUser1 == -1 || idx3 == -1 || idx1 == -1 || idxUser2 == -1 {
					t.Fatalf("missing rows in content:\n%s", content)
				}

				if !(idx5 < idxUser1 && idxUser1 < idx3 && idx3 < idx1 && idx1 < idxUser2) {
					t.Errorf("unparseable rows were moved or reordered incorrectly:\n%s", content)
				}
			},
		},
		{
			name: "idempotent rerun is byte-identical",
			setupIndex: `# Index
## Recent Dev Logs
| Date | Notes |
|---|---|
| [[Projects/Foo/Devlog/2026-10-01|2026-10-01]] | Foo 1 |
`,
			updates: []*DevlogData{
				{
					ProjectName:  "Foo",
					Date:         "2026-10-03",
					Content:      "**Foo 3**",
					CommitsCount: 1,
				},
			},
			verify: func(t *testing.T, indexPath string, raw []byte) {
				vaultCfg := config.VaultConfig{
					Path:      filepath.Dir(indexPath),
					IndexFile: filepath.Base(indexPath),
				}
				data := &DevlogData{
					ProjectName:  "Foo",
					Date:         "2026-10-03",
					Content:      "**Foo 3**",
					CommitsCount: 1,
				}
				if err := UpdateIndex(vaultCfg, data); err != nil {
					t.Fatalf("second UpdateIndex failed: %v", err)
				}
				secondRaw, err := os.ReadFile(indexPath)
				if err != nil {
					t.Fatalf("reading second raw: %v", err)
				}
				if !bytes.Equal(raw, secondRaw) {
					t.Errorf("rerun not byte-identical:\nFirst:\n%s\nSecond:\n%s", string(raw), string(secondRaw))
				}
			},
		},
		{
			name:       "CRLF index preserved",
			setupIndex: "# Index\r\n## Recent Dev Logs\r\n| Date | Notes |\r\n|---|---|\r\n| [[Projects/Foo/Devlog/2026-10-01|2026-10-01]] | Foo 1 |\r\n",
			updates: []*DevlogData{
				{
					ProjectName:  "Foo",
					Date:         "2026-10-03",
					Content:      "**Foo 3**",
					CommitsCount: 1,
				},
			},
			verify: func(t *testing.T, indexPath string, raw []byte) {
				content := string(raw)
				if !strings.Contains(content, "\r\n") {
					t.Errorf("expected CRLF in file, got none")
				}
				lines := strings.Split(content, "\n")
				for i, l := range lines {
					if i == len(lines)-1 && l == "" {
						continue
					}
					if !strings.HasSuffix(l, "\r") {
						t.Errorf("line %d does not end in \\r: %q", i, l)
					}
				}
			},
		},
		{
			name: "same date + project replaced in place",
			setupIndex: `# Index
## Recent Dev Logs
| Date | Notes |
|---|---|
| [[Projects/Foo/Devlog/2026-10-05|2026-10-05]] | Foo 5 |
| [[Projects/Foo/Devlog/2026-10-03|2026-10-03]] | Old summary |
| [[Projects/Foo/Devlog/2026-10-01|2026-10-01]] | Foo 1 |
`,
			updates: []*DevlogData{
				{
					ProjectName:  "Foo",
					Date:         "2026-10-03",
					Content:      "**Updated summary**",
					CommitsCount: 1,
				},
			},
			verify: func(t *testing.T, indexPath string, raw []byte) {
				content := string(raw)
				if strings.Contains(content, "Old summary") {
					t.Errorf("expected Old summary to be replaced, but it was found:\n%s", content)
				}
				if !strings.Contains(content, "Updated summary") {
					t.Errorf("expected Updated summary in content:\n%s", content)
				}
				count := strings.Count(content, "2026-10-03")
				if count != 2 { // link has date twice: [[.../2026-10-03|2026-10-03]]
					t.Errorf("expected date 2026-10-03 to appear in exactly one row (count=2 in link), got count=%d in:\n%s", count, content)
				}
				// Verify relative ordering is unchanged
				idx5 := strings.Index(content, "2026-10-05")
				idx3 := strings.Index(content, "2026-10-03")
				idx1 := strings.Index(content, "2026-10-01")
				if !(idx5 < idx3 && idx3 < idx1) {
					t.Errorf("expected row to remain in place between 10-05 and 10-01, got content:\n%s", content)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			indexPath := filepath.Join(tmpDir, "00-Dev-Index.md")
			if err := os.WriteFile(indexPath, []byte(tt.setupIndex), 0o644); err != nil {
				t.Fatalf("writing setup index: %v", err)
			}
			vaultCfg := config.VaultConfig{
				Path:      tmpDir,
				IndexFile: "00-Dev-Index.md",
			}
			for _, u := range tt.updates {
				if err := UpdateIndex(vaultCfg, u); err != nil {
					t.Fatalf("UpdateIndex failed: %v", err)
				}
			}
			raw, err := os.ReadFile(indexPath)
			if err != nil {
				t.Fatalf("reading index file: %v", err)
			}
			tt.verify(t, indexPath, raw)
		})
	}
}
