# Implementation phases and evidence

## Phase 0 — design

Create the five design documents before code. Establish Clean Architecture dependency direction, storage limitations, key recovery, API contract and latest-stable dependency policy. Status: documentation created; detailed contract evolves with implemented routes.

## Phase 1 — foundation and dashboard

Composition roots, migrations, PostgreSQL repositories, secure identity and invitations, projects/memberships, audit, scoped keys, real overview and health, responsive sidebar/pages, Docker Compose. Unimplemented upload/storage controls must say unavailable rather than simulate success. Gate: builds, import-boundary checks, unit/integration tests and browser verification.

Status: foundation implemented and verified with real PostgreSQL 18.6 integration tests, desktop/mobile Playwright workflows, production static build, type checks, Go tests/vet, npm audit and ARM64 cross-compilation. See VERIFICATION.md for current container evidence and DEPENDENCIES.md for the TypeScript generator compatibility exception. PostgreSQL uses typed explicit SQL mappings; sqlc generation remains optional rather than leaking generated database types into domain entities.

Phase 1 established the persistent foundation. Phases 2 and 3 have now been implemented; phase 4 software tooling is delivered with external production release gates still pending. No additional architectural approval is currently required.

## Phase 2 — ingestion

Resumable sessions, validation/quotas, restricted staging, outbox, Redis queue/worker, crypto, Telegram and Local providers, independent backup verification, retry/cancel/progress. Gate: partial retries, crash recovery, provider failures, backup absence and byte integrity.

Status: implemented with PostgreSQL reservations/outbox/leases, Redis queue, restricted staging, per-asset AES-GCM keys, encrypted private references, hosted-compatible Telegram transport, explicitly development-only Local provider, verified backup chunks/manifests, retries, cancellation, cleanup and multi-file browser progress. Real PostgreSQL/Redis tests cover queue loss, partial retries, corrupted bytes, absent backup, quota/cancellation and project/key scope. No live Telegram credentials were supplied.

## Phase 3 — content and management

Cache/fallback, Range/HEAD/content, public/private/signed access, previews, grid/list/search/filter, rename/delete/restore. Gate: denied cross-project access, key/link revocation, chunk-boundary ranges and restore.

Status: implemented with bounded encrypted cache and single-flight fetches, authenticated decryption, backup fallback, full/single-range/HEAD content, safe image/video previews, public/private and expiring signed capabilities, metadata filters, rename, tombstone deletion, best-effort remote purge, administrator restore and 30-day retention cleanup. Integration and browser workflows verify persisted management actions and byte ranges/recovery.

## Phase 4 — operations

Pinned ARM64 images, tunnel, resource budgets, backup/recovery scripts, capacity/queue health, dependency checks, target endurance and real Telegram verification. Gate: restore drill and native ARM64 execution. Operator credentials and hardware are external prerequisites; do not mark these gates complete based on mocks.

Status: operational software delivered: API/admin/worker images, persistent volumes, filesystem preparation, resource/health controls, limited runtime database role, wrapping-key fingerprint guard, snapshot/checksum and isolated database-restore scripts, recovery runbook and automated PostgreSQL dump/key/backup drill. Container and native hardware evidence is reported separately in VERIFICATION.md. Production sign-off still requires live Telegram, the physical backup mount, HTTPS/tunnel and native Orange Pi endurance/recovery validation. Automated import of orphan recovery manifests and S3/R2/MTProto providers remain future work.

## Phase reports

Every report records completed behavior, exact checks/results, limitations, remaining work and changed architectural decisions. Keep source and documentation in sync. Test fakes are confined to tests. Development Local storage is identified prominently and never claimed to be production Telegram.

## Dependency policy

Resolve latest stable direct packages from npm/Go module registries at implementation time. Pin package versions, go.sum and npm lockfiles. Resolve official container stable releases, verify linux/arm64 manifests and pin digests. Record verification date, exceptions and vulnerability review. Avoid prereleases. Upcoming security patches must be reviewed again before public deployment.
