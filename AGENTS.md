# AGENTS.md — Contributor & Agent Operational Guidelines

## Overview & Architecture

VaultChron crawls configured Git project repositories, harvests commit telemetry and churn over a configurable time window, and prompts an LLM to generate structured devlog notes adhering to the Obsidian Callout v3 taxonomy. It automatically maintains an interactive devlog index (`00-Dev-Index.md`) with linked recent rollups and project directories.

### Package Layout
- `cmd/vaultchron`: Main entry point and run-to-completion CLI command for repository scanning, devlog generation, and vault indexing.
- `cmd/vaultchron_migrate`: Standalone CLI migration tool for retrofitting notes to the v3 callout taxonomy.
- `internal/collector`: Git repository scanner and telemetry collector (commits, churn, diff summaries, and optional agent session metadata).
- `internal/config`: YAML configuration parsing, environment resolution, and schema validation.
- `internal/llm`: Provider-agnostic OpenAI-compatible HTTP client (`/chat/completions`) with configurable `base_url` and waterfall fallback chain.
- `internal/pipeline`: Orchestration engine coordinating telemetry collection, prompt assembly, LLM synthesis, and vault writing.
- `internal/vault`: Obsidian vault manager, note rendering, index maintenance, and atomic file operations.

---

## Build, Test & Lint Commands

The project requires Go **1.27.1** (as specified by `go 1.27.1` in `go.mod`).

All contributors and automated agents must verify changes against the full suite:

- **Build:** `go build ./...`
- **Race-Enabled Tests:** `go test -race ./...`
- **Go Vet:** `go vet ./...`
- **Formatting:** `gofmt -l .` (must report zero unformatted files)
- **Vulnerability Audit:** `govulncheck ./...`
- **Linter:** `golangci-lint run ./...`
  - *Requirement:* `golangci-lint` version **`v2.14.0`** or higher is required (Go 1.27.1 produces export data format v4/v5, which causes older linter decoders to panic).
  - *Installation:*
    ```bash
    go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0
    ```

---

## Code Conventions

- **Stdlib-First Grain:** Rely on Go standard library packages (`os/exec`, `path/filepath`, `text/template`, `syscall`, `net/http`) wherever feasible. Avoid introducing third-party Go modules without explicit justification.
- **GoDoc Documentation:** All exported packages, types, interfaces, functions, and methods must have descriptive GoDoc comments.
- **Context-Bound Process Execution:** Always execute subcommands with `exec.CommandContext` and reasonable bounded timeouts to prevent hanging processes.
- **Atomic Vault Operations:** Target vaults contain live user documentation. File writes to notes and index files must be atomic (e.g. write to a temporary file in the same filesystem and rename) to eliminate corruption risks.
- **UTF-8-Safe Truncation:** Commit diffs and LLM payloads must be truncated on rune boundaries rather than raw byte slices to avoid broken UTF-8 sequences.

---

## Commit & Pull Request Conventions

- **Conventional Commits:** Structure commit messages around single logical units of work (`feat:`, `fix:`, `refactor:`, `test:`, `chore:`, `docs:`).
- **Atomic PRs:** Keep pull requests focused on a single change or feature branch.
- **Test Requirements:** All bug fixes, behavior-changing logic, and new features must include accompanying automated tests.
- **No Version Tags or Release Triggers:** Contributors and automated agents must never create or push version tags or releases (tags matching `v*` trigger the release workflow).

---

## Operational Boundaries & Token Economy

AI agents and contributors operating in this repository should adhere to standard operational boundaries:

- **Scope Containment:** Touch only the files designated for the current task. Do not execute sweeping reformatting or unsolicited refactors outside the task scope.
- **Proportional Diffs & Grepping:** Provide targeted, proportional diffs and bounded searches rather than dumping entire files or unbounded logs.
- **Concise Reporting:** Report test and verification outcomes as concise pass/fail summaries.
- **Obsidian Callout v3 Compliance:** Devlog templates and generated notes must adhere strictly to the Obsidian Callout v3 taxonomy (`[!note]-`, `[!abstract]`, `[!info]`, `[!bug]`, `[!warning]`, `[!check]`) with no raw markdown headings (`#`, `##`, `###`) inside blockquote callout bodies.

---

## Safety Rules for Contributors and AI Agents

- **No Secrets:** Never commit API keys, personal access tokens, or sensitive credentials. Secrets must be routed through environment variables or local, untracked configuration files.
- **No Real Names or Local Paths:** Do not introduce real personal names, employer names, proprietary project names, or local absolute filesystem paths into code, fixtures, templates, or documentation. Use neutral placeholders (e.g. `AcmeWidgets.com`, `sampleAuth`, `sampleCMS`, `test-org/demo-repo`).
- **Preserve Secret Redaction:** Do not weaken, bypass, or remove existing secret detection or redaction routines.
- **Agent Logs Default:** The `agent_logs.enabled` configuration setting must remain `false` by default.