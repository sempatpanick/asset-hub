# AssetHub agent instructions

## Required checks before completing work

For every frontend or backend code change, run formatting, type/compile checks,
linting and tests for each affected component. Fix failures before finishing.
Run the checks again after further edits. Preserve unrelated user changes.

Do not claim a check passed when it was skipped, unavailable or failed. If a
required runtime, dependency or fixture is unavailable, complete independent
checks and report the exact blocker and remaining validation. Do not bypass
checks, loosen type checking or disable lint rules solely to obtain a pass.

### Frontend and TypeScript tooling

Use the pinned pnpm release and frozen workspace lockfile.

1. Format changed supported files with `pnpm exec prettier --write <paths>`.
2. Run `pnpm run format:check`.
3. Run `pnpm run typecheck`.
4. Run `pnpm run lint`.
5. Run the relevant Playwright workflows with `pnpm run test:e2e`, using the
   disposable PostgreSQL/Redis-backed browser fixture described in
   docs/VERIFICATION.md. A test listing is not a passing test run.
6. Run `pnpm run build` when application code, dependencies or build configuration
   changes. Regenerate the API client with `pnpm run generate:api` after contract
   changes, then rerun the checks.

Tests must exercise real behavior and persistent APIs where appropriate. Never
replace required integration tests with production mock data.

### Backend (Go)

From services/backend:

1. Run `gofmt -w <changed Go files>` and verify `gofmt -l` returns no paths for
   those files. Use `go fmt ./...` when formatting the entire component.
2. Run `go build ./...` for compile/type validation.
3. Run `go vet ./...` for lint/static analysis.
4. Run `go test ./...` for unit and Clean Architecture boundary checks.
5. Run the PostgreSQL/Redis integration suite for persistence, authentication,
   authorization, migrations, queues, ingestion, content or recovery changes.
   Use only the dedicated test services; never production/shared databases.
   Run scripts/test-integration.ps1 on Windows, or configure TEST_DATABASE_URL
   and TEST_REDIS_URL for the integration suite. Use `go test -count=1` for
   database-backed tests so cached results cannot stand in for a fresh run.
6. Use supported race checks for concurrency changes and ARM64 cross-compilation
   for changes affecting deployment portability. Report unavailable checks.

Go tests compile and type-check Go code, but do not substitute for formatting,
the explicit build or go vet.

### Shared and deployment changes

Run both sets of applicable checks when a change affects both components.
Validate changed Compose/configuration/scripts and verify affected Docker builds.
For CI/deployment edits, also run `pnpm run test:deploy`, actionlint, shell
formatting and ShellCheck on changed shell helpers. Inspect actual workflow
results after publishing; configuration validation is not deployment evidence.
Never restart unrelated applications to make a check pass. Keep .env, keys,
tokens, local runtime data and test credentials out of version control.

## Completion report

Summarize what changed, formatting/typecheck/lint/test results, remaining blockers
and any required user decision. Distinguish successful tests from skipped tests,
test discovery and configuration-only validation.
