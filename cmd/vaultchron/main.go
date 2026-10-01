package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/ZeezyCodes/vaultchron/internal/collector"
	"github.com/ZeezyCodes/vaultchron/internal/config"
	"github.com/ZeezyCodes/vaultchron/internal/pipeline"
	"github.com/ZeezyCodes/vaultchron/internal/vault"
)

var version = "dev"

// resolveVersion resolves the application version string.
// If v is set and not "dev", it is returned directly.
// If v is "dev" (or empty), it falls back to the build info main version
// reported by readInfo if non-empty and not "(devel)".
// Otherwise, it returns "dev".
func resolveVersion(v string, readInfo func() (*debug.BuildInfo, bool)) string {
	if v != "" && v != "dev" {
		return v
	}
	if readInfo != nil {
		if info, ok := readInfo(); ok && info != nil {
			if info.Main.Version != "" && info.Main.Version != "(devel)" {
				return info.Main.Version
			}
		}
	}
	if v != "" {
		return v
	}
	return "dev"
}

func getVersion() string {
	return resolveVersion(version, debug.ReadBuildInfo)
}

// hasVersionFlag returns true if args contains -version or --version before any bare "--" separator.
func hasVersionFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "-version" || arg == "--version" {
			return true
		}
	}
	return false
}

// validWindowRegex enforces git time-window format defense-in-depth.
var validWindowRegex = regexp.MustCompile(`^[0-9]+\.(minute|minutes|hour|hours|day|days|week|weeks|month|months)\.ago$`)

// lockFilePerms is the restricted permission set for the per-user lock file.
const lockFilePerms = 0o600

func main() {
	if hasVersionFlag(os.Args[1:]) {
		fmt.Println("vaultchron " + getVersion())
		os.Exit(0)
	}

	lockPath := defaultLockPath()
	// Acquire advisory lock to prevent overlapping runs via cron or timers.
	lockFile, acquired, err := acquireLock(lockPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error acquiring lock: %v\n", err)
		os.Exit(1)
	}
	if !acquired {
		fmt.Println("[INFO] Another instance of vaultchron is currently running. Exiting.")
		os.Exit(0)
	}

	// Set up signal handling for clean lock release on interrupt.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		releaseLock(lockFile)
		os.Exit(0)
	}()

	code := run()
	releaseLock(lockFile)
	os.Exit(code)
}

