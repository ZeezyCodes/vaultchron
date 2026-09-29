# Security Policy

## Supported Versions

VaultChron is under active development. Security updates and bug fixes are applied to the latest release on the primary development branch (`main`).

| Version | Supported |
| :--- | :--- |
| Latest release / `main` | :white_check_mark: |
| Older releases | :x: |

## Reporting a Vulnerability

If you discover a security vulnerability or potential threat in VaultChron, please report it responsibly using GitHub's Private Vulnerability Reporting:

1. Navigate to the repository's **Security** tab on GitHub: [Security Advisories](https://github.com/ZeezyCodes/vaultchron/security/advisories/new).
2. Click **Report a vulnerability** to open an advisory draft.
3. Provide a clear description of the vulnerability, including reproduction steps, affected versions, and potential impact.

Please do not open public GitHub issues or discussions for undisclosed security vulnerabilities.

### Response Timeline

- **Initial Acknowledgment**: Within 48 hours of report receipt.
- **Assessment & Triage**: Within 5 business days, including confirmation of severity and impact.
- **Remediation & Advisory Release**: A coordinated fix and public advisory will be published upon patch verification.

---

## Known Behavior: Network Transmission of Git Diffs & Metadata

> [!IMPORTANT]
> **Expected Operational Behavior (Not a Security Vulnerability):**
>
> By design, VaultChron extracts git diffs, commit logs, author telemetry, and project structures from local repositories and transmits this data across the network to configured LLM APIs (Google Gemini, OpenAI, OpenRouter, or other specified endpoints) to generate devlogs.
>
> - **Diff Redaction is Best-Effort:** VaultChron executes a regular expression sanitization pass over unified diffs prior to network transmission, stripping common credential signatures such as PEM private keys, AWS access keys, Bearer tokens, and assignment secrets (see [internal/collector/git.go](internal/collector/git.go)). **This redaction is heuristic-based and does not provide a formal guarantee that all sensitive patterns will be captured.**
> - **User Responsibility:** Users must ensure that local repositories analyzed by VaultChron do not contain sensitive uncommitted secrets or credentials. For air-gapped or confidential environments, point `llm.base_url` to a local model server (such as Ollama or LM Studio) rather than a cloud-hosted API.
>
> Reports concerning the intentional network transmission of diffs or the inability of regex heuristics to catch obfuscated/non-standard secrets will be closed as expected behavior.
