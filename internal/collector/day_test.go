package collector

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewDayWindow(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*3600)
	now := time.Date(2026, 10, 3, 14, 30, 0, 0, loc)

	tests := []struct {
		name        string
		date        string
		loc         *time.Location
		now         time.Time
		wantErr     bool
		wantPartial bool
		wantStart   time.Time
		wantEnd     time.Time
	}{
		{
			name:    "invalid format text",
			date:    "not-a-date",
			loc:     loc,
			now:     now,
			wantErr: true,
		},
		{
			name:    "invalid format short month",
			date:    "2026-9-3",
			loc:     loc,
			now:     now,
			wantErr: true,
		},
		{
			name:    "invalid month out of range",
			date:    "2026-13-01",
			loc:     loc,
			now:     now,
			wantErr: true,
		},
		{
			name:    "invalid day out of range",
			date:    "2026-02-30",
			loc:     loc,
			now:     now,
			wantErr: true,
		},
		{
			name:    "future date tomorrow",
			date:    "2026-10-04",
			loc:     loc,
			now:     now,
			wantErr: true,
		},
		{
			name:    "future date far",
			date:    "2027-01-01",
			loc:     loc,
			now:     now,
			wantErr: true,
		},
		{
			name:        "today partial true",
			date:        "2026-10-03",
			loc:         loc,
			now:         now,
			wantErr:     false,
			wantPartial: true,
			wantStart:   time.Date(2026, 10, 3, 0, 0, 0, 0, loc),
			wantEnd:     time.Date(2026, 10, 4, 0, 0, 0, 0, loc),
		},
		{
			name:        "yesterday partial false",
			date:        "2026-10-02",
			loc:         loc,
			now:         now,
			wantErr:     false,
			wantPartial: false,
			wantStart:   time.Date(2026, 10, 2, 0, 0, 0, 0, loc),
			wantEnd:     time.Date(2026, 10, 3, 0, 0, 0, 0, loc),
		},
		{
			name:        "past date far",
			date:        "2026-01-01",
			loc:         loc,
			now:         now,
			wantErr:     false,
			wantPartial: false,
			wantStart:   time.Date(2026, 1, 1, 0, 0, 0, 0, loc),
			wantEnd:     time.Date(2026, 1, 2, 0, 0, 0, 0, loc),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, err := NewDayWindow(tt.date, tt.loc, tt.now)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewDayWindow(%q) err = %v, wantErr = %v", tt.date, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if w.Date != tt.date {
				t.Errorf("w.Date = %q, want %q", w.Date, tt.date)
			}
			if w.Partial != tt.wantPartial {
				t.Errorf("w.Partial = %v, want %v", w.Partial, tt.wantPartial)
			}
			if !w.Start.Equal(tt.wantStart) {
				t.Errorf("w.Start = %v, want %v", w.Start, tt.wantStart)
			}
			if !w.End.Equal(tt.wantEnd) {
				t.Errorf("w.End = %v, want %v", w.End, tt.wantEnd)
			}
		})
	}
}

func setupIsolatedTestRepo(t *testing.T) (string, func(envDates []string, args ...string)) {
	t.Helper()
	dir := t.TempDir()
	emptyConfig := filepath.Join(dir, ".emptyconfig")
	if err := os.WriteFile(emptyConfig, []byte(""), 0o600); err != nil {
		t.Fatalf("failed to create empty gitconfig: %v", err)
	}

	baseEnv := []string{
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=" + emptyConfig,
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	}

	runGit := func(envDates []string, args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), baseEnv...)
		cmd.Env = append(cmd.Env, envDates...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v\nOutput: %s", strings.Join(args, " "), err, string(out))
		}
	}

	runGit(nil, "init")
	runGit(nil, "config", "user.name", "test")
	runGit(nil, "config", "user.email", "test@example.com")

	return dir, runGit
}

