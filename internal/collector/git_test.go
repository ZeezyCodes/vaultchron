package collector

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// TestParseCommitLines verifies that git log --oneline output is correctly
// parsed into a slice of non-empty, trimmed commit message lines.
func TestParseCommitLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty input",
			input:    "",
			expected: nil,
		},
		{
			name:     "single commit",
			input:    "abc1234 Initial commit",
			expected: []string{"abc1234 Initial commit"},
		},
		{
			name:     "multiple commits",
			input:    "abc1234 Initial commit\ndef5678 Fix bug\nghi9abc Add feature",
			expected: []string{"abc1234 Initial commit", "def5678 Fix bug", "ghi9abc Add feature"},
		},
		{
			name:     "whitespace handling",
			input:    "  abc1234 Initial commit  \n  \n  def5678 Fix bug  ",
			expected: []string{"abc1234 Initial commit", "def5678 Fix bug"},
		},
		{
			name:     "trailing newline",
			input:    "abc1234 Initial commit\n",
			expected: []string{"abc1234 Initial commit"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseCommitLines(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d commits, got %d", len(tt.expected), len(result))
				return
			}
			for i, c := range result {
				if c != tt.expected[i] {
					t.Errorf("commit %d: expected %q, got %q", i, tt.expected[i], c)
				}
			}
		})
	}
}

// TestRankPackages verifies that git diff --name-only output is correctly
// aggregated into directory/package prefixes and ranked by modification count.
func TestRankPackages(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty input",
			input:    "",
			expected: nil,
		},
		{
			name:     "single file",
			input:    "internal/pkg/file.go",
			expected: []string{"internal/pkg"},
		},
		{
			name:     "multiple files same package",
			input:    "internal/pkg/file1.go\ninternal/pkg/file2.go\ninternal/pkg/file3.go",
			expected: []string{"internal/pkg"},
		},
		{
			name:     "multiple packages ranked by count",
			input:    "internal/pkg/a.go\ninternal/pkg/b.go\ninternal/pkg/c.go\ncmd/app/main.go\ninternal/other/c.go\ninternal/other/d.go",
			expected: []string{"internal/pkg", "internal/other", "cmd/app"},
		},
		{
			name:     "top 4 limit",
			input:    "pkg1/sub/a.go\npkg1/sub/b.go\npkg1/sub/c.go\npkg1/sub/d.go\npkg2/sub/e.go\npkg2/sub/f.go\npkg2/sub/g.go\npkg3/sub/h.go\npkg3/sub/i.go\npkg4/sub/j.go",
			expected: []string{"pkg1/sub", "pkg2/sub", "pkg3/sub", "pkg4/sub"},
		},
		{
			name:     "whitespace handling",
			input:    "  internal/pkg/a.go  \n  internal/pkg/b.go  \n  \n  cmd/app/main.go  ",
			expected: []string{"internal/pkg", "cmd/app"},
		},
		{
			name:     "windows backslash paths",
			input:    "internal\\pkg\\a.go\ninternal\\pkg\\b.go\ncmd\\app\\main.go",
			expected: []string{"internal/pkg", "cmd/app"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rankPackages(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d packages, got %d: %v", len(tt.expected), len(result), result)
				return
			}
			for i, p := range result {
				if p != tt.expected[i] {
					t.Errorf("package %d: expected %q, got %q", i, tt.expected[i], p)
				}
			}
		})
	}
}

// TestTruncateDiff verifies that diff content is capped at 25,000 characters
// with head + tail truncation and a truncation marker.
func TestTruncateDiff(t *testing.T) {
	t.Run("under limit", func(t *testing.T) {
		diff := "short diff"
		result := truncateDiff(diff)
		if result != diff {
			t.Errorf("expected %q, got %q", diff, result)
		}
	})

	t.Run("at limit", func(t *testing.T) {
		diff := strings.Repeat("a", maxDiffChars)
		result := truncateDiff(diff)
		if result != diff {
			t.Error("expected unchanged diff at limit")
		}
	})

	t.Run("over limit", func(t *testing.T) {
		diff := strings.Repeat("a", maxDiffChars+1000)
		result := truncateDiff(diff)
		if !strings.Contains(result, "[TRUNCATED") {
			t.Error("expected truncated diff to contain [TRUNCATED marker")
		}
		// Verify head and tail are preserved.
		half := maxDiffChars / 2
		if !strings.HasPrefix(result, diff[:half]) {
			t.Error("expected truncated diff to preserve head")
		}
		if !strings.HasSuffix(result, diff[len(diff)-half:]) {
			t.Error("expected truncated diff to preserve tail")
		}
	})

	t.Run("MaxDiffChars returns 25000", func(t *testing.T) {
		if MaxDiffChars() != 25000 {
			t.Errorf("expected MaxDiffChars() to return 25000, got %d", MaxDiffChars())
		}
	})
}

