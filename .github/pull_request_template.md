## Summary
<!-- Briefly explain the purpose and high-level changes in this pull request. -->

## Changes Made
<!-- Detail the key modifications, architectural choices, and fixes. -->
- 

## Pre-Submission Verification
Please ensure all local verification checks pass prior to opening this PR:

- [ ] Code is formatted with `gofmt -l .` (no drifted files)
- [ ] `go vet ./...` reports zero issues
- [ ] `go build ./...` compiles cleanly
- [ ] `go test -race -cover ./...` passes all tests
- [ ] `golangci-lint run ./...` (v2.14.0+) passes with 0 issues
- [ ] Accompanying unit tests have been added or updated
- [ ] Commits follow Conventional Commits formatting (`feat:`, `fix:`, `refactor:`, `test:`, `chore:`, `docs:`)
- [ ] No secrets, credentials, or personal paths are introduced
