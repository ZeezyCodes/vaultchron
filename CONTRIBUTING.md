# Contributing to VaultChron

Thank you for your interest in contributing to VaultChron! We welcome contributions, bug reports, and suggestions.

## Development Setup

### Prerequisites

- **Go**: Version `1.27.1` or higher (per `go.mod`).
- **Git**: Ensure `git` is available in your `$PATH`.
- **Linter & Security Tools**:
  - `golangci-lint`: Version **`v2.14.0`** or higher.
    > [!IMPORTANT]
    > Go 1.27.1 produces export data format v4/v5. Versions of `golangci-lint` earlier than v2.14.0 bundle an older decoder that panics on Go 1.27 export data. Install `v2.14.0` via:
    > ```bash
    > go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0
    > ```
  - `govulncheck` (optional but recommended):
    ```bash
    go install golang.org/x/vuln/cmd/govulncheck@latest
    ```

### Cloning and Building

```bash
git clone https://github.com/ZeezyCodes/vaultchron.git
cd vaultchron

# Build binaries
go build ./...

# Run tests
go test -v ./...
```

## Branching & Commit Conventions

- **Branch Naming**: Create dedicated task/feature branches off `main` (e.g. `feat/my-feature`, `fix/issue-description`, `docs/clarify-config`).
- **Commit Messages**: Structure commits around single logical units of work following [Conventional Commits](https://www.conventionalcommits.org/):
  - `feat:` New features or capabilities
  - `fix:` Bug fixes and defect remediations
  - `refactor:` Code improvements without behavioral changes
  - `test:` Test additions or test refactoring
  - `chore:` Maintenance, dependency, or configuration tasks
  - `docs:` Documentation updates
- **AI-Assisted Contributions**: The repository includes an [AGENTS.md](AGENTS.md) specification detailing operational rules, token economy principles, and boundary isolation for automated and AI-assisted workflows. Human and AI contributors alike are encouraged to observe these operational boundaries.

## Pre-PR Checklist

Before opening a pull request, run the full validation suite locally:

```bash
# 1. Format check (must report zero unformatted files)
test -z "$(gofmt -l .)"

# 2. Go vet
go vet ./...

# 3. Compilation check (Linux and Windows cross-compilation)
go build ./...
GOOS=windows go build ./...

# 4. Unit and race tests
go test -race -cover ./...

# 5. Golangci-lint (v2.14.0+)
$(go env GOPATH)/bin/golangci-lint run ./...

# 6. Vulnerability scan (if installed)
$(go env GOPATH)/bin/govulncheck ./...
```

## Pull Request Expectations

- **Cross-Platform Compatibility**: CI builds and tests on Linux, macOS, and Windows. All changes must keep `GOOS=windows go build ./...` green.
- **Test Coverage**: Any new behavior or bug fix should include accompanying unit tests covering edge cases.
- **Scope Containment**: Keep changes focused on the task at hand. Avoid unrelated refactorings or cosmetic formatting changes across untouched files.
- **Clean Git History**: Avoid merge commits in PRs; rebase cleanly against the target base branch before submission.
- **Standard Library First**: Adhere to standard library implementations wherever possible (`os/exec`, `path/filepath`, `text/template`, `net/http`) before proposing new external dependencies.

## Releases (maintainers)

To publish a new release:
1. Ensure `main` is clean, all CI checks pass, and tests succeed locally.
2. Create an annotated git tag matching `vX.Y.Z` on `main` (e.g. `git tag -a v0.1.0 -m "Release v0.1.0"`).
3. Push the tag to upstream (`git push origin v0.1.0`).
4. The GitHub Actions release workflow automatically triggers, builds cross-compiled binaries, and generates a draft release with checksums via GoReleaser.
5. Review the draft release, verify the checksums, and publish the release.

The one-line install scripts (`install.sh` and `install.ps1`) live on `main` rather than in release assets. They depend directly on GoReleaser archive naming (`vaultchron_<version>_<os>_<arch>.tar.gz` and `.zip`) and `checksums.txt`; changing the release asset naming convention requires updating both installer scripts.
