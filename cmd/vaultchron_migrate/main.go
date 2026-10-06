package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/ZeezyCodes/vaultchron/internal/config"
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

// resolveVaultConfig resolves the Obsidian vault configuration using vaultFlag,
// then config file, and finally fallback to DefaultConfig.
// If vaultFlag or fileFlag is supplied without a loadable config, DefaultConfig settings are used.
func resolveVaultConfig(vaultFlag, configPath, fileFlag string, dryRun bool, errOut io.Writer) (config.VaultConfig, error) {
	vCfg := config.DefaultConfig().Vault
	resolved, err := config.Resolve(config.ResolveOptions{
		Flag:         configPath,
		AllowExample: dryRun,
		Getenv:       os.Getenv,
	})
	if err != nil {
		if vaultFlag != "" || fileFlag != "" {
			if vaultFlag != "" {
				vCfg.Path = vaultFlag
			}
			if configPath != "" {
				fmt.Fprintf(errOut, "[WARN] config not applied to index_file/projects_dir, using defaults: %v\n", err)
			}
			return vCfg, nil
		}
		return vCfg, err
	}
	if resolved.Source == "example" {
		fmt.Fprintln(errOut, "[INFO] config.yaml not found, falling back to config.example.yaml")
	} else if resolved.Source == "env" || resolved.Source == "user" {
		fmt.Fprintf(errOut, "[INFO] using config %s\n", resolved.Path)
	}
	cfg, err := config.Load(resolved.Path)
	if err != nil {
		if vaultFlag != "" || fileFlag != "" {
			if vaultFlag != "" {
				vCfg.Path = vaultFlag
			}
			fmt.Fprintf(errOut, "[WARN] config not applied to index_file/projects_dir, using defaults: %v\n", err)
			return vCfg, nil
		}
		return vCfg, err
	}
	vCfg = cfg.Vault
	if vaultFlag != "" {
		vCfg.Path = vaultFlag
	} else if vCfg.Path == "" {
		vCfg.Path = config.DefaultConfig().Vault.Path
	}
	return vCfg, nil
}

// resolveVaultPath resolves the Obsidian vault root path using vaultFlag,
// then config file, and finally fallback to DefaultConfig.
func resolveVaultPath(vaultFlag, configPath string, dryRun bool, errOut io.Writer) (string, error) {
	vCfg, err := resolveVaultConfig(vaultFlag, configPath, "", dryRun, errOut)
	return vCfg.Path, err
}

// runMigrate executes migration or preview logic based on options, writing human-readable
// status to out and error diagnostics to errOut. Returns exit code (0 on success, 1 on error).
func runMigrate(vaultFlag, configPath, fileFlag string, dryRun bool, out, errOut io.Writer) int {
	vaultCfg, err := resolveVaultConfig(vaultFlag, configPath, fileFlag, dryRun, errOut)
	if err != nil {
		fmt.Fprintf(errOut, "error: %v\n", err)
		return 1
	}

	if fileFlag != "" {
		if dryRun {
			raw, err := os.ReadFile(fileFlag)
			if err != nil {
				fmt.Fprintf(errOut, "error reading file %s: %v\n", fileFlag, err)
				return 1
			}
			migrated, err := vault.MigrateContent(string(raw), "", "", vaultCfg)
			if err != nil {
				fmt.Fprintf(errOut, "error transforming %s: %v\n", fileFlag, err)
				return 1
			}
			fmt.Fprintf(out, "--- Dry Run Preview for %s ---\n%s\n", fileFlag, migrated)
			return 0
		}

		changed, err := vault.MigrateFile(fileFlag, vaultCfg)
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

	vaultPath := vaultCfg.Path
	pattern := filepath.Join(vaultPath, filepath.FromSlash(vaultCfg.ProjectsDir), "*", "Devlog", "*.md")
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
			migrated, err := vault.MigrateContent(string(raw), "", "", vaultCfg)
			if err == nil && migrated != string(raw) {
				fmt.Fprintf(out, "  [PENDING] %s\n", f)
				count++
			}
		}
		fmt.Fprintf(out, "\n%d/%d files would be modified.\n", count, len(files))
		return 0
	}

	modified, err := vault.MigrateVault(vaultPath, vaultCfg)
	if err != nil {
		fmt.Fprintf(errOut, "migration failed: %v\n", err)
		return 1
	}

	fmt.Fprintf(out, "Successfully migrated %d devlog notes to v3 callouts in %s\n", len(modified), vaultPath)
	return 0
}

func main() {
	if hasVersionFlag(os.Args[1:]) {
		fmt.Println("vaultchron_migrate " + getVersion())
		os.Exit(0)
	}

	vaultFlag := flag.String("vault", "", "path to the Obsidian vault root (defaults to config.yaml vault.path or default vault path)")
	configPath := flag.String("config", "", "path to config.yaml (order: -config, $VAULTCHRON_CONFIG, ./config.yaml, per-user config, ./config.example.yaml for dry-run)")
	fileFlag := flag.String("file", "", "migrate a single devlog markdown file instead of the whole vault")
	dryRun := flag.Bool("dry-run", false, "display what would be migrated without modifying files")
	versionFlag := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Println("vaultchron_migrate " + getVersion())
		os.Exit(0)
	}

	os.Exit(runMigrate(*vaultFlag, *configPath, *fileFlag, *dryRun, os.Stdout, os.Stderr))
}
