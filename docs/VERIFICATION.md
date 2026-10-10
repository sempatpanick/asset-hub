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

## Disabled-backup browser fixture

Run the default four workflows with `pnpm run test:e2e --grep-invert "backups disabled"`. Then stop the disposable fixture gracefully and start it again with `BACKUP_ENABLED=false`, using the same dedicated test services. Run `pnpm run test:e2e --grep "backups disabled"`. This separate fixture omits the backup adapter and backup directory; the workflow performs a real upload, signed range download, deletion and disabled-restore UI check. CI performs both fixture runs. The no-backup Go integration test also covers checkpoint recovery across an ingestion restart, cleanup and primary corruption.

## Searchable controls and live refresh — 2026-10-10

All selection fields use the shared searchable popover, including project pickers, filters, page sizes, expiry, roles and asset visibility. Large authorized option collections render in batches of 30 as the user scrolls; searching covers the entire collection. Arrow keys, Enter, Escape and focus restoration work in page toolbars and modal dialogs. Projects and API keys use their complete authorized API collections for local filtering and paging. Files, uploads and activity filter on the server before applying limit/offset, so matching records beyond the current page remain searchable. Page sizes are 10, 25, 50 and 100, and changing a filter or size returns to the first page. Paging controls remain mounted through empty results and loading, preserving an open page-size popup when a filtered response arrives.

Header search submits an asset query to File Manager, accepts Enter or the search button, supports the `/` shortcut and remains available on mobile. Tables have name/text searches and appropriate status, role, action, type or project filters, including the overview's recent-assets table. Members also have name/email and role filters.

Visible resources refresh through sequential HTTP polling: uploads every three seconds, projects/files/activity/overview/keys/details/health every five seconds, and settings every thirty seconds. Successful mutations refresh mounted resources immediately. Hidden/offline pages pause requests; focus, visibility and reconnect resume them. Requests are aborted on navigation, bounded to fifteen seconds and retried with backoff after failures. Previously loaded rows stay visible during refresh and transient errors. Session expiry redirects to sign-in. The approach uses existing authorized HTTP endpoints and does not add a WebSocket server or migrations.

The two additional browser workflows create real projects/transfers/assets through the persistent API and verify dropdown scroll loading, keyboard selection, modal Escape, server searches beyond the first page, page sizes, external-client updates, sidebar storage, header search and recovery from network errors. A delayed real API response verifies that page-size selection survives the transition from empty results to rows. The mobile workflow also verifies searchable popover bounds and header search. Go integration assertions cover effective upload statuses, matching offsets and authorization when filtering uploads/audits. No production mock assets or counts are introduced.

Frozen pnpm installation, API generation, formatting, strict frontend/tooling type checks, zero-warning lint and the static production build passed. Backend formatting, explicit build, vet, unit/architecture tests and Linux ARM64 cross-compilation passed. The fresh PostgreSQL/Redis integration suite passed all five integration workflows and the runtime-role check, including the snapshot/offline-key drill.

The final six main Playwright workflows passed together against a fresh UTF-8 PostgreSQL 18.6 cluster on loopback port 55434 and the dedicated Redis service; the normal browser-preview helper accepts this alternate test port. The separate backups-disabled workflow passed against a second fresh fixture with `BACKUP_ENABLED=false`. The Docker-backed database initially passed the browser suite, but later container-build activity caused severe database latency and invalidated subsequent timeout results. The temporary native cluster uses only disposable fixture databases. Both browser fixtures shut down cleanly and the native server was stopped. Tool policy blocked recursive deletion of the stopped cluster; its files remain in the ignored `.local/controls-pg-c3fc3543a34645178cca131dd3d768c6` directory. No shared database or unrelated application was restarted.

At that verification run, full Docker source builds remained unverified. The backend image failed twice during `go mod download` with a TLS handshake timeout fetching `github.com/cespare/xxhash/v2@v2.3.0` from proxy.golang.org. The frontend image was stopped during its frozen dependency installation after prolonged registry/download latency (many tarballs reported 0–5 KiB/s) and fixture contention. Neither attempt was a passing image build; dependency and policy checks were not bypassed. The later successful frontend image build is recorded below; the backend image gap remains separate.

## Persistent workspace navigation — 2026-10-10

Authenticated pages now share the `(workspace)` layout and `WorkspaceShell`. The sidebar, header, footer, authenticated session and workspace project/settings/health resources stay mounted as navigation changes the page content. Public login and invitation pages retain the common root layout without the workspace shell. Route-group folders do not change public URLs. The shared shell updates active navigation and breadcrumbs from the current pathname; mobile navigation closes after choosing a page.

