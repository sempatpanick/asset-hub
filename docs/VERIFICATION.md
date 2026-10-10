# Verification and development fixtures

## Commands

At the repository root with pnpm 12.10.1: `pnpm install --frozen-lockfile`, `pnpm run generate:api`, `pnpm run format:check`, `pnpm run typecheck`, `pnpm run lint`, `pnpm run build`, `pnpm audit --audit-level=high`, `pnpm test`. Apply frontend/tooling formatting with `pnpm run format`. Type checking covers the dashboard, API client and Playwright tests/configuration. Historical npm results below describe the pre-migration implementation.

From services/backend with Go 1.27.2: `gofmt -w <changed Go files>`, verify `gofmt -l .` is empty, then `go build ./...`, `go vet ./...`, `go test ./...`. Architecture tests reject outward/framework dependencies from domain/application. API, administrator and worker binaries cross-compile with `CGO_ENABLED=0 GOOS=linux GOARCH=arm64`. AGENTS.md makes these checks mandatory for affected code changes. The combined `sh scripts/test.sh` also runs the frontend browser suite; start its disposable fixture beforehand. Database-backed Go tests require the integration environment described below; otherwise those tests explicitly skip.

## Persistent integration tests

Use an isolated PostgreSQL and Redis instance, never production credentials. Each test creates a uniquely named database, applies the actual migrations and removes its database afterward. The administrator test additionally provisions the runtime role inside that dedicated test cluster and verifies CRUD/sequence access and denied schema changes/elevated privileges.

Windows: start `deploy/compose.test.yaml` using ignored `.local/test.env` containing TEST_POSTGRES_PASSWORD, then run `scripts/test-integration.ps1 -Port 55433`. Redis is on loopback port 56380. The helper uses `go test -count=1` to force fresh database-backed test execution and optional PostgreSQL client executables under `.local/pg-runtime/pgsql/bin` for the actual custom-format dump/restore drill. Without those binaries the drill explicitly skips; do not claim it passed.

Coverage includes persistent authentication, CSRF, invitations, project isolation, membership, scoped/revoked keys and audit; immutable resumable parts/checksums, incomplete transfers, quota/cancellation, Redis queue loss and durable recovery, worker lease expiration, checkpoint retries, missing/corrupt storage; private/public/signed access, expiry/version revocation, cross-chunk/suffix/HEAD/416 ranges, backup fallback, tombstone deletion, remote purge, retained restore and retention expiry. Telegram tests exercise the real HTTP transport against a controlled protocol server, including ambiguous sends, sanitized errors and retry_after. They do not contact a live bot/channel.

The snapshot drill creates a ready encrypted asset, runs real pg_dump/pg_restore into a new database, reads identical bytes with the primary disconnected and the offline master key, and rejects the wrong key. A separate fingerprint check rejects accidental replacement of the installation wrapping key.

## Browser fixture

Build the dashboard first. `scripts/browser-preview.ps1` serves the real static dashboard/API on loopback port 18080, using a disposable PostgreSQL database, Redis database 12 and restricted temporary staging/Local/backup/cache directories. The fixture uses the development Local provider and explicitly permits the shared Windows filesystem. Its known account/password are test-only and the fixture is excluded from release binaries. It does not seed fake assets or counts.

Four Playwright workflows cover invitation acceptance with URL-token removal and persistent sign-in, desktop projects/keys/audits, mobile navigation/dialog/filters, and multi-file resumable uploads through the worker with persisted ready status after reload, signed byte-range retrieval, rename/delete/restore and link invalidation. Screenshots/reports stay in ignored `.local` and test-results directories.

## Results — 2026-10-09, release 0.3.0

- Normal npm peer resolution, generated OpenAPI declarations, strict dashboard type checking and the static production build passed. The root generator uses TypeScript 5.9.3 for its declared peer support; the dashboard uses 7.0.2.
- npm audit reported zero vulnerabilities in the resolved tree.
- Go unit tests, Clean Architecture dependency checks and go vet passed.
- Bash syntax checks passed separately for host preparation, database backup, isolated restore and the verification script.
- A real HTTP server test verifies that actively flowing content outlives the ordinary response timeout; content Range/recovery integration passed after this change.
- API/admin/worker cross-compilation for Linux ARM64 passed.
- Final real PostgreSQL 18.6 / Redis 8.10.2 suite passed with all four migrations. The snapshot/offline-key recovery and restricted runtime-role drill passed.
- Three Playwright workflows passed against the persistent API. Desktop/mobile screenshots were inspected.
- Compose configuration validates. Pinned base-service manifests support Linux ARM64. Complete image build/runtime results are recorded below after the local deployment check.
- An earlier Linux race attempt used a live host source mount while implementation was still changing and failed compilation. It is not valid race evidence. Native ARM64 execution and endurance remain external release gates.