// run contains the main application logic and returns an exit code.
// It is separated from main() so that the advisory lock can be released
// via defer before os.Exit is called.
func run() int {
	versionFlag := flag.Bool("version", false, "print version and exit")
	configPath := flag.String("config", "", "path to config.yaml (defaults to config.yaml or config.example.yaml)")
	scanMode := flag.Bool("scan", false, "run collector only: discover repos, harvest metadata, print results, and exit 0")
	window := flag.String("window", "24.hours.ago", "git time window for --since log query and HEAD@{<window>} diff reference")
	dryRun := flag.Bool("dry-run", false, "skip LLM calls and vault writes; render populated DevlogData preview to stdout")
	repoFilter := flag.String("repo", "", "target a single repository by base directory name (e.g. my-project)")
	force := flag.Bool("force", false, "process repositories even if they have zero commits in the window")
	migrateVault := flag.Bool("migrate-vault", false, "migrate legacy devlog notes in vault to v3 callout taxonomy in-place")
	migrateV3 := flag.Bool("migrate-v3", false, "alias for -migrate-vault")
	flag.Parse()

	if *versionFlag {
		fmt.Println("vaultchron " + getVersion())
		return 0
	}

	if !validWindowRegex.MatchString(*window) {
		fmt.Fprintf(os.Stderr, "invalid -window %q: expected format like <number>.(hours|days|weeks|minutes).ago (e.g. 24.hours.ago)\n", *window)
		return 1
	}

	// Resolve config path: explicit flag > config.yaml > config.example.yaml.
	cfgPath := config.ResolveConfigPath(*configPath)
	if *configPath == "" && cfgPath == "config.example.yaml" {
		fmt.Fprintln(os.Stderr, "[INFO] config.yaml not found, falling back to config.example.yaml")
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		return 1
	}

	if *migrateVault || *migrateV3 {
		vaultPath := cfg.Vault.Path
		if vaultPath == "" {
			vaultPath = config.DefaultConfig().Vault.Path
		}
		fmt.Printf("Migrating legacy devlogs in vault: %s\n", vaultPath)
		modified, err := vault.MigrateVault(vaultPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error migrating vault: %v\n", err)
			return 1
		}
		fmt.Printf("Migration complete. Successfully updated %d note(s) to v3 callouts.\n", len(modified))
		return 0
	}

	if *scanMode {
		return runScan(cfg, *window)
	}

	// Standard pipeline execution (includes dry-run mode when -dry-run is set).
	opts := pipeline.PipelineOptions{
		Window:     *window,
		RepoFilter: *repoFilter,
		Force:      *force,
		DryRun:     *dryRun,
	}
	if err := pipeline.Run(cfg, opts); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

// runScan executes the non-destructive collector scan: discovers repositories,
// harvests metadata for each, and prints a formatted terminal table.
func runScan(cfg *config.Config, window string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	repos, err := collector.DiscoverRepositories(ctx, cfg.Scan.Roots, cfg.Scan.MaxDepth, cfg.Scan.Excludes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error discovering repositories: %v\n", err)
		return 1
	}

	fmt.Printf("Scan window: %s\n", window)
	fmt.Printf("Discovered %d repositories:\n\n", len(repos))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "Repo Name\tBranch\tCommits\tChurn\tTop Packages")
	fmt.Fprintln(w, "--------- \t------\t-------\t-----\t-------------")

	activeCount := 0
	for _, repoPath := range repos {
		result, err := collector.ScanRepository(ctx, repoPath, window)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  [WARN] %s: %v\n", repoPath, err)
			continue
		}
		if result.Commits > 0 {
			activeCount++
		}

		churn := result.Shortstat
		if churn == "" {
			churn = "-"
		}
		packages := "-"
		if len(result.TopPackages) > 0 {
			packages = strings.Join(result.TopPackages, ", ")
		}

		fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\n",
			result.Name, result.Branch, result.Commits, churn, packages)
	}
	w.Flush()

	fmt.Printf("\n%d active repository(ies) with commits in window \"%s\".\n", activeCount, window)
	fmt.Println("Scan complete.")
	return 0
}

// defaultLockPath returns a per-user lock path to avoid world-writable /tmp shared namespace collisions.
func defaultLockPath() string {
	return lockPathForUID(os.TempDir(), os.Getuid())
}

// lockPathForUID computes the lock file path for a given UID and temp directory.
// Extracted as a pure function for unit testing.
func lockPathForUID(tempDir string, uid int) string {
	if uid >= 0 {
		return filepath.Join(tempDir, fmt.Sprintf("vaultchron-%d.lock", uid))
	}
	// Fallback for platforms where os.Getuid() is not supported (e.g. Windows returns -1).
	if cacheDir, err := os.UserCacheDir(); err == nil && cacheDir != "" {
		return filepath.Join(cacheDir, "vaultchron", "vaultchron.lock")
	}
	return filepath.Join(tempDir, "vaultchron.lock")
}

// acquireLock attempts to acquire an exclusive, non-blocking advisory lock
// on path using syscall.Flock.
// It returns:
//   - (*os.File, true, nil) if lock is acquired;
//   - (nil, false, nil) if another instance already holds the lock;
//   - (nil, false, err) if opening or creating the file fails (e.g. permissions or disk error).
func acquireLock(path string) (*os.File, bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, false, fmt.Errorf("creating lock directory %q: %w", filepath.Dir(path), err)
	}

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, lockFilePerms)
	if err != nil {
		return nil, false, fmt.Errorf("opening lock file %q: %w", path, err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, false, nil
	}
	return f, true, nil
}

// releaseLock releases the advisory lock and closes the file handle.
// Safe to call multiple times.
func releaseLock(f *os.File) {
	if f == nil {
		return
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	_ = f.Close()
}