func TestScanRepositoryDay_ThreeConsecutiveDays(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*3600)
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, loc)
	dir, runGit := setupIsolatedTestRepo(t)

	// Day 1: 2026-09-28
	if err := os.WriteFile(filepath.Join(dir, "day1.txt"), []byte("Day 1 content\n"), 0o644); err != nil {
		t.Fatalf("writing day1.txt: %v", err)
	}
	runGit(nil, "add", "day1.txt")
	runGit([]string{
		"GIT_AUTHOR_DATE=2026-09-28T10:00:00-05:00",
		"GIT_COMMITTER_DATE=2026-09-28T10:00:00-05:00",
	}, "commit", "-m", "Commit on day 1")

	// Day 2: 2026-09-29
	if err := os.WriteFile(filepath.Join(dir, "day2.txt"), []byte("Day 2 content\n"), 0o644); err != nil {
		t.Fatalf("writing day2.txt: %v", err)
	}
	runGit(nil, "add", "day2.txt")
	runGit([]string{
		"GIT_AUTHOR_DATE=2026-09-29T10:00:00-05:00",
		"GIT_COMMITTER_DATE=2026-09-29T10:00:00-05:00",
	}, "commit", "-m", "Commit on day 2")

	// Day 3: 2026-09-30
	if err := os.WriteFile(filepath.Join(dir, "day3.txt"), []byte("Day 3 content\n"), 0o644); err != nil {
		t.Fatalf("writing day3.txt: %v", err)
	}
	runGit(nil, "add", "day3.txt")
	runGit([]string{
		"GIT_AUTHOR_DATE=2026-09-30T10:00:00-05:00",
		"GIT_COMMITTER_DATE=2026-09-30T10:00:00-05:00",
	}, "commit", "-m", "Commit on day 3")

	ctx := context.Background()

	// Verify Day 1
	w1, err := NewDayWindow("2026-09-28", loc, now)
	if err != nil {
		t.Fatalf("NewDayWindow day1: %v", err)
	}
	meta1, err := ScanRepositoryDay(ctx, dir, w1)
	if err != nil {
		t.Fatalf("ScanRepositoryDay day1: %v", err)
	}
	if meta1 == nil || meta1.CommitsCount != 1 {
		t.Fatalf("expected 1 commit for day 1, got %+v", meta1)
	}
	if !strings.Contains(meta1.UnifiedDiff, "Day 1 content") {
		t.Errorf("day 1 diff missing day 1 content: %s", meta1.UnifiedDiff)
	}
	if strings.Contains(meta1.UnifiedDiff, "Day 2 content") || strings.Contains(meta1.UnifiedDiff, "Day 3 content") {
		t.Errorf("day 1 diff contains content from other days: %s", meta1.UnifiedDiff)
	}

	// Verify Day 2
	w2, err := NewDayWindow("2026-09-29", loc, now)
	if err != nil {
		t.Fatalf("NewDayWindow day2: %v", err)
	}
	meta2, err := ScanRepositoryDay(ctx, dir, w2)
	if err != nil {
		t.Fatalf("ScanRepositoryDay day2: %v", err)
	}
	if meta2 == nil || meta2.CommitsCount != 1 {
		t.Fatalf("expected 1 commit for day 2, got %+v", meta2)
	}
	if !strings.Contains(meta2.UnifiedDiff, "Day 2 content") {
		t.Errorf("day 2 diff missing day 2 content: %s", meta2.UnifiedDiff)
	}
	if strings.Contains(meta2.UnifiedDiff, "Day 1 content") || strings.Contains(meta2.UnifiedDiff, "Day 3 content") {
		t.Errorf("day 2 diff contains content from other days: %s", meta2.UnifiedDiff)
	}

	// Verify Day 3
	w3, err := NewDayWindow("2026-09-30", loc, now)
	if err != nil {
		t.Fatalf("NewDayWindow day3: %v", err)
	}
	meta3, err := ScanRepositoryDay(ctx, dir, w3)
	if err != nil {
		t.Fatalf("ScanRepositoryDay day3: %v", err)
	}
	if meta3 == nil || meta3.CommitsCount != 1 {
		t.Fatalf("expected 1 commit for day 3, got %+v", meta3)
	}
	if !strings.Contains(meta3.UnifiedDiff, "Day 3 content") {
		t.Errorf("day 3 diff missing day 3 content: %s", meta3.UnifiedDiff)
	}
	if strings.Contains(meta3.UnifiedDiff, "Day 1 content") || strings.Contains(meta3.UnifiedDiff, "Day 2 content") {
		t.Errorf("day 3 diff contains content from other days: %s", meta3.UnifiedDiff)
	}
}