## Container validation

The standard build passed normal npm peer resolution and audit inside the Node image after correcting the generator compatibility and workspace version. It was stopped during subsequent build steps because the local Docker engine exhibited severe filesystem latency (one image layer took 485 seconds and npm ci took 14 minutes). Concurrent extraction/build caused fixture database requests to time out and a later browser rerun remained stuck saving its project. Do not substitute those failed/stopped attempts for a successful complete Docker source build.

A separate runtime check packaged the final locally cross-compiled Linux API/admin/worker binaries into the exact pinned non-root distroless image successfully. The static-dashboard runtime image attempt stalled copying files, and Compose stalled creating its PostgreSQL/Redis containers. Those clients were stopped. Complete Compose startup, Linux race checks and the full source build remain unverified on this host. Existing unrelated services were not restarted. The recovery scripts have syntax and database/key/byte-recovery evidence, but their operator mount/timer workflow must still be exercised on Linux.

The final browser fixture was shut down through its test-only loopback cleanup route. All verification build/startup clients were stopped. The verification network and its delayed Created PostgreSQL/Redis containers were removed, along with the test Redis container. Only `assethub-test-postgres-1` could not be removed: Docker returned “tried to kill container, but did not receive an exit event.” Its test network also remains. Remove that exact disposable container/network once the engine recovers. Unrelated services were not restarted. Ignored screenshots, build outputs and test configuration remain available locally.

## Required production gates

Live Telegram credentials/channel; independently mounted physical backup and disappearance/recovery testing; native Orange Pi ARM64 execution and sustained resource measurements; HTTPS/tunnel/cache configuration; operator-run recovery using protected configuration/offline keys. Cross-compilation, protocol-server tests and a development Local provider do not satisfy these gates. S3/R2/MTProto and automated orphan-manifest recovery are future work.

## Port configuration update — 2026-10-09

Host defaults are frontend 3005, API 8085, PostgreSQL 5435 and Redis 6385. Nginx/Go listen on 3005/8085; PostgreSQL/Redis keep internal ports 5432/6379. The environment template, local private environment, Compose mappings, Next development port, Docker declarations, reverse proxy, API/web health checks and tunnel instructions were updated together.

Resolved Compose JSON confirms all four loopback bindings and matching proxy/listener configuration. Go command-package checks/vet, frontend type checking and whitespace checks passed. Services were not restarted. Local port 8085 is already published by discord-moderation-bot-worker-1, so simultaneous startup on this Windows host needs that conflict resolved. Target Orange Pi port availability still needs verification.

## Root Compose entry — 2026-10-09

The standard command previously failed from the repository root with “no configuration file provided: not found.” A root compose.yaml now includes deploy/compose.yaml. Both the root and explicit deployment commands validate, and their fully resolved service models match, including environment interpolation and build paths. The installed Compose version is 5.5.1; include requires 2.20+. This configuration-only check does not imply successful local startup: the current private environment uses production settings, its configured host storage paths/backup marker are absent on Windows, and host API port 8085 is occupied by the Discord worker. No application services were restarted or external tunnel settings changed.

## Worker port conflict resolved — 2026-10-09

At the user's request, the Discord moderation bot's worker host mapping was changed to 127.0.0.1:8095, keeping internal port 8081. Its private environment, example environment and Compose default were updated. The worker was recreated using its existing image. A Docker container-name collision was repaired by removing only the stopped predecessor and renaming/starting the verified replacement. The resulting worker is healthy and GET http://127.0.0.1:8095/ready returns 200. A local listener check confirms 8085 is free for AssetHub. Missing local storage configuration remains a separate startup prerequisite.

## Frontend pnpm migration — 2026-10-10

The frontend/tooling workspace now uses pnpm 12.10.1, verified against the package registry and pinned in package.json. pnpm-workspace.yaml declares apps/web and packages/api-client, strict peer resolution and native build-script approvals. The API client uses workspace:* local linking. The existing npm lockfile was imported to pnpm-lock.yaml and retired; application package versions were preserved. Old dependency directories and the old lockfile remain only in ignored .local backups.

The verification script, frontend Dockerfile and current setup commands use pnpm. npm only bootstraps the exact pnpm tool in the Node build image or on a machine without pnpm. The OpenAPI generator remains on supported TypeScript 5.9.3 while dashboard checks use 7.0.2; both versions were confirmed directly.

