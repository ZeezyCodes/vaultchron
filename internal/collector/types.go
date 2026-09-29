package collector

// RepoMetadata holds all telemetry and content data collected from a single
// git repository for devlog generation.
type RepoMetadata struct {
	// Name is the vault project directory name (e.g. "AcmeWidgets.com").
	Name string
	// Path is the absolute filesystem path to the git repository root.
	Path string
	// Branch is the active git branch name (e.g. "main").
	Branch string
	// Commits is the list of oneline commit messages from the collection window.
	Commits []string
	// CommitsCount is the number of commits in Commits.
	CommitsCount int
	// Shortstat is the git diff --shortstat line (e.g. "8 files changed, 780 insertions(+)").
	Shortstat string
	// TopPackages is the 3-4 most-modified directory/package paths from git diff --name-only.
	TopPackages []string
	// UnifiedDiff is the truncated unified diff (head + tail) of changes in the window.
	UnifiedDiff string
}

// AgentContext holds high-level agent session metadata extracted from
// configured agent log directories.
type AgentContext struct {
	// Goals extracted from session metadata (JSON "goal" fields or "Goal:" labels).
	Goals []string
	// Outlines extracted from session metadata (JSON "outcome" fields or "Outcome:" labels).
	Outlines []string
	// Prompts extracted from session metadata (JSON "user_prompt"/"task" fields or "Prompt:"/"Task:" labels).
	Prompts []string
}
