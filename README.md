# VaultChron

[![CI](https://github.com/ZeezyCodes/vaultchron/actions/workflows/ci.yml/badge.svg)](https://github.com/ZeezyCodes/vaultchron/actions/workflows/ci.yml)

VaultChron automatically synthesizes structured, publication-grade developer logs from your local git repositories into an Obsidian-style markdown vault using an LLM.

## What It Does / Who It's For

VaultChron crawls configured project directories, harvests git commit telemetry, file churn, and author statistics over a configurable time window, and prompts an LLM to generate rich devlog notes adhering to the Obsidian Callout v3 taxonomy. It maintains an automated devlog index (`00-Dev-Index.md`) with linked recent devlog entries. VaultChron is designed for solo engineers, technical leads, and indie hackers who want a cohesive, queryable knowledge base of their daily engineering work without manually drafting devlogs.

## ⚠️ Data Flow Warning

> [!WARNING]
> **Data Privacy Notice:** VaultChron extracts git diffs, commit messages, branch names, and metadata from your local repositories and transmits them across the network to a third-party LLM provider (e.g., Google Gemini, OpenAI, OpenRouter, or a configured endpoint).
>
> While VaultChron runs an automated heuristic sanitization pass to strip private keys, AWS access keys, Bearer tokens, and standard credential assignments before payload construction (see [internal/collector/git.go](internal/collector/git.go#L417-L430)), **diff redaction is best-effort and not a cryptographic guarantee**.
>
> Before running VaultChron against proprietary, sensitive, or NDA-governed codebases, review your repository contents to ensure no plaintext secrets exist, or configure an air-gapped / local inference backend such as Ollama or LM Studio.

## Quick install

Linux and macOS:

```bash
curl -fsSL https://raw.githubusercontent.com/ZeezyCodes/vaultchron/main/install.sh | sh
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/ZeezyCodes/vaultchron/main/install.ps1 | iex
```

With Go installed (any OS):

```bash
go install github.com/ZeezyCodes/vaultchron/cmd/vaultchron@latest
```

To upgrade, re-run the same install command.

Requirements: Git on PATH and an LLM API key (such as `GOOGLE_API_KEY`) configured in your environment.

| Variable | Description | Default |
| :--- | :--- | :--- |
| `VAULTCHRON_VERSION` | Release version to install | Latest published release |
| `VAULTCHRON_INSTALL_DIR` | Installation directory for binaries | `$HOME/.local/bin` (Linux/macOS), `%LOCALAPPDATA%\Programs\vaultchron` (Windows) |
| `VAULTCHRON_BASE_URL` | Base download URL for release assets | `https://github.com/ZeezyCodes/vaultchron/releases/download/<version>` |

The installer scripts download prebuilt binaries from GitHub Releases, verify archive integrity against `checksums.txt` (this catches file corruption, not a compromised release), and install into a user folder with no administrator privileges required. The scripts are intentionally concise and auditable before execution: see [install.sh](install.sh) and [install.ps1](install.ps1).

### First run

1. Create the configuration file at your per-user location:

   Linux and macOS:
   ```bash
   mkdir -p ~/.config/vaultchron
   curl -fsSLo ~/.config/vaultchron/config.yaml https://raw.githubusercontent.com/ZeezyCodes/vaultchron/main/config.example.yaml
   ```

   Windows (PowerShell):
   ```powershell
   New-Item -ItemType Directory -Force "$env:APPDATA\vaultchron"
   irm https://raw.githubusercontent.com/ZeezyCodes/vaultchron/main/config.example.yaml -OutFile "$env:APPDATA\vaultchron\config.yaml"
   ```

2. Open `~/.config/vaultchron/config.yaml` (or `$env:APPDATA\vaultchron\config.yaml` on Windows) to set `vault.path` (your Obsidian vault directory) and `scan.roots` (directories containing git repositories to scan).
3. Set your LLM API key in your environment (e.g. `export GOOGLE_API_KEY="..."`).
4. Test with a dry run:
   ```bash
   vaultchron -dry-run
   ```
   VaultChron resolves `config.yaml` automatically from your per-user config directory, `$VAULTCHRON_CONFIG`, or the current working directory; use `-config <path>` if your config file is located elsewhere.

## Requirements

- **Go**: Version `1.27.1` or higher (only needed to build from source or use `go install`).
- **Git**: Git CLI installed and accessible on `$PATH`.
- **LLM API Key or Endpoint**: An API key for Google Gemini (`GOOGLE_API_KEY`), or an alternative provider API key if using an OpenAI-compatible endpoint.

## Platform support

Linux is the primary platform and the one the maintainer uses day to day. macOS (amd64, arm64) and Windows (amd64) binaries are built, and the test suite runs on all three platforms in CI on every commit, but the maintainer does not exercise macOS or Windows by hand on every release. If something does not work there, please open an issue with your OS, the command you ran and the output.

## Other ways to install

### Manual install (download and verify)

#### Linux & macOS

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

#### Windows (PowerShell)

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
   powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\vaultchron\deploy\windows\register-task.ps1
   ```

   > [!NOTE]
   > Scripts extracted from a downloaded zip can be blocked by PowerShell with the message 'is not digitally signed'. Running them through `powershell.exe -ExecutionPolicy Bypass -File` as above avoids that for this one run only.
   > You can instead unblock the extracted folder once by running `Get-ChildItem -Recurse . | Unblock-File` from inside it. The Scheduled Task already starts the wrapper with `-ExecutionPolicy Bypass`.

   - `-Time`: Daily trigger time in `HH:mm` format (default: `"07:00"`, mirroring `deploy/vaultchron.timer`).
   - `-TaskName`: Name for the Scheduled Task (default: `"VaultChron"`).
   - `-Unregister`: Remove the task when no longer needed:
     ```powershell
     powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\vaultchron\deploy\windows\register-task.ps1 -Unregister
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

## Configuration

VaultChron resolves its configuration file by checking the following locations in order (first match wins):

1. **Command-line flag**: The `-config <path>` flag. If provided, the file must exist; VaultChron does not fall through to other locations if it is missing.
2. **Environment variable**: `$VAULTCHRON_CONFIG`. If set, the file must exist; VaultChron does not fall through if it is missing.
3. **Working directory**: `./config.yaml` in the current working directory.
4. **Per-user configuration**:
   - **Windows**: `%APPDATA%\vaultchron\config.yaml`
   - **Linux, macOS, and Unix**: `$XDG_CONFIG_HOME/vaultchron/config.yaml` if `$XDG_CONFIG_HOME` is set and absolute, otherwise `~/.config/vaultchron/config.yaml`.
5. **Example fallback**: `./config.example.yaml` in the current working directory (only allowed when running in `-scan` or `-dry-run` mode). Real runs and vault migrations strictly refuse to run against the example file.

If no configuration file is found, VaultChron outputs the list of searched paths and instructions for creating one, then exits with code 1.

Optional environment files for loading API keys without modifying system environment variables:
- **Linux & macOS**: `~/.config/vaultchron/env`
- **Windows**: `%APPDATA%\vaultchron\env`

Tilde (`~`) and environment variables (`$HOME`) in paths are automatically expanded. Both `index_file` and `projects_dir` must be relative paths inside the vault (`\` is accepted and treated as `/`).

| Section | Key | Type | Description |
| :--- | :--- | :--- | :--- |
| `vault` | `path` | string | Target directory of your Obsidian vault (e.g. `~/vault`). |
| `vault` | `index_file` | string | Filename for the root dev index (default: `00-Dev-Index.md`). |
| `vault` | `rollups_dir` | string | *Reserved, currently unused.* Subdirectory for daily rollups (default: `Daily-Rollups`). |
| `vault` | `projects_dir` | string | Subdirectory for project logs (default: `Projects`). |
| `vault` | `recent_days` | int | *Reserved, currently unused.* Number of rolling days tracked in the index (default: `7`). |
| `scan` | `roots` | list | List of root paths searched for git repositories (e.g. `[~/projects]`). |
| `scan` | `max_depth` | int | Directory traversal depth limit for discovering git repos (default: `3`). |
| `scan` | `excludes` | list | Directory names or patterns to ignore during scan (e.g. `.nvm`, `node_modules`, `vendor`). |
| `scan` | `catch_up_days` | int | Number of past calendar days to scan in default day mode (default: `7`, min: `1`). |
| `llm` | `provider` | string | Identifier for LLM provider (default: `gemini`). |
| `llm` | `base_url` | string | *Optional.* OpenAI-compatible endpoint URL. Defaults to Gemini OpenAI-compatible endpoint (`https://generativelanguage.googleapis.com/v1beta/openai`). Compatible with OpenAI (`https://api.openai.com/v1`), OpenRouter (`https://openrouter.ai/api/v1`), or local providers such as Ollama (`http://localhost:11434/v1`) and LM Studio. |
| `llm` | `waterfall` | list | Ordered list of models to try. Fallback proceeds sequentially on HTTP 429 rate limit errors (e.g. `[gemini-3.8-flash, gemini-3.7-flash, gemini-3.6-flash]`). |
| `llm` | `api_key_env` | string | Name of the environment variable containing the API key (default: `GOOGLE_API_KEY`). |
| `llm` | `max_calls_per_run` | int | Maximum LLM calls allowed per execution run; once reached, remaining items report `PENDING` (default: `20`, `0` = unlimited). |
| `project_tags` | `<DirName>` | object | *Optional.* Custom mapping for repository directory names to define `slug` and `lang`. If omitted, slugs are auto-generated and language defaults to `go`. |
| `agent_logs` | `enabled` | bool | Enables harvesting local AI agent session logs (e.g. Antigravity, Poolside) to supply session goals and outcomes to the prompt. **Default: `false`** for privacy. |
| `agent_logs` | `antigravity_path` | string | Path to local Antigravity session directory (default: `~/.antigravity`). |
| `agent_logs` | `poolside_path` | string | Path to local Poolside session directory (default: `~/.poolside`). |

## CLI Flags

### `vaultchron`

Main engine for scanning git activity, prompting LLM synthesis, and updating the vault.

| Flag | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `-config` | string | `""` | path to config.yaml (order: -config, $VAULTCHRON_CONFIG, ./config.yaml, per-user config, ./config.example.yaml for scan/dry-run) |
| `-date` | string | `""` | single calendar day to generate devlog for (`YYYY-MM-DD`, today writes `partial: true`) |
| `-from` | string | `""` | start date for devlog range (`YYYY-MM-DD`, inclusive) |
| `-to` | string | `""` | end date for devlog range (`YYYY-MM-DD`, inclusive, defaults to yesterday) |
| `-catch-up` | int | `0` | number of catch-up days in default day mode (overrides `scan.catch_up_days`, min: `1`) |
| `-max-calls` | int | `-1` | maximum LLM calls allowed per run (`0` = unlimited, overrides `llm.max_calls_per_run`) |
| `-scan` | bool | `false` | run collector only: discover repos, harvest metadata, print results, and exit 0 |
| `-dry-run` | bool | `false` | skip LLM calls and vault writes; render populated plan preview to stdout |
| `-repo` | string | `""` | target a single repository by base directory name (e.g. my-project) |
| `-force` | bool | `false` | overwrite existing notes; process repositories even if they have zero commits in legacy window mode |
| `-window` | string | `"24.hours.ago"` | legacy git time window for --since log query and HEAD@{<window>} diff reference |
| `-migrate-vault` | bool | `false` | migrate legacy devlog notes in vault to v3 callout taxonomy in-place |
| `-migrate-v3` | bool | `false` | alias for -migrate-vault |

### `vaultchron_migrate`

Standalone migration utility for retrofitting existing Obsidian devlogs to the v3 callout taxonomy without requiring LLM invocations.

| Flag | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `-vault` | string | `""` | path to the Obsidian vault root (defaults to config.yaml vault.path or default vault path) |
| `-config` | string | `""` | path to config.yaml (order: -config, $VAULTCHRON_CONFIG, ./config.yaml, per-user config, ./config.example.yaml for dry-run) |
| `-file` | string | `""` | migrate a single devlog markdown file instead of the whole vault |
| `-dry-run` | bool | `false` | display what would be migrated without modifying files |

## Day-based devlogs

VaultChron defaults to generating structured devlogs on a local calendar-day boundary rather than rolling 24-hour time windows:

- **Catch-Up Lookback:** When run without date flags, VaultChron scans the preceding calendar days `[today - N, yesterday]` (where `N` is `scan.catch_up_days`, default 7, or `-catch-up`). Processing is day-major and chronological (oldest day first across all discovered repositories). Today is not generated during default catch-up runs.
- **Skip-Existing & Partial Notes:** Existing devlogs are skipped by default. However, if an existing note for a previous day is marked `partial: true` (e.g. generated before the day concluded), it is automatically regenerated. Re-running on a partial note for today is skipped unless `-force` is passed.
- **Targeted Dates & Ranges:** Use `-date YYYY-MM-DD` for a specific day (including today with `partial: true`), or `-from YYYY-MM-DD [-to YYYY-MM-DD]` for a historical date range.
- **Call Cap Protection:** Execution respects `llm.max_calls_per_run` (default 20, 0 = unlimited) or `-max-calls`. Once the cap is reached, cheap checks continue and remaining pairs are reported as `PENDING` without invoking the LLM, allowing subsequent runs to resume where the last run stopped.
- **Index Handling:** If the vault index file does not exist, it is created with a minimal recent dev logs table before the first note is written. If the index exists but is unusable (for example, missing required section headers), an update failure stops the run: devlog notes are not written, remaining pairs stay pending, and you can fix the index and run again to continue.
- **Rollup Notes Not Generated:** Daily rollup notes are not generated by VaultChron; the `[[Daily-Rollups/<date>|📅 Rollup]]` breadcrumb is a placeholder link, and `rollups_dir` and `recent_days` remain reserved.
- **Empty Days:** Repo-days with zero commits are never written into notes, even when `-force` is supplied.
- **Legacy Compatibility:** Supplying `-window` retains the previous reflog window collection workflow. Note that agent session log harvesting (`agent_logs.enabled`) is active only in legacy `-window` mode.

### Existing vault

When pointing VaultChron to an existing Obsidian vault:
- **Target vault location**: Set `vault.path` in your configuration to the root directory of your existing Obsidian vault.
- **Contained write scope**: Devlog notes are written under `<vault>/<projects_dir>/<repo>/Devlog/` (e.g. `<vault>/Projects/my-project/Devlog/2026-10-02.md`). The write scope also includes creating the index file (`<vault>/<index_file>`) and `<projects_dir>/<repo>/Overview.md`, each only if missing and never overwritten. VaultChron will not touch other directories or notes in your vault.
- **In-place index maintenance**: The root index file (`00-Dev-Index.md` by default) is updated in place under a `## Recent Dev Logs` or `## Recent Activity` heading, inserting entries in date order (newest first) and replacing an existing entry for the same project and day without disturbing your existing notes or surrounding content.
- **Preservation of existing notes**: Notes already written for complete days are skipped automatically, preventing unintended overwrites.
- **Index error safeguard**: If the index file exists but cannot be updated (see Index Handling above), VaultChron aborts the run before writing note files to disk.
- **Backup recommendation**: Always back up or commit your Obsidian vault to version control before running VaultChron for the first time against live documentation.
- **Config safety**: Real runs now refuse to start without a valid configuration file, preventing unintended runs against default or example templates.

### Missed days

VaultChron handles missed days and offline periods automatically:
- **Automatic catch-up**: By default, every run inspects the preceding 7 calendar days (`scan.catch_up_days` or `-catch-up`), automatically identifying and generating devlogs for any days that lack notes.
- **Historical backfill ranges**: To generate devlogs for an extended past period, supply `-from` and `-to`:
  ```bash
  vaultchron -from 2026-09-01 -to 2026-09-15
  ```
- **Call cap and re-running**: Large backfills honor the `llm.max_calls_per_run` budget (default 20 calls, or `-max-calls`). If a run reaches the call cap, ungenerated days report `PENDING` without making LLM calls. Simply running `vaultchron` again resumes where the previous run stopped until all missed days are filled.
- **Limitations**:
  - **Checked-out branch**: Only the currently checked-out branch is scanned for each repository.
  - **Committer timestamps**: Commit inclusion is based on git committer dates in local calendar time.
  - **Agent logs**: Local AI agent session logs (`agent_logs.enabled`) are not used in day mode (session log harvesting is supported only in legacy `-window` mode).

## Scheduling

VaultChron can run unattended on a daily schedule across Linux, macOS, and Windows. Missed days resulting from sleep, reboot, or offline time are automatically filled by the catch-up mechanism on subsequent runs.

### Linux (systemd)

Set up a systemd user timer to run VaultChron daily at 07:00:

```bash
mkdir -p ~/.config/systemd/user
curl -fsSLo ~/.config/systemd/user/vaultchron.service https://raw.githubusercontent.com/ZeezyCodes/vaultchron/main/deploy/vaultchron.service
curl -fsSLo ~/.config/systemd/user/vaultchron.timer https://raw.githubusercontent.com/ZeezyCodes/vaultchron/main/deploy/vaultchron.timer
systemctl --user daemon-reload
systemctl --user enable --now vaultchron.timer
```

To allow the user timer to run when you are logged out, optionally enable lingering:
```bash
loginctl enable-linger "$USER"
```

View execution logs with:
```bash
journalctl --user -u vaultchron
```

**Running from a repository checkout**:
If running VaultChron directly from a git checkout rather than a binary installation on `PATH`, use the provided wrapper script `deploy/vaultchron.sh`. Set `VAULTCHRON_HOME` to your repository directory to have the wrapper change into the checkout and run with `-config "$VAULTCHRON_HOME/config.yaml"`:
```bash
export VAULTCHRON_HOME="/path/to/vaultchron"
deploy/vaultchron.sh
```

### macOS (launchd)

Schedule VaultChron as a macOS launchd user agent running daily at 07:00:

1. Download and load the launchd property list:
   ```bash
   mkdir -p ~/Library/LaunchAgents
   curl -fsSLo ~/Library/LaunchAgents/io.github.zeezycodes.vaultchron.plist https://raw.githubusercontent.com/ZeezyCodes/vaultchron/main/deploy/macos/io.github.zeezycodes.vaultchron.plist
   launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/io.github.zeezycodes.vaultchron.plist
   ```

2. To unload and stop the scheduled agent:
   ```bash
   launchctl bootout gui/$(id -u) ~/Library/LaunchAgents/io.github.zeezycodes.vaultchron.plist
   ```

3. Execution logs are appended to:
   `~/.config/vaultchron/logs/vaultchron.log`

> [!NOTE]
> The launchd agent configuration is linted via CI (`plutil -lint`) but has not been verified on real macOS hardware. On macOS, a system that is asleep at the scheduled run time (07:00) executes the job upon waking; if the system was powered off, the catch-up mechanism automatically processes missed days during the next run.

### Windows

The Scheduled Task is registered with `deploy/windows/register-task.ps1`; see [Windows Setup & Usage](#windows-setup--usage) for details:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\deploy\windows\register-task.ps1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\deploy\windows\register-task.ps1 -Unregister
```

## How It Works

VaultChron orchestrates a deterministic 7-stage pipeline:

1. **Repository Discovery**: Recursively traverses configured `scan.roots` down to `scan.max_depth`, excluding matches from `scan.excludes`, to locate valid git repositories.
2. **Metadata Harvesting**: Executes non-destructive git commands (`git log`, `git shortlog`, `git rev-parse`) to compute commit counts, author telemetry, churn statistics, and file modifications within the selected `-window`.
3. **Diff Sanitization & Truncation**: Generates the unified diff against the window base, filters out noise (lockfiles, minified assets, vendor dirs), redacts credential patterns via regex heuristics, and caps total diff size at 25,000 UTF-8 characters.
4. **Context Enrichment (Optional)**: If `agent_logs.enabled` is true, extracts session goals and task outcomes from local AI agent sessions matching the project window.
5. **LLM Synthesis**: Submits structured prompt context to the configured LLM endpoint, utilizing model waterfall failover on rate limits (HTTP 429).
6. **Note Rendering**: Assembles note markdown strictly conforming to the Obsidian Callout v3 specification (`[!note]-`, `[!abstract]`, `[!info]`, `[!bug]`, `[!warning]`, `[!check]`).
7. **Atomic Vault Write**: Writes notes using temporary file creation and atomic renames, preventing corrupt partial writes, and updates `00-Dev-Index.md` with current project telemetry.

## Security

For vulnerability disclosures and reporting procedures, see [SECURITY.md](SECURITY.md).

## Contributing

For guidelines on setting up your local development environment, testing, and pull request standards, see [CONTRIBUTING.md](CONTRIBUTING.md).

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.