Frozen-lockfile workspace installation, repeated frozen installation, generated API declarations, frontend type checks, the static production build, dependency audit, formatter checks, script syntax and Compose configuration passed. pnpm audit reported no known vulnerabilities. Generated API declarations and backend source have no changes. Playwright successfully discovers the three existing browser workflows; they were not rerun as part of this tooling migration.

The full frontend multistage Docker source build passed on Linux AMD64, including frozen pnpm installation, native Linux-musl dependencies and Next.js compilation/export. This resolves the previous frontend-image build gap; full-stack startup and native Orange Pi release gates remain separate.

The runtime check found that Nginx's default FastCGI/uWSGI/SCGI temporary paths attempted to create root-owned cache directories. All three are now configured under the writable /tmp mount. Nginx configuration validation passed as the non-root image user with a read-only filesystem and dropped capabilities. An isolated container served /login/ on port 3005 after a bounded startup wait, and the verification containers were removed afterward. Running application services were not redeployed.

## Mandatory agent quality checks — 2026-10-10

AGENTS.md now requires formatting, type/compile validation, lint/static analysis and actual tests for frontend and backend changes. Frontend scripts provide format, format:check, lint and test; typecheck covers application code, API client and browser tooling. ESLint uses the compatible Next.js configs with zero permitted warnings. scripts/test.sh runs the browser suite and Go formatting/build/vet/tests plus ARM64 cross-compilation, with the dependency audit last. Start the dedicated browser fixture before using the combined script.

The initial lint run identified redundant synchronous effect state updates, legacy navigation, image-preview warnings and unused imports. Resource loading now derives pending state from the request identity and ignores aborted responses. API-key project defaults are derived without a state-update effect. Invitation tokens use a component-owned browser store and are removed from the URL after capture. Navigation uses the Next.js router; authenticated previews use unoptimized Next.js images so the browser sends its session cookie directly to the content endpoint.

- Frozen dependency installation and regenerated API declarations passed; generation produced no contract diff.
- Frontend/tooling formatting, dashboard and tooling type checking, zero-warning ESLint and the static production build passed.
- The complete frontend Docker source build passed on Linux AMD64 with frozen pnpm installation and the new lint/tooling dependencies. Compose configuration also validates.
- All four Playwright workflows passed against the disposable PostgreSQL/Redis API. The new invitation workflow verifies token removal, account creation and sign-in persistence. Upload coverage also verifies private PNG previews in grid/details views.
- Backend gofmt verification, go build ./..., go vet ./... and go test ./... passed. All five persistent PostgreSQL/Redis integration tests passed, including the real snapshot/offline-key and runtime-role drills.
- The combined shell script passed syntax validation; its checks were executed separately on Windows. The browser fixture shut down cleanly and its dedicated containers/network were removed. Unrelated services were not restarted.

The full dependency audit currently fails on one high-severity development-tool dependency: eslint-config-next → @next/eslint-plugin-next → fast-glob → micromatch → braces 3.0.3. [GHSA-vfj7-8cjw-p6xm](https://github.com/advisories/GHSA-vfj7-8cjw-p6xm) lists no patched release, also confirmed by the package registry. The production-only dependency audit passes. The finding is retained and the audit gate remains enabled; the previous zero-vulnerability results describe the dependency tree before ESLint was added.

## GitHub Actions implementation — 2026-10-10

CI and Deploy workflows now verify both components before publishing native ARM64 backend/web images to GHCR. Deployment targets /srv/projects/asset-hub over the Discord bot's existing Cloudflare Access SSH pattern, with a fixed AssetHub Compose project, staged immutable manifests, independent-backup and Docker-port checks, explicit migration completion and health-gated promotion. Secrets stay in the protected server environment. docs/GITHUB_ACTIONS.md records setup and failure/recovery behavior.

Local Prettier/type checks/ESLint, production build, frozen installation and production dependency audit passed. Backend formatting/build/vet and unit/architecture checks passed with cached results; database/race/browser checks are mandatory in the new real-service Linux workflow. Nine deployment tests passed, covering immutable refs, changed-source hashing, project isolation, port/backup guards, failed migrations, promotion ordering and SSH argument injection. shfmt 3.14.1, ShellCheck 0.11.0, actionlint 1.7.12, shell syntax and both Compose models validate. The production override removes all application build definitions. These local checks do not claim a successful remote deployment; inspect the GitHub run and server prerequisites.
