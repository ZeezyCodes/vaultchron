package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolve_TableDriven(t *testing.T) {
	t.Run("order of every source over every later one", func(t *testing.T) {
		tempDir := t.TempDir()
		flagFile := filepath.Join(tempDir, "flag.yaml")
		envFile := filepath.Join(tempDir, "env.yaml")
		cwdFile := filepath.Join(tempDir, "config.yaml")
		userDir := filepath.Join(tempDir, "user", ".config", "vaultchron")
		if err := os.MkdirAll(userDir, 0o755); err != nil {
			t.Fatal(err)
		}
		userFile := filepath.Join(userDir, "config.yaml")
		exampleFile := filepath.Join(tempDir, "config.example.yaml")

		for _, p := range []string{flagFile, envFile, cwdFile, userFile, exampleFile} {
			if err := os.WriteFile(p, []byte(""), 0o644); err != nil {
				t.Fatal(err)
			}
		}

		getenv := func(k string) string {
			if k == "VAULTCHRON_CONFIG" {
				return envFile
			}
			return ""
		}

		// Flag beats all
		r, err := Resolve(ResolveOptions{
			Flag:         flagFile,
			AllowExample: true,
			Dir:          tempDir,
			GOOS:         "linux",
			Getenv:       getenv,
			HomeDir:      filepath.Join(tempDir, "user"),
		})
		if err != nil || r.Source != "flag" || r.Path != flagFile {
			t.Fatalf("expected flagFile, got %+v, err: %v", r, err)
		}

		// Env beats cwd, user, example
		r, err = Resolve(ResolveOptions{
			Flag:         "",
			AllowExample: true,
			Dir:          tempDir,
			GOOS:         "linux",
			Getenv:       getenv,
			HomeDir:      filepath.Join(tempDir, "user"),
		})
		if err != nil || r.Source != "env" || r.Path != envFile {
			t.Fatalf("expected envFile, got %+v, err: %v", r, err)
		}

		// CWD beats user, example
		getenvNoEnv := func(string) string { return "" }
		r, err = Resolve(ResolveOptions{
			Flag:         "",
			AllowExample: true,
			Dir:          tempDir,
			GOOS:         "linux",
			Getenv:       getenvNoEnv,
			HomeDir:      filepath.Join(tempDir, "user"),
		})
		if err != nil || r.Source != "cwd" || r.Path != cwdFile {
			t.Fatalf("expected cwdFile, got %+v, err: %v", r, err)
		}

		// User beats example
		if err := os.Remove(cwdFile); err != nil {
			t.Fatal(err)
		}
		r, err = Resolve(ResolveOptions{
			Flag:         "",
			AllowExample: true,
			Dir:          tempDir,
			GOOS:         "linux",
			Getenv:       getenvNoEnv,
			HomeDir:      filepath.Join(tempDir, "user"),
		})
		if err != nil || r.Source != "user" || r.Path != userFile {
			t.Fatalf("expected userFile, got %+v, err: %v", r, err)
		}

		// Example when user removed
		if err := os.Remove(userFile); err != nil {
			t.Fatal(err)
		}
		r, err = Resolve(ResolveOptions{
			Flag:         "",
			AllowExample: true,
			Dir:          tempDir,
			GOOS:         "linux",
			Getenv:       getenvNoEnv,
			HomeDir:      filepath.Join(tempDir, "user"),
		})
		if err != nil || r.Source != "example" || r.Path != exampleFile {
			t.Fatalf("expected exampleFile, got %+v, err: %v", r, err)
		}
	})

	t.Run("flag missing", func(t *testing.T) {
		tempDir := t.TempDir()
		missingFlag := filepath.Join(tempDir, "nonexistent.yaml")
		// Put config.yaml in cwd to ensure no fallback happens
		cwdFile := filepath.Join(tempDir, "config.yaml")
		if err := os.WriteFile(cwdFile, []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}

		r, err := Resolve(ResolveOptions{
			Flag: missingFlag,
			Dir:  tempDir,
		})
		if err == nil {
			t.Fatalf("expected error, got %+v", r)
		}
		expectedErr := "config file " + missingFlag + " (from -config) not found"
		if err.Error() != expectedErr {
			t.Fatalf("expected error %q, got %q", expectedErr, err.Error())
		}
	})

	t.Run("env missing", func(t *testing.T) {
		tempDir := t.TempDir()
		missingEnv := filepath.Join(tempDir, "nonexistent-env.yaml")
		// Put config.yaml in cwd to ensure no fallback happens
		cwdFile := filepath.Join(tempDir, "config.yaml")
		if err := os.WriteFile(cwdFile, []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}

		r, err := Resolve(ResolveOptions{
			Dir: tempDir,
			Getenv: func(k string) string {
				if k == "VAULTCHRON_CONFIG" {
					return missingEnv
				}
				return ""
			},
		})
		if err == nil {
			t.Fatalf("expected error, got %+v", r)
		}
		expectedErr := "config file " + missingEnv + " (from $VAULTCHRON_CONFIG) not found"
		if err.Error() != expectedErr {
			t.Fatalf("expected error %q, got %q", expectedErr, err.Error())
		}
	})

	t.Run("env beats ./config.yaml", func(t *testing.T) {
		tempDir := t.TempDir()
		envFile := filepath.Join(tempDir, "custom-env.yaml")
		cwdFile := filepath.Join(tempDir, "config.yaml")
		if err := os.WriteFile(envFile, []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(cwdFile, []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}

		r, err := Resolve(ResolveOptions{
			Dir: tempDir,
			Getenv: func(k string) string {
				if k == "VAULTCHRON_CONFIG" {
					return envFile
				}
				return ""
			},
		})
		if err != nil || r.Source != "env" || r.Path != envFile {
			t.Fatalf("expected envFile, got %+v, err: %v", r, err)
		}
	})

	t.Run("XDG absolute used, XDG relative ignored", func(t *testing.T) {
		tempDir := t.TempDir()
		homeDir := filepath.Join(tempDir, "home")
		homeConfigDir := filepath.Join(homeDir, ".config", "vaultchron")
		if err := os.MkdirAll(homeConfigDir, 0o755); err != nil {
			t.Fatal(err)
		}
		homeConfigFile := filepath.Join(homeConfigDir, "config.yaml")
		if err := os.WriteFile(homeConfigFile, []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}

		absXDGDir := filepath.Join(tempDir, "abs-xdg", "vaultchron")
		if err := os.MkdirAll(absXDGDir, 0o755); err != nil {
			t.Fatal(err)
		}
		absXDGFile := filepath.Join(absXDGDir, "config.yaml")
		if err := os.WriteFile(absXDGFile, []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}

		// When XDG is absolute, it is used
		r, err := Resolve(ResolveOptions{
			Dir:     tempDir,
			GOOS:    "linux",
			HomeDir: homeDir,
			Getenv: func(k string) string {
				if k == "XDG_CONFIG_HOME" {
					return filepath.Join(tempDir, "abs-xdg")
				}
				return ""
			},
		})
		if err != nil || r.Source != "user" || r.Path != absXDGFile {
			t.Fatalf("expected absXDGFile, got %+v, err: %v", r, err)
		}

		// When XDG is relative, it is ignored and falls back to HomeDir
		r, err = Resolve(ResolveOptions{
			Dir:     tempDir,
			GOOS:    "linux",
			HomeDir: homeDir,
			Getenv: func(k string) string {
				if k == "XDG_CONFIG_HOME" {
					return filepath.Join("relative", "xdg")
				}
				return ""
			},
		})
		if err != nil || r.Source != "user" || r.Path != homeConfigFile {
			t.Fatalf("expected homeConfigFile, got %+v, err: %v", r, err)
		}
	})

	t.Run("HOME fallback", func(t *testing.T) {
		tempDir := t.TempDir()
		homeDir := filepath.Join(tempDir, "home")
		homeConfigDir := filepath.Join(homeDir, ".config", "vaultchron")
		if err := os.MkdirAll(homeConfigDir, 0o755); err != nil {
			t.Fatal(err)
		}
		homeConfigFile := filepath.Join(homeConfigDir, "config.yaml")
		if err := os.WriteFile(homeConfigFile, []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}

		r, err := Resolve(ResolveOptions{
			Dir:     tempDir,
			GOOS:    "linux",
			HomeDir: homeDir,
			Getenv:  func(string) string { return "" },
		})
		if err != nil || r.Source != "user" || r.Path != homeConfigFile {
			t.Fatalf("expected homeConfigFile, got %+v, err: %v", r, err)
		}
	})

	t.Run("windows uses APPDATA and ignores XDG/HOME", func(t *testing.T) {
		tempDir := t.TempDir()
		appdataDir := filepath.Join(tempDir, "AppData", "vaultchron")
		if err := os.MkdirAll(appdataDir, 0o755); err != nil {
			t.Fatal(err)
		}
		appdataFile := filepath.Join(appdataDir, "config.yaml")
		if err := os.WriteFile(appdataFile, []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}

		homeDir := filepath.Join(tempDir, "home")
		homeConfigFile := filepath.Join(homeDir, ".config", "vaultchron", "config.yaml")
		if err := os.MkdirAll(filepath.Dir(homeConfigFile), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(homeConfigFile, []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}

		r, err := Resolve(ResolveOptions{
			Dir:     tempDir,
			GOOS:    "windows",
			HomeDir: homeDir,
			Getenv: func(k string) string {
				if k == "APPDATA" {
					return filepath.Join(tempDir, "AppData")
				}
				if k == "XDG_CONFIG_HOME" {
					return filepath.Join(tempDir, "xdg")
				}
				return ""
			},
		})
		if err != nil || r.Source != "user" || r.Path != appdataFile {
			t.Fatalf("expected appdataFile, got %+v, err: %v", r, err)
		}
	})

	t.Run("windows with APPDATA empty", func(t *testing.T) {
		tempDir := t.TempDir()
		homeDir := filepath.Join(tempDir, "home")
		homeConfigFile := filepath.Join(homeDir, ".config", "vaultchron", "config.yaml")
		if err := os.MkdirAll(filepath.Dir(homeConfigFile), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(homeConfigFile, []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}

		// Windows ignores HOME/XDG and APPDATA is empty -> user config unavailable
		r, err := Resolve(ResolveOptions{
			Dir:     tempDir,
			GOOS:    "windows",
			HomeDir: homeDir,
			Getenv: func(k string) string {
				if k == "XDG_CONFIG_HOME" {
					return filepath.Join(tempDir, "xdg")
				}
				return ""
			},
		})
		if err == nil {
			t.Fatalf("expected error, got %+v", r)
		}
		if !strings.Contains(err.Error(), "4. (unavailable: no home directory or APPDATA)") {
			t.Fatalf("expected error to mention unavailable APPDATA, got %q", err.Error())
		}
		if !strings.Contains(err.Error(), "to ./config.yaml, then set vault.path and scan.roots") {
			t.Fatalf("expected fallback hint to target ./config.yaml, got %q", err.Error())
		}
	})

	t.Run("darwin uses ~/.config", func(t *testing.T) {
		tempDir := t.TempDir()
		homeDir := filepath.Join(tempDir, "home")
		homeConfigFile := filepath.Join(homeDir, ".config", "vaultchron", "config.yaml")
		if err := os.MkdirAll(filepath.Dir(homeConfigFile), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(homeConfigFile, []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}

		r, err := Resolve(ResolveOptions{
			Dir:     tempDir,
			GOOS:    "darwin",
			HomeDir: homeDir,
			Getenv:  func(string) string { return "" },
		})
		if err != nil || r.Source != "user" || r.Path != homeConfigFile {
			t.Fatalf("expected homeConfigFile, got %+v, err: %v", r, err)
		}
	})

	t.Run("example only with AllowExample", func(t *testing.T) {
		tempDir := t.TempDir()
		exampleFile := filepath.Join(tempDir, "config.example.yaml")
		if err := os.WriteFile(exampleFile, []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}

		// AllowExample: false -> error
		r, err := Resolve(ResolveOptions{
			Dir:          tempDir,
			AllowExample: false,
			Getenv:       func(string) string { return "" },
		})
		if err == nil {
			t.Fatalf("expected error when AllowExample is false, got %+v", r)
		}

		// AllowExample: true -> example found
		r, err = Resolve(ResolveOptions{
			Dir:          tempDir,
			AllowExample: true,
			Getenv:       func(string) string { return "" },
		})
		if err != nil || r.Source != "example" || r.Path != exampleFile {
			t.Fatalf("expected exampleFile when AllowExample is true, got %+v, err: %v", r, err)
		}
	})

	t.Run("example never beats a real config", func(t *testing.T) {
		tempDir := t.TempDir()
		cwdFile := filepath.Join(tempDir, "config.yaml")
		exampleFile := filepath.Join(tempDir, "config.example.yaml")
		if err := os.WriteFile(cwdFile, []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(exampleFile, []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}

		r, err := Resolve(ResolveOptions{
			Dir:          tempDir,
			AllowExample: true,
			Getenv:       func(string) string { return "" },
		})
		if err != nil || r.Source != "cwd" || r.Path != cwdFile {
			t.Fatalf("expected cwdFile, got %+v, err: %v", r, err)
		}
	})

	t.Run("none-found error contains every searched path and the creation hint", func(t *testing.T) {
		tempDir := t.TempDir()
		homeDir := filepath.Join(tempDir, "home")
		expectedUserPath := filepath.Join(homeDir, ".config", "vaultchron", "config.yaml")
		expectedCwdPath := filepath.Join(tempDir, "config.yaml")
		expectedExamplePath := filepath.Join(tempDir, "config.example.yaml")

		// Case 1: AllowExample = true
		r, err := Resolve(ResolveOptions{
			Dir:          tempDir,
			HomeDir:      homeDir,
			GOOS:         "linux",
			AllowExample: true,
			Getenv:       func(string) string { return "" },
		})
		if err == nil {
			t.Fatalf("expected error, got %+v", r)
		}

		errMsg := err.Error()
		if !strings.Contains(errMsg, "no config file found; looked in this order:") {
			t.Errorf("missing header in error: %q", errMsg)
		}
		if !strings.Contains(errMsg, "1. -config (not set)") {
			t.Errorf("missing item 1 in error: %q", errMsg)
		}
		if !strings.Contains(errMsg, "2. $VAULTCHRON_CONFIG (not set)") {
			t.Errorf("missing item 2 in error: %q", errMsg)
		}
		if !strings.Contains(errMsg, "3. "+expectedCwdPath) {
			t.Errorf("missing item 3 in error: %q", errMsg)
		}
		if !strings.Contains(errMsg, "4. "+expectedUserPath) {
			t.Errorf("missing item 4 in error: %q", errMsg)
		}
		if !strings.Contains(errMsg, "5. "+expectedExamplePath) {
			t.Errorf("missing item 5 in error: %q", errMsg)
		}
		expectedHint := "create one by copying config.example.yaml (https://raw.githubusercontent.com/ZeezyCodes/vaultchron/main/config.example.yaml) to " + expectedUserPath + ", then set vault.path and scan.roots"
		if !strings.Contains(errMsg, expectedHint) {
			t.Errorf("missing expected creation hint in error: %q", errMsg)
		}

		// Case 2: AllowExample = false
		r2, err2 := Resolve(ResolveOptions{
			Dir:          tempDir,
			HomeDir:      homeDir,
			GOOS:         "linux",
			AllowExample: false,
			Getenv:       func(string) string { return "" },
		})
		if err2 == nil {
			t.Fatalf("expected error, got %+v", r2)
		}
		errMsg2 := err2.Error()
		if strings.Contains(errMsg2, "5. ") {
			t.Errorf("expected no item 5 when AllowExample is false, got %q", errMsg2)
		}
	})

	t.Run("directory named config.yaml is not a match", func(t *testing.T) {
		tempDir := t.TempDir()
		dirNamedConfig := filepath.Join(tempDir, "config.yaml")
		if err := os.MkdirAll(dirNamedConfig, 0o755); err != nil {
			t.Fatal(err)
		}

		r, err := Resolve(ResolveOptions{
			Dir:    tempDir,
			Getenv: func(string) string { return "" },
		})
		if err == nil {
			t.Fatalf("expected directory named config.yaml to not match, got %+v", r)
		}
	})
}