func TestScanRepositoryDay_NoCommits(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*3600)
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, loc)
	dir, runGit := setupIsolatedTestRepo(t)

	if err := os.WriteFile(filepath.Join(dir, "init.txt"), []byte("init\n"), 0o644); err != nil {
		t.Fatalf("writing init.txt: %v", err)
	}
	runGit(nil, "add", "init.txt")
	runGit([]string{
		"GIT_AUTHOR_DATE=2026-09-28T10:00:00-05:00",
		"GIT_COMMITTER_DATE=2026-09-28T10:00:00-05:00",
	}, "commit", "-m", "Commit on day 1")

	ctx := context.Background()
	w, err := NewDayWindow("2026-09-27", loc, now)
	if err != nil {
		t.Fatalf("NewDayWindow: %v", err)
	}
	meta, err := ScanRepositoryDay(ctx, dir, w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta != nil {
		t.Fatalf("expected nil result for day with no commits, got %+v", meta)
	}
}

func TestScanRepositoryDay_FirstCommitEmptyTree(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*3600)
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, loc)
	dir, runGit := setupIsolatedTestRepo(t)

	if err := os.WriteFile(filepath.Join(dir, "initial.txt"), []byte("Initial file created\n"), 0o644); err != nil {
		t.Fatalf("writing initial.txt: %v", err)
	}
	runGit(nil, "add", "initial.txt")
	runGit([]string{
		"GIT_AUTHOR_DATE=2026-09-28T10:00:00-05:00",
		"GIT_COMMITTER_DATE=2026-09-28T10:00:00-05:00",
	}, "commit", "-m", "Initial commit")

	ctx := context.Background()
	w, err := NewDayWindow("2026-09-28", loc, now)
	if err != nil {
		t.Fatalf("NewDayWindow: %v", err)
	}
	meta, err := ScanRepositoryDay(ctx, dir, w)
	if err != nil {
		t.Fatalf("ScanRepositoryDay: %v", err)
	}
	if meta == nil {
		t.Fatal("expected non-nil meta")
	}
	if meta.CommitsCount != 1 {
		t.Errorf("expected CommitsCount = 1, got %d", meta.CommitsCount)
	}
	if !strings.Contains(meta.UnifiedDiff, "initial.txt") || !strings.Contains(meta.UnifiedDiff, "Initial file created") {
		t.Errorf("diff does not show initial file addition: %s", meta.UnifiedDiff)
	}
}

