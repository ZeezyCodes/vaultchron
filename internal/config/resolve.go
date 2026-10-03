package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ResolveOptions controls config file discovery.
type ResolveOptions struct {
	Flag         string              // -config value, "" if not set
	AllowExample bool                // allow ./config.example.yaml
	Dir          string              // base dir for config.yaml / config.example.yaml; "" means "."
	GOOS         string              // "" means runtime.GOOS
	Getenv       func(string) string // nil means os.Getenv
	HomeDir      string              // "" means os.UserHomeDir() (ignore its error)
}

// Resolved represents the result of resolving a config file.
type Resolved struct {
	Path     string
	Source   string // flag|env|cwd|user|example
	Searched []string
}

// isRegularFile returns true if the path exists and is a regular file (not a directory).
func isRegularFile(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fi.Mode().IsRegular()
}

// Resolve locates the configuration file following the search order:
// 1. Explicit flag (-config)
// 2. Environment variable ($VAULTCHRON_CONFIG)
// 3. Working directory (<Dir>/config.yaml)
// 4. Per-user configuration location
// 5. Example configuration (<Dir>/config.example.yaml) if AllowExample is true
//
// Returns a non-nil error if no config file could be found or if explicit flag/env paths do not exist.
func Resolve(o ResolveOptions) (Resolved, error) {
	goos := o.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}

	getenv := o.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}

	homeDir := o.HomeDir
	if homeDir == "" {
		homeDir, _ = os.UserHomeDir()
	}

	var searched []string

	// 1. Flag non-empty
	if o.Flag != "" {
		searched = append(searched, o.Flag)
		if isRegularFile(o.Flag) {
			return Resolved{
				Path:     o.Flag,
				Source:   "flag",
				Searched: searched,
			}, nil
		}
		return Resolved{Searched: searched}, fmt.Errorf("config file %s (from -config) not found", o.Flag)
	}

	// 2. VAULTCHRON_CONFIG non-empty
	if envVal := getenv("VAULTCHRON_CONFIG"); envVal != "" {
		searched = append(searched, envVal)
		if isRegularFile(envVal) {
			return Resolved{
				Path:     envVal,
				Source:   "env",
				Searched: searched,
			}, nil
		}
		return Resolved{Searched: searched}, fmt.Errorf("config file %s (from $VAULTCHRON_CONFIG) not found", envVal)
	}

	// 3. <Dir>/config.yaml
	cwdConfig := filepath.Join(o.Dir, "config.yaml")
	searched = append(searched, cwdConfig)
	if isRegularFile(cwdConfig) {
		return Resolved{
			Path:     cwdConfig,
			Source:   "cwd",
			Searched: searched,
		}, nil
	}

	// 4. Per-user file
	var userConfigPath string
	if goos == "windows" {
		appdata := getenv("APPDATA")
		if appdata != "" {
			userConfigPath = filepath.Join(appdata, "vaultchron", "config.yaml")
		}
	} else {
		xdg := getenv("XDG_CONFIG_HOME")
		if xdg != "" && filepath.IsAbs(xdg) {
			userConfigPath = filepath.Join(xdg, "vaultchron", "config.yaml")
		} else if homeDir != "" {
			userConfigPath = filepath.Join(homeDir, ".config", "vaultchron", "config.yaml")
		}
	}

	if userConfigPath != "" {
		searched = append(searched, userConfigPath)
		if isRegularFile(userConfigPath) {
			return Resolved{
				Path:     userConfigPath,
				Source:   "user",
				Searched: searched,
			}, nil
		}
	}

	// 5. Only if AllowExample: <Dir>/config.example.yaml
	exampleConfig := filepath.Join(o.Dir, "config.example.yaml")
	if o.AllowExample {
		searched = append(searched, exampleConfig)
		if isRegularFile(exampleConfig) {
			return Resolved{
				Path:     exampleConfig,
				Source:   "example",
				Searched: searched,
			}, nil
		}
	}

	// 6. Otherwise a non-nil error whose text is multi-line
	var lines []string
	lines = append(lines, "no config file found; looked in this order:")
	lines = append(lines, "1. -config (not set)")
	lines = append(lines, "2. $VAULTCHRON_CONFIG (not set)")
	lines = append(lines, "3. "+cwdConfig)
	if userConfigPath != "" {
		lines = append(lines, "4. "+userConfigPath)
	} else {
		lines = append(lines, "4. (unavailable: no home directory or APPDATA)")
	}
	if o.AllowExample {
		lines = append(lines, "5. "+exampleConfig)
	}

	targetHint := userConfigPath
	if targetHint == "" {
		targetHint = "./config.yaml"
	}
	lines = append(lines, fmt.Sprintf("create one by copying config.example.yaml (https://raw.githubusercontent.com/ZeezyCodes/vaultchron/main/config.example.yaml) to %s, then set vault.path and scan.roots", targetHint))

	return Resolved{Searched: searched}, fmt.Errorf("%s", strings.Join(lines, "\n"))
}