Dashboard pages consume the shared workspace context and own their filters, tables and dialogs. Header search remains mounted, preserving an unsubmitted draft across sidebar navigation. Submitted queries use the App Router's search parameters inside a page-content Suspense boundary, so searches work across routes and on File Manager itself without replacing the shell.

The new persistent-API browser regression verifies sidebar/header/storage DOM identity, preserved header drafts, active navigation, browser back/forward, changed and repeated search submissions on File Manager, sign-out and protected-route redirects. Across all seven workspace destinations it requires exactly one document navigation and one session load.

Frozen pnpm installation, formatting checks, frontend/tooling type checks, zero-warning lint and the static production build passed. All seven main Playwright workflows passed together, and the separate backups-disabled workflow passed against a fresh primary-only fixture. Both use disposable PostgreSQL/Redis-backed APIs. The fixtures and dedicated Redis service were shut down, and the reused native PostgreSQL server was stopped; its previously retained ignored cache was preserved.

The complete frontend Docker source build passed on Linux AMD64, including the unchanged frozen dependency layer, compilation/type checking/static export of the shared layout, and packaging into the pinned non-root Nginx runtime. The verification image is tagged `assethub-verification/web:navigation`. This is image-build evidence; running application services were not redeployed. Backend code and API contracts are unchanged by this navigation revision.

## Interactive API documentation — 2026-10-10

The workspace's API Docs menu opens the public `/api-docs/` reference in a new tab with `noopener noreferrer`. The reference covers all 35 backend HTTP operations, including liveness/readiness outside `/v1`, using the same contract offered as downloadable JSON and YAML. Plain-language guides cover server integration, sessions and CSRF, roles/scopes, pagination/search, resumable parts, processing, checksums, content/ranges, signed links, quotas, backup retention and error handling. The contract now includes concrete success/error examples and previously omitted response bodies, progress fields, pagination metadata and session-only security details. The generated TypeScript client was refreshed; file visibility selection retains its concrete private/public type.

The request console sends real same-installation requests with a dashboard session, an in-memory project key or anonymous access. It supports JSON, multipart files, raw part checksums, GET/HEAD/ranges, cancellation and bounded response previews. Live status/headers/body are distinct from illustrative samples. Credentials in generated cURL requests are placeholders. Sessions obtain CSRF tokens before mutations, while key/anonymous requests omit session cookies. It uses the existing runtime and CSP without a remote docs service or new backend handlers.

Pinned frozen pnpm installation, API/docs generation, formatting checks (including the generator and canonical YAML), strict frontend/tooling type checks, zero-warning lint, the static production build and all 12 deployment-helper tests passed. Backend explicit build, vet and unit/architecture checks passed; no Go backend code changed. The generator's read-only freshness check passed. Documentation maintenance is described in `docs/API.md`.

All nine main Playwright workflows passed together against a fresh disposable UTF-8 PostgreSQL 18.6 / Redis fixture. The new documentation workflows compare the reference to every backend route, visit every response example, verify downloadable contracts, endpoint search, mobile bounds and new-tab navigation, and exercise persistent session/key requests, project/key creation, multipart uploads, raw resumable parts, incomplete completion, checksum rejection, HEAD/ranges, forbidden requests, missing files and revoked keys. The documentation workflows reuse a real administrator session across isolated browser contexts to stay within the unchanged login rate limit; an earlier combined run had reached that limit on its final workflow. The separate backups-disabled workflow also passed against a fresh primary-only fixture. Both fixtures shut down cleanly; dedicated PostgreSQL and Redis services were stopped and the existing ignored PostgreSQL cache was retained.

After final response-example corrections, frontend formatting/type/lint/build checks passed again and both documentation workflows passed against another fresh persistent fixture. This includes endpoint-specific busy messages, accurate authentication Retry-After samples and the invalid file-type filter response. The final fixture and its dedicated services were stopped cleanly.

The complete frontend Docker source build passed on Linux AMD64 and packages the docs in the pinned Nginx runtime as user `101:101`, tagged `assethub-verification/web:api-docs`. The initial frozen dependency installation passed after approximately 25 minutes of registry/download latency; the final example update reused that verified dependency layer and passed compilation, type checking, static export and image packaging again. Inspection of a disposable, never-started verification container confirmed that the docs HTML exists and the packaged JSON/YAML hashes match the current canonical artifacts. The verification container was removed. This is build and artifact evidence; running application services were not redeployed.