func TestScanRepositoryDay_Boundaries(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*3600)
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, loc)
	dir, runGit := setupIsolatedTestRepo(t)

	// Commit A at 23:59:59 local
	if err := os.WriteFile(filepath.Join(dir, "file_a.txt"), []byte("content a\n"), 0o644); err != nil {
		t.Fatalf("writing file_a: %v", err)
	}
	runGit(nil, "add", "file_a.txt")
	runGit([]string{
		"GIT_AUTHOR_DATE=2026-09-28T23:59:59-05:00",
		"GIT_COMMITTER_DATE=2026-09-28T23:59:59-05:00",
	}, "commit", "-m", "Commit at 23:59:59 on day 1")

	// Commit B at 00:00:00 local (new day)
	if err := os.WriteFile(filepath.Join(dir, "file_b.txt"), []byte("content b\n"), 0o644); err != nil {
		t.Fatalf("writing file_b: %v", err)
	}
	runGit(nil, "add", "file_b.txt")
	runGit([]string{
		"GIT_AUTHOR_DATE=2026-09-29T00:00:00-05:00",
		"GIT_COMMITTER_DATE=2026-09-29T00:00:00-05:00",
	}, "commit", "-m", "Commit at 00:00:00 on day 2")

	ctx := context.Background()

	// Day 1: 2026-09-28 must contain only Commit A
	w1, err := NewDayWindow("2026-09-28", loc, now)
	if err != nil {
		t.Fatalf("NewDayWindow 09-28: %v", err)
	}
	meta1, err := ScanRepositoryDay(ctx, dir, w1)
	if err != nil {
		t.Fatalf("ScanRepositoryDay 09-28: %v", err)
	}
	if meta1 == nil || meta1.CommitsCount != 1 {
		t.Fatalf("expected 1 commit for day 1, got %+v", meta1)
	}
	if !strings.Contains(meta1.Commits[0], "23:59:59") {
		t.Errorf("expected commit at 23:59:59, got %q", meta1.Commits[0])
	}

	// Day 2: 2026-09-29 must contain only Commit B
	w2, err := NewDayWindow("2026-09-29", loc, now)
	if err != nil {
		t.Fatalf("NewDayWindow 09-29: %v", err)
	}
	meta2, err := ScanRepositoryDay(ctx, dir, w2)
	if err != nil {
		t.Fatalf("ScanRepositoryDay 09-29: %v", err)
	}
	if meta2 == nil || meta2.CommitsCount != 1 {
		t.Fatalf("expected 1 commit for day 2, got %+v", meta2)
	}
	if !strings.Contains(meta2.Commits[0], "00:00:00") {
		t.Errorf("expected commit at 00:00:00, got %q", meta2.Commits[0])
	}
}

