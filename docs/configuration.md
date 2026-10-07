# Configuration

## Where the config file is

VaultChron uses the first of these that exists:

1. The `-config <path>` flag. The file must exist. VaultChron does not try other places if it is missing.
2. The `VAULTCHRON_CONFIG` environment variable. Same rule: the file must exist.
3. `./config.yaml` in the current directory.
4. The per-user file:
   - Linux and macOS: `$XDG_CONFIG_HOME/vaultchron/config.yaml` if `XDG_CONFIG_HOME` is set and absolute, otherwise `~/.config/vaultchron/config.yaml`.
   - Windows: `%APPDATA%\vaultchron\config.yaml`. HOME and XDG are ignored on Windows.
5. `./config.example.yaml`, only for `-scan` and `-dry-run`. Real runs and vault migrations refuse to use the example file.

An empty flag or variable counts as not set. If no file is found, VaultChron prints the places it looked, explains how to create the file, and exits with code 1.

## Config keys

Start from [config.example.yaml](https://github.com/ZeezyCodes/vaultchron/blob/main/config.example.yaml). `~` and `$HOME` in paths are expanded.

| Section | Key | Default | Meaning |
| :--- | :--- | :--- | :--- |
| `vault` | `path` | `~/vault` | Your Obsidian vault folder. |
| `vault` | `index_file` | `00-Dev-Index.md` | Name of the index note in the vault root. |
| `vault` | `projects_dir` | `Projects` | Folder inside the vault for project notes. |
| `vault` | `rollups_dir` | `Daily-Rollups` | Reserved. Read but not used. |
| `vault` | `recent_days` | `7` | Reserved. Not used. |
| `scan` | `roots` | none | Folders searched for git repositories. |
| `scan` | `max_depth` | `3` | How deep to search below each root. |
| `scan` | `catch_up_days` | `7` | Days to look back when no date flag is given. Minimum 1. |
| `scan` | `excludes` | none | Directory names or patterns to skip. |
| `llm` | `provider` | `gemini` | LLM provider name. |
| `llm` | `base_url` | Gemini's OpenAI-compatible endpoint | Optional. Any OpenAI-compatible endpoint: OpenAI (`https://api.openai.com/v1`), OpenRouter (`https://openrouter.ai/api/v1`), or a local server such as Ollama (`http://localhost:11434/v1`) or LM Studio. |
| `llm` | `waterfall` | see example | Model names, tried in order. The next one is tried when a call gets HTTP 429. |
| `llm` | `api_key_env` | `GOOGLE_API_KEY` | Name of the environment variable that holds the API key. |
| `llm` | `max_calls_per_run` | `20` | Most LLM calls in one run. `0` means no limit. |
| `project_tags` | `<directory name>` | none | Optional. Sets `slug` and `lang` for a repository. Without it, the slug is generated from the name and `lang` is `go`. |
| `agent_logs` | `enabled` | `false` | Read local AI agent session logs. Off by default for privacy. |
| `agent_logs` | `antigravity_path` | `~/.antigravity` | Antigravity session folder. |
| `agent_logs` | `poolside_path` | `~/.poolside` | Poolside session folder. |

`index_file` and `projects_dir` must be relative paths inside the vault. They cannot be empty or `.`, contain `..`, start with `/`, or include a drive letter. `\` is accepted and treated as `/`. A bad value stops VaultChron at startup with an error that names the key.

## Programs and flags

### vaultchron

| Flag | Default | Meaning |
| :--- | :--- | :--- |
| `-config` | none | Path to the config file. |
| `-date` | none | Generate the devlog for one calendar day (`YYYY-MM-DD`). Today is allowed and is marked `partial: true`. |
| `-from` | none | Start of a date range (`YYYY-MM-DD`). |
| `-to` | yesterday | End of a date range (`YYYY-MM-DD`). |
| `-catch-up` | `0` | Days to look back in default mode. Overrides `scan.catch_up_days`. |
| `-max-calls` | `-1` | Most LLM calls in this run. `0` means no limit. `-1` uses `llm.max_calls_per_run`. |
| `-scan` | off | Find repositories and print what was collected. No LLM, no writes. |
| `-dry-run` | off | Skip LLM calls and vault writes. Print a preview of the notes. |
| `-repo` | none | Only this repository, by directory name. |
| `-force` | off | Overwrite existing notes. Also process repositories with no commits in the older window mode. |
| `-window` | `24.hours.ago` | Git time window for the older rolling-window mode. |
| `-migrate-vault` | off | Convert older devlog notes in the vault to the current callout layout. |
| `-migrate-v3` | off | Same as `-migrate-vault`. |
| `-version` | off | Print the version and exit. |

### vaultchron_migrate

Converts older devlog notes to the current callout layout. It makes no LLM calls.

| Flag | Default | Meaning |
| :--- | :--- | :--- |
| `-vault` | config value | Path to the vault root. |
| `-config` | none | Path to the config file. Same lookup order as above. |
| `-file` | none | Convert one note instead of the whole vault. |
| `-dry-run` | off | Show what would change. Change nothing. |
| `-version` | off | Print the version and exit. |

## Environment variables

| Variable | Used by | Meaning |
| :--- | :--- | :--- |
| `VAULTCHRON_CONFIG` | `vaultchron`, `vaultchron_migrate` | Path to the config file. |
| `VAULTCHRON_HOME` | `deploy/vaultchron.sh`, `vaultchron-run.ps1` | Folder that holds the program and `config.yaml`. |
| `VAULTCHRON_VERSION` | install scripts | Release to install. Default: latest published release. |
| `VAULTCHRON_INSTALL_DIR` | install scripts | Install folder. Default: `~/.local/bin`, or `%LOCALAPPDATA%\Programs\vaultchron` on Windows. |
| `VAULTCHRON_BASE_URL` | install scripts | Where release files are downloaded from. Default: the GitHub release for the chosen version. |
| `XDG_CONFIG_HOME` | `vaultchron` | Per-user config location on Linux and macOS. |
| `APPDATA` | `vaultchron` | Per-user config location on Windows. |
| `GOOGLE_API_KEY` | `vaultchron` | Default API key variable. Change it with `llm.api_key_env`. |

You can keep API keys in an env file of `KEY=value` lines: `~/.config/vaultchron/env` on Linux and macOS, `%APPDATA%\vaultchron\env` on Windows. The systemd service, the launchd agent and the wrapper scripts load this file before they start `vaultchron`. For manual runs, set the key in your shell.

## How a run works

By default VaultChron works in calendar days, not rolling 24-hour windows.

- With no date flags, it covers `[today - N, yesterday]`, where N is `scan.catch_up_days` or `-catch-up`. It goes oldest day first, across all repositories. Today is not generated in this mode.
- Existing notes are skipped. A note marked `partial: true` for a past day is regenerated. A partial note for today is skipped unless you pass `-force`.
- Use `-date` for one day, or `-from` and `-to` for a range, for example `vaultchron -from 2026-09-01 -to 2026-09-15`.
- When the call limit is reached, the remaining days are reported as `PENDING` and no LLM call is made. Run `vaultchron` again to continue.
- Days with no commits never produce a note, even with `-force`.
- Only the checked-out branch of each repository is read. Commits are matched by committer date in your local time.
- `-window` switches back to the older rolling-window mode. Agent session logs (`agent_logs.enabled`) are used only in that mode.

For each repository, a run does these steps:

1. Find git repositories under `scan.roots`, down to `scan.max_depth`, skipping `scan.excludes`.
2. Collect commit counts, authors, changed files and line counts with read-only git commands.
3. Build the diff. Lockfiles, minified files and vendor folders are dropped, credential-like text is removed, and the diff is cut at 25,000 characters.
4. Ask the LLM to write the note, trying the next model in `llm.waterfall` on HTTP 429.
5. Render the note.
6. Write it to a temporary file, then rename it into place, so a failed run never leaves half a note. Then update the index.

### Your existing vault

- Devlogs go to `<vault>/<projects_dir>/<repo>/Devlog/<date>.md`. The index and each project's `Overview.md` are created only if missing and are never overwritten. Nothing else in the vault is touched.
- The index is updated in place under a `## Recent Dev Logs` or `## Recent Activity` heading, newest first. An entry for the same project and day is replaced. The rest of your content is left alone.
- If the index exists but cannot be updated (for example, the heading is missing), the run stops before writing notes. Fix the index and run again.
- Real runs refuse to start without a valid config file.

### Rollup notes are not generated

Each note's top links include `Daily-Rollups/<date>`. VaultChron does not create that note, so the link points to nothing. `rollups_dir` and `recent_days` are reserved and have no effect.

## Scheduling on Linux and macOS

Windows is covered in [windows.md](https://github.com/ZeezyCodes/vaultchron/blob/main/docs/windows.md).

### Linux (systemd user timer)

```bash
mkdir -p ~/.config/systemd/user
curl -fsSLo ~/.config/systemd/user/vaultchron.service https://raw.githubusercontent.com/ZeezyCodes/vaultchron/main/deploy/vaultchron.service
curl -fsSLo ~/.config/systemd/user/vaultchron.timer https://raw.githubusercontent.com/ZeezyCodes/vaultchron/main/deploy/vaultchron.timer
systemctl --user daemon-reload
systemctl --user enable --now vaultchron.timer
```

The timer starts the service at 07:00 every day. If the machine was off then, systemd runs it at next start (`Persistent=true`). The service runs `~/.local/bin/vaultchron`, loads `~/.config/vaultchron/env` if it exists, and stops a run after 45 minutes. If you installed the program somewhere else, run `systemctl --user edit vaultchron.service` and set `ExecStart` (an empty `ExecStart=` line first, then yours).

The service passes no `-config`, so it uses the per-user config file. Logs: `journalctl --user -u vaultchron`. To run while you are logged out: `loginctl enable-linger "$USER"`.

To run from a git checkout instead of an installed binary, set `VAULTCHRON_HOME` to the checkout and use `deploy/vaultchron.sh`. It runs `./vaultchron -config "$VAULTCHRON_HOME/config.yaml"` and logs to `~/.config/vaultchron/logs/vaultchron.log`.

### macOS (launchd agent)

```bash
mkdir -p ~/Library/LaunchAgents
curl -fsSLo ~/Library/LaunchAgents/io.github.zeezycodes.vaultchron.plist https://raw.githubusercontent.com/ZeezyCodes/vaultchron/main/deploy/macos/io.github.zeezycodes.vaultchron.plist
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/io.github.zeezycodes.vaultchron.plist
```

To stop it:

```bash
launchctl bootout gui/$(id -u) ~/Library/LaunchAgents/io.github.zeezycodes.vaultchron.plist
```

The agent runs at 07:00 every day. It loads `~/.config/vaultchron/env` if it exists, adds `~/.local/bin`, `/usr/local/bin` and `/opt/homebrew/bin` to PATH, runs `vaultchron` with no `-config` (so it uses the per-user config file), and appends output to `~/.config/vaultchron/logs/vaultchron.log`.

CI checks the plist with `plutil -lint`. It has not been tried on real Mac hardware.
