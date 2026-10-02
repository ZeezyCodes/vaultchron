# VaultChron

[![CI](https://github.com/ZeezyCodes/vaultchron/actions/workflows/ci.yml/badge.svg)](https://github.com/ZeezyCodes/vaultchron/actions/workflows/ci.yml)
*(Note: CI badge activates once GitHub Actions executes on the public repository)*

VaultChron automatically synthesizes structured, publication-grade developer logs from your local git repositories into an Obsidian-style markdown vault using an LLM.

## What It Does / Who It's For

VaultChron crawls configured project directories, harvests git commit telemetry, file churn, and author statistics over a configurable time window, and prompts an LLM to generate rich devlog notes adhering to the Obsidian Callout v3 taxonomy. It maintains an automated project cockpit (`00-Dev-Index.md`) with linked recent rollups and project directories. VaultChron is designed for solo engineers, technical leads, and indie hackers who want a cohesive, queryable knowledge base of their daily engineering work without manually drafting devlogs.

## ⚠️ Data Flow Warning

> [!WARNING]
> **Data Privacy Notice:** VaultChron extracts git diffs, commit messages, branch names, and metadata from your local repositories and transmits them across the network to a third-party LLM provider (e.g., Google Gemini, OpenAI, OpenRouter, or a configured endpoint).
>
> While VaultChron runs an automated heuristic sanitization pass to strip private keys, AWS access keys, Bearer tokens, and standard credential assignments before payload construction (see [internal/collector/git.go](internal/collector/git.go#L417-L430)), **diff redaction is best-effort and not a cryptographic guarantee**.
>
> Before running VaultChron against proprietary, sensitive, or NDA-governed codebases, review your repository contents to ensure no plaintext secrets exist, or configure an air-gapped / local inference backend such as Ollama or LM Studio.

## Requirements

- **Go**: Version `1.27.1` or higher is specified in `go.mod`.
  > [!NOTE]
  > `go.mod` pins `go 1.27.1` as the minimum toolchain, and running linters locally needs `golangci-lint` v2.14.0+ because older releases cannot read Go 1.27 export data.
- **Git**: Git CLI installed and accessible on `$PATH`.
- **LLM API Key or Endpoint**: An API key for Google Gemini (`GOOGLE_API_KEY`), or an alternative provider API key if using an OpenAI-compatible endpoint.

## Installation

### Prerequisites

Before running VaultChron, ensure the following prerequisites are met:
- **Git**: Git CLI installed and available on `$PATH`.
- **LLM API Key**: API key configured in your environment (e.g. `export GOOGLE_API_KEY="..."`).
- **Configuration**: Copy `config.example.yaml` to `config.yaml` (`cp config.example.yaml config.yaml`) and configure your vault and scan directories.

### Download a Release (Linux & macOS)

Prebuilt, checksummed binaries for Linux and macOS (`amd64` and `arm64`) are published on [GitHub Releases](https://github.com/ZeezyCodes/vaultchron/releases).

Run the following snippet to download, verify, and extract a release:

```bash
# Specify the desired release version (e.g. v0.1.0)
VERSION="vX.Y.Z"
OS="linux"   # "linux" or "darwin"
ARCH="amd64" # "amd64" or "arm64"

# Archive names omit the leading 'v' of the tag
VER_NUM="${VERSION#v}"
ARCHIVE="vaultchron_${VER_NUM}_${OS}_${ARCH}.tar.gz"
BASE_URL="https://github.com/ZeezyCodes/vaultchron/releases/download/${VERSION}"

# Download the archive and the checksum list
curl -fsSLO "${BASE_URL}/${ARCHIVE}"
curl -fsSLO "${BASE_URL}/checksums.txt"

# Verify the archive you downloaded (run the line for your platform)
grep -F "  ${ARCHIVE}" checksums.txt | sha256sum -c -        # Linux
grep -F "  ${ARCHIVE}" checksums.txt | shasum -a 256 -c -    # macOS

# Extract into its own directory (the archive has no top-level folder)
mkdir -p vaultchron && tar -xzf "${ARCHIVE}" -C vaultchron
cd vaultchron && ./vaultchron -version
```

> [!NOTE]
> **Unsigned Binaries & macOS Gatekeeper:**
> Binaries are currently unsigned. If you download binaries via a macOS web browser rather than `curl`, macOS may quarantine them. Remove the quarantine attribute with:
> ```bash
> xattr -d com.apple.quarantine vaultchron vaultchron_migrate
> ```

> [!NOTE]
> **Windows Support:**
> Windows (`amd64`) is supported. Windows `arm64` binaries are not provided yet. Binaries are currently unsigned.

### Download a Release (Windows PowerShell)

Prebuilt, checksummed binaries for Windows (`amd64`) are published on [GitHub Releases](https://github.com/ZeezyCodes/vaultchron/releases).

Run the following snippet to download, verify, and extract a release:

```powershell
# Specify the desired release version (e.g. v0.1.0)
$ProgressPreference = 'SilentlyContinue'
$Version = "vX.Y.Z"
$VerNum  = $Version.TrimStart("v")
$Archive = "vaultchron_${VerNum}_windows_amd64.zip"
$BaseUrl = "https://github.com/ZeezyCodes/vaultchron/releases/download/$Version"

# Download the archive and the checksum list
Invoke-WebRequest -Uri "$BaseUrl/$Archive" -OutFile $Archive
Invoke-WebRequest -Uri "$BaseUrl/checksums.txt" -OutFile checksums.txt

# Verify the archive you downloaded
$line = Select-String -Path checksums.txt -SimpleMatch "  $Archive" | Select-Object -First 1
if (-not $line) { throw "No checksum entry found for $Archive" }
$expected = ($line.Line -split "\s+")[0].ToLower()
$actual   = (Get-FileHash -Path $Archive -Algorithm SHA256).Hash.ToLower()
if ($actual -ne $expected) { throw "Checksum mismatch for $Archive" }

# Extract into its own directory and check the version
Expand-Archive -Path $Archive -DestinationPath vaultchron
.\vaultchron\vaultchron.exe -version
```

#### Windows Setup & Usage

Follow these steps to configure and run VaultChron on Windows:

1. **Prerequisites**:
   - **Git for Windows**: Git CLI must be installed and available on `PATH`.
   - **LLM API Key**: An API key (e.g. `GOOGLE_API_KEY`) for Gemini or an alternative OpenAI-compatible provider.
   *(Note: Go is NOT needed when running prebuilt release binaries.)*

2. **Browser Downloads & SmartScreen**:
   If the release zip was downloaded through a web browser rather than `Invoke-WebRequest`, Windows may block execution. Unblock the downloaded zip with:
   ```powershell
   Unblock-File -Path .\vaultchron_*_windows_amd64.zip
   ```
   Because binaries are currently unsigned, Windows SmartScreen may show a warning on first run. Click **More info** and then **Run anyway** to proceed.

3. **Configuration**:
   Copy `config.example.yaml` to `config.yaml`:
   ```powershell
   Copy-Item .\vaultchron\config.example.yaml .\vaultchron\config.yaml
   ```
   Open `config.yaml` to configure your vault and scan directories:
   - Tilde (`~`) and `$HOME` paths automatically expand to your user profile directory on Windows.
   - For path separators in YAML, use forward slashes (e.g. `C:/Users/me/vault`) or single quotes for backslash paths (e.g. `'C:\Users\me\vault'`).

4. **API Key Setup**:
   Set the API key in your user environment (takes effect in new shells):
   ```powershell
   [Environment]::SetEnvironmentVariable('GOOGLE_API_KEY', 'your-api-key-here', 'User')
   ```
   For the current session only, set a session variable:
   ```powershell
   $env:GOOGLE_API_KEY = 'your-api-key-here'
   ```
   Alternatively, create the optional environment file at `%APPDATA%\vaultchron\env` with lines in `KEY=VALUE` format:
   ```text
   GOOGLE_API_KEY=your-api-key-here
   ```
   The `%APPDATA%\vaultchron\env` file is read ONLY by `vaultchron-run.ps1` (and therefore the Scheduled Task); manual runs of `vaultchron.exe` need the user environment variable (new shell) or a session variable (`$env:GOOGLE_API_KEY = 'your-api-key-here'`).

5. **Running Once by Hand**:
   Execute VaultChron directly from PowerShell:
   ```powershell
   cd vaultchron
   .\vaultchron.exe -scan
   .\vaultchron.exe -dry-run
   .\vaultchron.exe
   ```

6. **Scheduling with Scheduled Tasks**:
   Schedule VaultChron to run automatically every day using the registration script:
   ```powershell
   powershell.exe -ExecutionPolicy Bypass -File .\vaultchron\deploy\windows\register-task.ps1
   ```
   - `-Time`: Daily trigger time in `HH:mm` format (default: `"07:00"`, mirroring `deploy/vaultchron.timer`).
   - `-TaskName`: Name for the Scheduled Task (default: `"VaultChron"`).
   - `-Unregister`: Remove the task when no longer needed:
     ```powershell
     powershell.exe -ExecutionPolicy Bypass -File .\vaultchron\deploy\windows\register-task.ps1 -Unregister
     ```
   The registered task runs under your current user account with interactive logon (runs only while you are logged on; no password is stored). If the PC is off at the trigger time, it runs when available (`StartWhenAvailable`). The task stores the absolute path of the extracted folder, so keep the folder where it is or re-run `register-task.ps1` after moving it.

7. **Log Location**:
   When executed via the wrapper script (`deploy\windows\vaultchron-run.ps1`) or Scheduled Task, timestamped logs are written to:
   `%LOCALAPPDATA%\vaultchron\logs\vaultchron.log`

### Via `go install`

Install the latest binaries directly into your `$GOPATH/bin`:

```bash
go install github.com/ZeezyCodes/vaultchron/cmd/vaultchron@latest
go install github.com/ZeezyCodes/vaultchron/cmd/vaultchron_migrate@latest
```

### From Source

Clone the repository and build the binaries using Go 1.27.1+:

```bash
git clone https://github.com/ZeezyCodes/vaultchron.git
cd vaultchron
go build -o vaultchron ./cmd/vaultchron
go build -o vaultchron_migrate ./cmd/vaultchron_migrate
```

## Quick Start

1. **Initialize configuration:**
   Copy the example configuration to `config.yaml`:
   ```bash
   cp config.example.yaml config.yaml
   ```

2. **Configure your environment:**
   Set your LLM API key (or add it to `~/.config/vaultchron/env`):
   ```bash
   export GOOGLE_API_KEY="your-api-key-here"
   ```

3. **Verify repository discovery with `-scan`:**
   Inspect which git repositories are discovered and their recent commit volume without making LLM calls or modifying your vault:
   ```bash
   ./vaultchron -scan
   ```

4. **Dry-run generation:**
   Preview generated devlog data structures to stdout without calling the LLM or writing files:
   ```bash
   ./vaultchron -dry-run
   ```

5. **Generate devlogs:**
   Run VaultChron for real to generate notes and update your Obsidian index:
   ```bash
   ./vaultchron
   ```

## Configuration Reference

VaultChron looks for `config.yaml` in the working directory (falling back to `config.example.yaml` if not found), or loads the path specified via `-config`. Tilde (`~`) and environment variables (`$HOME`) in paths are automatically expanded.

| Section | Key | Type | Description |
| :--- | :--- | :--- | :--- |
| `vault` | `path` | string | Target directory of your Obsidian vault (e.g. `~/vault`). |
| `vault` | `index_file` | string | Filename for the root dev index (default: `00-Dev-Index.md`). |
| `vault` | `rollups_dir` | string | Subdirectory for daily rollups (default: `Daily-Rollups`). |
| `vault` | `projects_dir` | string | Subdirectory for project logs (default: `Projects`). |
| `vault` | `recent_days` | int | Number of rolling days tracked in the index (default: `7`). |
| `scan` | `roots` | list | List of root paths searched for git repositories (e.g. `[~/projects]`). |
| `scan` | `max_depth` | int | Directory traversal depth limit for discovering git repos (default: `3`). |
| `scan` | `excludes` | list | Directory names or patterns to ignore during scan (e.g. `.nvm`, `node_modules`, `vendor`). |
| `llm` | `provider` | string | Identifier for LLM provider (default: `gemini`). |
| `llm` | `base_url` | string | *Optional.* OpenAI-compatible endpoint URL. Defaults to Gemini OpenAI-compatible endpoint (`https://generativelanguage.googleapis.com/v1beta/openai`). Compatible with OpenAI (`https://api.openai.com/v1`), OpenRouter (`https://openrouter.ai/api/v1`), or local providers such as Ollama (`http://localhost:11434/v1`) and LM Studio. |
| `llm` | `waterfall` | list | Ordered list of models to try. Fallback proceeds sequentially on HTTP 429 rate limit errors (e.g. `[gemini-3.8-flash, gemini-3.7-flash, gemini-3.6-flash]`). |
| `llm` | `api_key_env` | string | Name of the environment variable containing the API key (default: `GOOGLE_API_KEY`). |
| `project_tags` | `<DirName>` | object | *Optional.* Custom mapping for repository directory names to define `slug` and `lang`. If omitted, slugs are auto-generated and language defaults to `go`. |
| `agent_logs` | `enabled` | bool | Enables harvesting local AI agent session logs (e.g. Antigravity, Poolside) to supply session goals and outcomes to the prompt. **Default: `false`** for privacy. |
| `agent_logs` | `antigravity_path` | string | Path to local Antigravity session directory (default: `~/.antigravity`). |
| `agent_logs` | `poolside_path` | string | Path to local Poolside session directory (default: `~/.poolside`). |

## CLI Flags

### `vaultchron`

Main engine for scanning git activity, prompting LLM synthesis, and updating the vault.

| Flag | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `-config` | string | `""` | path to config.yaml (defaults to config.yaml or config.example.yaml) |
| `-scan` | bool | `false` | run collector only: discover repos, harvest metadata, print results, and exit 0 |
| `-window` | string | `"24.hours.ago"` | git time window for --since log query and HEAD@{<window>} diff reference |
| `-dry-run` | bool | `false` | skip LLM calls and vault writes; render populated DevlogData preview to stdout |
| `-repo` | string | `""` | target a single repository by base directory name (e.g. my-project) |
| `-force` | bool | `false` | process repositories even if they have zero commits in the window |
| `-migrate-vault` | bool | `false` | migrate legacy devlog notes in vault to v3 callout taxonomy in-place |
| `-migrate-v3` | bool | `false` | alias for -migrate-vault |

### `vaultchron_migrate`

Standalone migration utility for retrofitting existing Obsidian devlogs to the v3 callout taxonomy without requiring LLM invocations.

| Flag | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `-vault` | string | `""` | path to the Obsidian vault root (defaults to config.yaml vault.path or default vault path) |
| `-config` | string | `""` | path to config.yaml |
| `-file` | string | `""` | migrate a single devlog markdown file instead of the whole vault |
| `-dry-run` | bool | `false` | display what would be migrated without modifying files |

## How It Works

VaultChron orchestrates a deterministic 7-stage pipeline:

1. **Repository Discovery**: Recursively traverses configured `scan.roots` down to `scan.max_depth`, excluding matches from `scan.excludes`, to locate valid git repositories.
2. **Metadata Harvesting**: Executes non-destructive git commands (`git log`, `git shortlog`, `git rev-parse`) to compute commit counts, author telemetry, churn statistics, and file modifications within the selected `-window`.
3. **Diff Sanitization & Truncation**: Generates the unified diff against the window base, filters out noise (lockfiles, minified assets, vendor dirs), redacts credential patterns via regex heuristics, and caps total diff size at 25,000 UTF-8 characters.
4. **Context Enrichment (Optional)**: If `agent_logs.enabled` is true, extracts session goals and task outcomes from local AI agent sessions matching the project window.
5. **LLM Synthesis**: Submits structured prompt context to the configured LLM endpoint, utilizing model waterfall failover on rate limits (HTTP 429).
6. **Note Rendering**: Assembles note markdown strictly conforming to the Obsidian Callout v3 specification (`[!note]-`, `[!abstract]`, `[!info]`, `[!bug]`, `[!warning]`, `[!check]`).
7. **Atomic Vault Write**: Writes notes using temporary file creation and atomic renames, preventing corrupt partial writes, and updates `00-Dev-Index.md` with current project telemetry.

## Deployment

VaultChron can run unattended on a daily schedule using the provided deployment scripts:

### Linux & macOS (systemd)

- **Execution wrapper (`deploy/vaultchron.sh`)**: Sources environment variables from `~/.config/vaultchron/env`, resolves the working directory via `VAULTCHRON_HOME` (defaulting to `/opt/vaultchron`), runs `vaultchron`, and logs stdout/stderr to `~/.config/vaultchron/logs/vaultchron.log`.
- **Systemd service (`deploy/vaultchron.service`)**: Oneshot service unit invoking `vaultchron.sh`.
- **Systemd timer (`deploy/vaultchron.timer`)**: Triggers execution daily at 07:00:00 local time with `Persistent=true`.

To set up the systemd timer:
```bash
cp deploy/vaultchron.service deploy/vaultchron.timer ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now vaultchron.timer
```

### Windows (Scheduled Tasks)

- **Execution wrapper (`deploy/windows/vaultchron-run.ps1`)**: Sources environment variables from `%APPDATA%\vaultchron\env`, resolves the installation directory via `$env:VAULTCHRON_HOME` (defaulting to the archive root or `%LOCALAPPDATA%\vaultchron`), executes `vaultchron.exe`, and appends UTF-8 stdout/stderr to `%LOCALAPPDATA%\vaultchron\logs\vaultchron.log`.
- **Task registration script (`deploy/windows/register-task.ps1`)**: Registers a Windows Scheduled Task executing daily at 07:00 (matching the systemd timer) with `StartWhenAvailable`, battery execution allowed, a 2-hour timeout, and interactive user logon without requiring administrator privileges.

To register or unregister the Scheduled Task:
```powershell
powershell.exe -ExecutionPolicy Bypass -File .\deploy\windows\register-task.ps1
powershell.exe -ExecutionPolicy Bypass -File .\deploy\windows\register-task.ps1 -Unregister
```

## Security

For vulnerability disclosures and reporting procedures, see [SECURITY.md](SECURITY.md).

## Contributing

For guidelines on setting up your local development environment, testing, and pull request standards, see [CONTRIBUTING.md](CONTRIBUTING.md).

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.