func TestScanRepositoryDay_LocationHonored(t *testing.T) {
	dir, runGit := setupIsolatedTestRepo(t)

	// Commit at 2026-09-28 12:00:00 -05:00 (which is 17:00:00 UTC, and 2026-09-29 02:00:00 +09:00 in Tokyo)
	if err := os.WriteFile(filepath.Join(dir, "tokyo.txt"), []byte("tokyo test\n"), 0o644); err != nil {
		t.Fatalf("writing tokyo.txt: %v", err)
	}
	runGit(nil, "add", "tokyo.txt")
	runGit([]string{
		"GIT_AUTHOR_DATE=2026-09-28T12:00:00-05:00",
		"GIT_COMMITTER_DATE=2026-09-28T12:00:00-05:00",
	}, "commit", "-m", "Timezone sensitive commit")

	locUTC5 := time.FixedZone("UTC-5", -5*3600)
	locTokyo := time.FixedZone("UTC+9", 9*3600)
	nowUTC5 := time.Date(2026, 10, 1, 12, 0, 0, 0, locUTC5)
	nowTokyo := time.Date(2026, 10, 1, 12, 0, 0, 0, locTokyo)
	ctx := context.Background()

	// In UTC-5, the commit lands on 2026-09-28
	wUTC5Day1, err := NewDayWindow("2026-09-28", locUTC5, nowUTC5)
	if err != nil {
		t.Fatalf("NewDayWindow UTC-5 09-28: %v", err)
	}
	metaUTC5Day1, err := ScanRepositoryDay(ctx, dir, wUTC5Day1)
	if err != nil {
		t.Fatalf("ScanRepositoryDay UTC-5 09-28: %v", err)
	}
	if metaUTC5Day1 == nil || metaUTC5Day1.CommitsCount != 1 {
		t.Errorf("expected 1 commit in UTC-5 on 2026-09-28, got %+v", metaUTC5Day1)
	}

	wUTC5Day2, err := NewDayWindow("2026-09-29", locUTC5, nowUTC5)
	if err != nil {
		t.Fatalf("NewDayWindow UTC-5 09-29: %v", err)
	}
	metaUTC5Day2, err := ScanRepositoryDay(ctx, dir, wUTC5Day2)
	if err != nil {
		t.Fatalf("ScanRepositoryDay UTC-5 09-29: %v", err)
	}
	if metaUTC5Day2 != nil {
		t.Errorf("expected nil result in UTC-5 on 2026-09-29, got %+v", metaUTC5Day2)
	}

	// In Tokyo (UTC+9), the commit lands on 2026-09-29
	wTokyoDay1, err := NewDayWindow("2026-09-28", locTokyo, nowTokyo)
	if err != nil {
		t.Fatalf("NewDayWindow Tokyo 09-28: %v", err)
	}
	metaTokyoDay1, err := ScanRepositoryDay(ctx, dir, wTokyoDay1)
	if err != nil {
		t.Fatalf("ScanRepositoryDay Tokyo 09-28: %v", err)
	}
	if metaTokyoDay1 != nil {
		t.Errorf("expected nil result in Tokyo on 2026-09-28, got %+v", metaTokyoDay1)
	}

	wTokyoDay2, err := NewDayWindow("2026-09-29", locTokyo, nowTokyo)
	if err != nil {
		t.Fatalf("NewDayWindow Tokyo 09-29: %v", err)
	}
	metaTokyoDay2, err := ScanRepositoryDay(ctx, dir, wTokyoDay2)
	if err != nil {
		t.Fatalf("ScanRepositoryDay Tokyo 09-29: %v", err)
	}
	if metaTokyoDay2 == nil || metaTokyoDay2.CommitsCount != 1 {
		t.Errorf("expected 1 commit in Tokyo on 2026-09-29, got %+v", metaTokyoDay2)
	}
}

func TestScanRepositoryDay_CloneNoReflog(t *testing.T) {
	loc := time.FixedZone("UTC-5", -5*3600)
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, loc)
	dir, runGit := setupIsolatedTestRepo(t)

	if err := os.WriteFile(filepath.Join(dir, "cloned.txt"), []byte("cloned content\n"), 0o644); err != nil {
		t.Fatalf("writing cloned.txt: %v", err)
	}
	runGit(nil, "add", "cloned.txt")
	runGit([]string{
		"GIT_AUTHOR_DATE=2026-09-28T10:00:00-05:00",
		"GIT_COMMITTER_DATE=2026-09-28T10:00:00-05:00",
	}, "commit", "-m", "Commit before clone")

	cloneParent := t.TempDir()
	cloneDir := filepath.Join(cloneParent, "clone")
	emptyConfig := filepath.Join(cloneParent, ".emptyconfig")
	if err := os.WriteFile(emptyConfig, []byte(""), 0o600); err != nil {
		t.Fatalf("writing empty config: %v", err)
	}

	cloneCmd := exec.CommandContext(context.Background(), "git", "clone", dir, cloneDir)
	cloneCmd.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+emptyConfig,
	)
	out, err := cloneCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git clone failed: %v\nOutput: %s", err, string(out))
	}

	ctx := context.Background()
	w, err := NewDayWindow("2026-09-28", loc, now)
	if err != nil {
		t.Fatalf("NewDayWindow: %v", err)
	}
	meta, err := ScanRepositoryDay(ctx, cloneDir, w)
	if err != nil {
		t.Fatalf("ScanRepositoryDay on clone: %v", err)
	}
	if meta == nil || meta.CommitsCount != 1 {
		t.Fatalf("expected 1 commit on clone, got %+v", meta)
	}
	if !strings.Contains(meta.UnifiedDiff, "cloned content") {
		t.Errorf("clone diff missing content: %s", meta.UnifiedDiff)
	}
}
