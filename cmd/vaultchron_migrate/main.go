package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ZeezyCodes/vaultchron/internal/config"
	"github.com/ZeezyCodes/vaultchron/internal/vault"
)

// resolveVaultPath resolves the Obsidian vault root path using vaultFlag,
// then config file, and finally fallback to DefaultConfig.
func resolveVaultPath(vaultFlag, configPath string) string {
	if vaultFlag != "" {
		return vaultFlag
	}
	cfgPath := config.ResolveConfigPath(configPath)
	if cfg, err := config.Load(cfgPath); err == nil && cfg.Vault.Path != "" {
		return cfg.Vault.Path
	}
	return config.DefaultConfig().Vault.Path
}

// runMigrate executes migration or preview logic based on options, writing human-readable
// status to out and error diagnostics to errOut. Returns exit code (0 on success, 1 on error).
func runMigrate(vaultFlag, configPath, fileFlag string, dryRun bool, out, errOut io.Writer) int {
	if fileFlag != "" {
		if dryRun {
			raw, err := os.ReadFile(fileFlag)
			if err != nil {
				fmt.Fprintf(errOut, "error reading file %s: %v\n", fileFlag, err)
				return 1
			}
			migrated, err := vault.MigrateContent(string(raw), "", "")
			if err != nil {
				fmt.Fprintf(errOut, "error transforming %s: %v\n", fileFlag, err)
				return 1
			}
			fmt.Fprintf(out, "--- Dry Run Preview for %s ---\n%s\n", fileFlag, migrated)
			return 0
		}

		changed, err := vault.MigrateFile(fileFlag)
		if err != nil {
			fmt.Fprintf(errOut, "error migrating file %s: %v\n", fileFlag, err)
			return 1
		}
		if changed {
			fmt.Fprintf(out, "Migrated: %s\n", fileFlag)
		} else {
			fmt.Fprintf(out, "Already up to date: %s\n", fileFlag)
		}
		return 0
	}

	vaultPath := resolveVaultPath(vaultFlag, configPath)
	pattern := filepath.Join(vaultPath, "Projects", "*", "Devlog", "*.md")
	files, err := filepath.Glob(pattern)
	if err != nil {
		fmt.Fprintf(errOut, "error searching for devlog files in %s: %v\n", vaultPath, err)
		return 1
	}

	fmt.Fprintf(out, "Found %d devlog notes in %s\n", len(files), vaultPath)

	if dryRun {
		fmt.Fprintln(out, "Dry-run enabled. Checking which files require migration:")
		count := 0
		for _, f := range files {
			raw, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			migrated, err := vault.MigrateContent(string(raw), "", "")
			if err == nil && migrated != string(raw) {
				fmt.Fprintf(out, "  [PENDING] %s\n", f)
				count++
			}
		}
		fmt.Fprintf(out, "\n%d/%d files would be modified.\n", count, len(files))
		return 0
	}

	modified, err := vault.MigrateVault(vaultPath)
	if err != nil {
		fmt.Fprintf(errOut, "migration failed: %v\n", err)
		return 1
	}

	fmt.Fprintf(out, "Successfully migrated %d devlog notes to v3 callouts in %s\n", len(modified), vaultPath)
	return 0
}

func main() {
	vaultFlag := flag.String("vault", "", "path to the Obsidian vault root (defaults to config.yaml vault.path or default vault path)")
	configPath := flag.String("config", "", "path to config.yaml")
	fileFlag := flag.String("file", "", "migrate a single devlog markdown file instead of the whole vault")
	dryRun := flag.Bool("dry-run", false, "display what would be migrated without modifying files")
	flag.Parse()

	os.Exit(runMigrate(*vaultFlag, *configPath, *fileFlag, *dryRun, os.Stdout, os.Stderr))
}
