# VaultChron

[![CI](https://github.com/ZeezyCodes/vaultchron/actions/workflows/ci.yml/badge.svg)](https://github.com/ZeezyCodes/vaultchron/actions/workflows/ci.yml)

VaultChron writes a daily devlog from your git history. It scans your local repositories, collects each day's commits and diffs, asks an LLM to write up the work, and saves the result as a Markdown note in your Obsidian vault. An Obsidian vault is just a folder of Markdown files.

## What a run produces

For each repository and each day that has commits, VaultChron writes one note, by default at `<vault>/Projects/<repo>/Devlog/<date>.md`. It also keeps an index note in the vault that links to recent devlogs. If the index or a project's `Overview.md` does not exist yet, it creates them. It never overwrites them.

Each devlog note has:

- front matter with the date, project, tags, commit count, model and generation time
- links back to the index and the project's Overview note
- a small facts block: commits, active branch, lines changed and the most-touched packages
- text written by the model in five Obsidian callouts (the highlighted boxes written as `> [!note]`): Architectural Evolution & Design Decisions, Data Contracts & Interface Shifts, Regressions & Defect Remediations, Immediate Action Items & Operational Checklists, and Verification & Test Suite Status

## Privacy

VaultChron sends data from your repositories over the network to the LLM service you configure. The default is Google Gemini. It sends commit messages, branch names, file statistics and code diffs.

Before sending, it removes private keys, AWS access keys, Bearer tokens and common password or token assignments. This is a best-effort check. It can miss secrets.

Do not run VaultChron on repositories that contain secrets or are under an NDA unless you use a local model. To do that, set `llm.base_url` to a local server such as Ollama or LM Studio. Reading logs from local AI coding agents is off by default (`agent_logs.enabled`).

## Install

Linux and macOS:

```bash
curl -fsSL https://raw.githubusercontent.com/ZeezyCodes/vaultchron/main/install.sh | sh
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/ZeezyCodes/vaultchron/main/install.ps1 | iex
```

With Go 1.27.1 or later, on any system:

```bash
go install github.com/ZeezyCodes/vaultchron/cmd/vaultchron@latest
```

The installers download the release archive for your system, check it against `checksums.txt`, and copy the programs into your user folder (`~/.local/bin` on Linux and macOS, `%LOCALAPPDATA%\Programs\vaultchron` on Windows). No administrator rights are needed. The checksum check catches corrupted downloads. It does not protect against a tampered release. You can read the scripts first: [install.sh](https://github.com/ZeezyCodes/vaultchron/blob/main/install.sh) and [install.ps1](https://github.com/ZeezyCodes/vaultchron/blob/main/install.ps1).

To upgrade, run the same command again. The Windows installer adds the install folder to your user PATH, so open a new terminal afterwards. The Linux and macOS installer does not change PATH. If the folder is not on your PATH, it prints the line to add. Archives for Linux, macOS (amd64, arm64) and Windows (amd64) are also on the [Releases page](https://github.com/ZeezyCodes/vaultchron/releases).

You need Git on your PATH and an LLM API key.

## First run

1. Copy the example config.

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

2. Open the file and set `vault.path` (your vault folder) and `scan.roots` (folders that contain your git repositories).
3. Set your API key. The default variable is `GOOGLE_API_KEY`:
   ```bash
   export GOOGLE_API_KEY="your-key"
   ```
4. Try a dry run. It skips the LLM, writes nothing and prints a preview of the notes:
   ```bash
   vaultchron -dry-run
   ```
5. Run it for real:
   ```bash
   vaultchron
   ```

With no date flags, VaultChron covers the 7 days before today. It makes at most 20 LLM calls per run. If it stops at that limit, run it again to continue. It writes only inside the vault's projects folder, plus the index. Back up your vault before the first run anyway.

## Run it every day

VaultChron can run on a schedule: a systemd user timer on Linux, a launchd agent on macOS, a Scheduled Task on Windows. All three default to 07:00. If your computer was off, the next run fills in the days it missed.

- [Linux and macOS scheduling](https://github.com/ZeezyCodes/vaultchron/blob/main/docs/configuration.md#scheduling-on-linux-and-macos)
- [Windows setup and scheduling](https://github.com/ZeezyCodes/vaultchron/blob/main/docs/windows.md)

## Platform support

Linux is the main platform and the one the maintainer uses every day. macOS (amd64, arm64) and Windows (amd64) builds are tested in CI on every commit, but the maintainer does not try them by hand for each release. If something does not work there, open an issue with your OS, the command you ran and the output.

## More

- [Configuration, flags and how a run works](https://github.com/ZeezyCodes/vaultchron/blob/main/docs/configuration.md)
- [Windows setup](https://github.com/ZeezyCodes/vaultchron/blob/main/docs/windows.md)
- [Contributing](https://github.com/ZeezyCodes/vaultchron/blob/main/CONTRIBUTING.md)
- [Security policy](https://github.com/ZeezyCodes/vaultchron/blob/main/SECURITY.md)
- License: MIT, see [LICENSE](https://github.com/ZeezyCodes/vaultchron/blob/main/LICENSE)