// TestRedactSecrets verifies that secret tokens, keys, credentials, and private keys
// in git diffs are redacted to [REDACTED], while ordinary identifiers without assignments
// are preserved.
func TestRedactSecrets(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty diff",
			input:    "",
			expected: "",
		},
		{
			name:     "AWS access key standalone",
			input:    "+AKIAIOSFODNN7EXAMPLE",
			expected: "+[REDACTED]",
		},
		{
			name:     "AWS access key in assignment",
			input:    "+AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE",
			expected: "+AWS_ACCESS_KEY_ID=[REDACTED]",
		},
		{
			name:     "generic api_key double-quoted",
			input:    `+api_key = "abcdef123456"`,
			expected: `+api_key = "[REDACTED]"`,
		},
		{
			name:     "generic apikey colon unquoted",
			input:    `+apikey: secret_key_val_999`,
			expected: `+apikey: [REDACTED]`,
		},
		{
			name:     "generic api-key hyphenated single-quoted",
			input:    `+api-key: 'secret-val-here'`,
			expected: `+api-key: '[REDACTED]'`,
		},
		{
			name:     "secret key in JSON",
			input:    `+  "client_secret": "my-client-secret-123",`,
			expected: `+  "client_secret": "[REDACTED]",`,
		},
		{
			name:     "secret in .env uppercase",
			input:    `+SECRET=top-secret-passphrase`,
			expected: `+SECRET=[REDACTED]`,
		},
		{
			name:     "token walrus assignment in Go",
			input:    `+token := "ghp_1234567890abcdef"`,
			expected: `+token := "[REDACTED]"`,
		},
		{
			name:     "auth_token unquoted YAML",
			input:    `+auth_token: ghp_9876543210zyxwvu`,
			expected: `+auth_token: [REDACTED]`,
		},
		{
			name:     "password assignment double-quoted",
			input:    `+password = "hunter2"`,
			expected: `+password = "[REDACTED]"`,
		},
		{
			name:     "db_password in .env style",
			input:    `+DB_PASSWORD=supersecretpassword123`,
			expected: `+DB_PASSWORD=[REDACTED]`,
		},
		{
			name:     "Authorization: Bearer header unquoted",
			input:    `+Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9`,
			expected: `+Authorization: Bearer [REDACTED]`,
		},
		{
			name:     "Authorization: Bearer header quoted",
			input:    `+"Authorization": "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"`,
			expected: `+"Authorization": "Bearer [REDACTED]"`,
		},
		{
			name: "PEM RSA private key block",
			input: `+-----BEGIN RSA PRIVATE KEY-----
+MIIEowIBAAKCAQEA0Yxyz...
+-----END RSA PRIVATE KEY-----`,
			expected: `+[REDACTED]`,
		},
		{
			name: "PEM OpenSSH private key block",
			input: `+-----BEGIN OPENSSH PRIVATE KEY-----
+b3BlbnNzaC1rZXktdjEAAAA...
+-----END OPENSSH PRIVATE KEY-----`,
			expected: `+[REDACTED]`,
		},
		{
			name:     ".env style KEY=value assignment with custom prefix",
			input:    `+STRIPE_API_KEY=sk_test_51Mzxyz123`,
			expected: `+STRIPE_API_KEY=[REDACTED]`,
		},
		{
			name:     "false-positive: token as function parameter and return",
			input:    `+func validateToken(token string) bool { return token != "" }`,
			expected: `+func validateToken(token string) bool { return token != "" }`,
		},
		{
			name:     "false-positive: token in equality check",
			input:    `+if token == "expected-value" {`,
			expected: `+if token == "expected-value" {`,
		},
		{
			name:     "false-positive: token in inequality check",
			input:    `+if token != "invalid" {`,
			expected: `+if token != "invalid" {`,
		},
		{
			name:     "false-positive: token := functionCall()",
			input:    `+token := generateToken(ctx)`,
			expected: `+token := generateToken(ctx)`,
		},
		{
			name:     "false-positive: token: null YAML",
			input:    `+token: null`,
			expected: `+token: null`,
		},
		{
			name:     "false-positive: var token string declaration",
			input:    `+var token string`,
			expected: `+var token string`,
		},
		{
			name:     "already redacted value remains redacted without duplication",
			input:    `+api_key = "[REDACTED]"`,
			expected: `+api_key = "[REDACTED]"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redactSecrets(tt.input)
			if got != tt.expected {
				t.Errorf("\nInput:    %q\nExpected: %q\nGot:      %q", tt.input, tt.expected, got)
			}
		})
	}
}

// TestSafeTruncate verifies that truncation backs off to valid rune boundaries,
// avoids RuneError / replacement characters, and maintains valid UTF-8 strings.
func TestSafeTruncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxBytes int
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			maxBytes: 10,
			expected: "",
		},
		{
			name:     "maxBytes <= 0",
			input:    "hello",
			maxBytes: 0,
			expected: "",
		},
		{
			name:     "exact boundary at ascii",
			input:    "Hello world",
			maxBytes: 5,
			expected: "Hello",
		},
		{
			name:     "longer than string",
			input:    "Hello",
			maxBytes: 50,
			expected: "Hello",
		},
		{
			name:     "truncate in middle of 4-byte emoji",
			input:    "Hello 🚀 world", // "Hello " is 6 bytes, 🚀 is 4 bytes [6..10)
			maxBytes: 8,               // cuts into 🚀
			expected: "Hello ",        // should back off to before 🚀
		},
		{
			name:     "exact boundary after 4-byte emoji",
			input:    "Hello 🚀 world",
			maxBytes: 10,
			expected: "Hello 🚀",
		},
		{
			name:     "truncate in middle of 3-byte CJK character",
			input:    "世界你好", // each is 3 bytes
			maxBytes: 5,      // 5 bytes cuts into 界 (bytes 3..6)
			expected: "世",    // 3 bytes
		},
		{
			name:     "all multibyte truncated at 1 byte",
			input:    "🚀",
			maxBytes: 2,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := safeTruncate(tt.input, tt.maxBytes)
			if got != tt.expected {
				t.Errorf("safeTruncate(%q, %d) = %q; want %q", tt.input, tt.maxBytes, got, tt.expected)
			}
			if !utf8.ValidString(got) {
				t.Errorf("safeTruncate returned invalid UTF-8: %q", got)
			}
			if strings.ContainsRune(got, utf8.RuneError) {
				t.Errorf("safeTruncate returned RuneError: %q", got)
			}
		})
	}
}

// TestTruncateDiff_UTF8 verifies that truncateDiff never slices across UTF-8 rune boundaries.
func TestTruncateDiff_UTF8(t *testing.T) {
	var sb strings.Builder
	for sb.Len() < MaxDiffChars()+500 {
		sb.WriteString("diff --git a/file b/file\n+🚀 世界你好 test line ")
		sb.WriteString(strings.Repeat("x", 50))
		sb.WriteString("\n")
	}
	longDiff := sb.String()

	truncated := truncateDiff(longDiff)
	if !utf8.ValidString(truncated) {
		t.Errorf("truncateDiff produced invalid UTF-8")
	}
	if strings.ContainsRune(truncated, utf8.RuneError) {
		t.Errorf("truncateDiff produced RuneError in output")
	}
	if !strings.Contains(truncated, "[TRUNCATED") {
		t.Errorf("expected truncation notice in output")
	}
}

// TestHarvestRepoMetadata_FreshSingleCommitRepo verifies that HarvestRepoMetadata
// produces a valid diff and metadata on a fresh single-commit repository without reflog entries.
func TestHarvestRepoMetadata_FreshSingleCommitRepo(t *testing.T) {
	tempDir := t.TempDir()

	runGit := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = tempDir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test User",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test User",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v\nOutput: %s", strings.Join(args, " "), err, string(out))
		}
	}

	runGit("init")
	runGit("config", "user.name", "Test User")
	runGit("config", "user.email", "test@example.com")

	filePath := filepath.Join(tempDir, "README.md")
	if err := os.WriteFile(filePath, []byte("# Test Repo\nHello world.\n"), 0o644); err != nil {
		t.Fatalf("failed writing test file: %v", err)
	}
	runGit("add", "README.md")
	runGit("commit", "-m", "Initial commit")

	ctx := context.Background()
	meta, err := HarvestRepoMetadata(ctx, tempDir, "24.hours.ago")
	if err != nil {
		t.Fatalf("HarvestRepoMetadata failed on fresh repo: %v", err)
	}
	if meta == nil {
		t.Fatal("expected non-nil RepoMetadata")
	}
	if meta.CommitsCount != 1 {
		t.Errorf("expected CommitsCount 1, got %d", meta.CommitsCount)
	}
	if !strings.Contains(meta.UnifiedDiff, "README.md") && !strings.Contains(meta.UnifiedDiff, "Hello world") {
		t.Errorf("expected diff to contain README content, got: %q", meta.UnifiedDiff)
	}
	if meta.Branch == "" {
		t.Errorf("expected non-empty branch name, got %q", meta.Branch)
	}
}
