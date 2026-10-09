# Verification and development fixtures

## Commands

At the repository root: `npm ci`, `npm run generate:api`, `npm run typecheck`, `npm run build`, `npm audit --audit-level=high`, `npm run test:e2e`.

From services/backend with Go 1.27.2: `go test ./...`, `go vet ./...`. Architecture tests reject outward/framework dependencies from domain/application. API, administrator and worker binaries cross-compile with `CGO_ENABLED=0 GOOS=linux GOARCH=arm64`.

## Persistent integration tests

Use an isolated PostgreSQL and Redis instance, never production credentials. Each test creates a uniquely named database, applies the actual migrations and removes its database afterward. The administrator test additionally provisions the runtime role inside that dedicated test cluster and verifies CRUD/sequence access and denied schema changes/elevated privileges.

Windows: start `deploy/compose.test.yaml` using ignored `.local/test.env` containing TEST_POSTGRES_PASSWORD, then run `scripts/test-integration.ps1 -Port 55433`. Redis is on loopback port 56380. The helper uses optional PostgreSQL client executables under `.local/pg-runtime/pgsql/bin` for the actual custom-format dump/restore drill. Without those binaries the drill explicitly skips; do not claim it passed.

Coverage includes persistent authentication, CSRF, invitations, project isolation, membership, scoped/revoked keys and audit; immutable resumable parts/checksums, incomplete transfers, quota/cancellation, Redis queue loss and durable recovery, worker lease expiration, checkpoint retries, missing/corrupt storage; private/public/signed access, expiry/version revocation, cross-chunk/suffix/HEAD/416 ranges, backup fallback, tombstone deletion, remote purge, retained restore and retention expiry. Telegram tests exercise the real HTTP transport against a controlled protocol server, including ambiguous sends, sanitized errors and retry_after. They do not contact a live bot/channel.

The snapshot drill creates a ready encrypted asset, runs real pg_dump/pg_restore into a new database, reads identical bytes with the primary disconnected and the offline master key, and rejects the wrong key. A separate fingerprint check rejects accidental replacement of the installation wrapping key.

## Browser fixture

Build the dashboard first. `scripts/browser-preview.ps1` serves the real static dashboard/API on loopback port 18080, using a disposable PostgreSQL database, Redis database 12 and restricted temporary staging/Local/backup/cache directories. The fixture uses the development Local provider and explicitly permits the shared Windows filesystem. Its known account/password are test-only and the fixture is excluded from release binaries. It does not seed fake assets or counts.

Three Playwright workflows cover desktop projects/keys/audits, mobile navigation/dialog/filters, and multi-file resumable uploads through the worker with persisted ready status after reload, signed byte-range retrieval, rename/delete/restore and link invalidation. Screenshots/reports stay in ignored `.local` and test-results directories.

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
