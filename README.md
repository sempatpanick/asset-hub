# AssetHub

A self-hosted asset workspace with Clean Architecture: Next.js/TypeScript/Tailwind dashboard, Go API and workers, PostgreSQL metadata, Redis background execution, encrypted Telegram storage and an independent encrypted backup.

## Implemented

Invited accounts, secure sessions/CSRF, project roles, scoped/revocable keys, resumable multi-file uploads, quota/staging reservations, durable jobs and chunk checkpoints, Telegram and development Local providers, verified encrypted backups, authorized/public/signed content, video byte ranges, file search/type/status/project filters, grid/list previews, rename/delete/administrator restore, audit and detailed health. Dashboard data comes from persistent APIs; no example assets or counters are seeded.

The implementation and automated checks are complete through the ingestion/content phases. Operational tooling is included. Production sign-off still requires the live Telegram channel, independent physical backup storage, HTTPS/tunnel configuration, native Orange Pi execution and an operator-run endurance/recovery drill. Read docs/IMPLEMENTATION_PHASES.md and docs/VERIFICATION.md for evidence and remaining gates.

## Run on ARM64 Linux

1. Copy `.env.example` to `.env`. Generate distinct 64-character hexadecimal secrets with `openssl rand -hex 32` for POSTGRES_PASSWORD, APP_DATABASE_PASSWORD, REDIS_PASSWORD, ASSET_MASTER_KEY and SIGNED_URL_KEY. Set PUBLIC_ORIGIN to your HTTPS origin. Keep the real file private and store recovery keys offline.
2. Mount the SSD under `/srv` and an independent backup filesystem at `/mnt/assethub-backup`. Run `sudo bash scripts/prepare-host.sh /srv/assethub /mnt/assethub-backup`. Set matching DATA_ROOT/BACKUP_PATH. The helper verifies the mount and prepares restricted UID/GID 65532 directories.
3. Create a Telegram bot with posting/deletion rights in a private storage channel. Set TELEGRAM_BOT_TOKEN and TELEGRAM_CHANNEL_ID in `.env`.
4. Run `docker compose up --build -d` from the project root. The root compose.yaml includes deploy/compose.yaml and loads your root `.env`; Docker Compose 2.20+ is required. The explicit `docker compose --env-file .env -f deploy/compose.yaml up -d --build` command also works. The migrator applies schemas and provisions the limited runtime database role; owner credentials are absent from API/worker containers.
5. Run `docker compose --env-file .env -f deploy/compose.yaml run --rm api /app/admin bootstrap` and enter your administrator credentials interactively. No default production account exists.
6. Route HTTPS traffic through your existing Orange Pi tunnel to `http://127.0.0.1:3005` when cloudflared runs directly on the host. Leave TUNNEL_TOKEN empty and use the normal Compose command above. If you choose AssetHub's optional tunnel container instead, set TUNNEL_TOKEN, add `--profile tunnel`, and route it to `http://web:3005`. Bypass caching for `/v1/*`.

Default host ports: frontend **3005**, backend **8085**, PostgreSQL **5435**, Redis **6385**. Configure them with HTTP_PORT, API_PORT, POSTGRES_PORT and REDIS_PORT. Backend/database/cache bindings are localhost-only. PostgreSQL/Redis keep their standard internal container ports (5432/6379); inter-container connections use their service names. The dashboard sends `/v1` requests through its Nginx proxy to `api:8085`.

Production requires HTTPS cookies, the same independent backup filesystem checks, and at least a 10% free-disk reserve. The wrapping-key fingerprint prevents accidental key replacement: keep the original key or perform a deliberate rewrap migration.

For local development only, use APP_ENV=development, STORAGE_PROVIDER=local and an HTTP localhost origin. Local storage is explicitly identified as development. A test backup marker is still required. Development can lower DISK_RESERVE_PERCENT on a shared test disk; it does not establish production backup independence.

## API usage

All applications call AssetHub. Authenticate with `Authorization: Bearer <project-key>`; browser sessions are same-origin cookies with X-CSRF-Token for mutations. Never put application keys in URLs or frontend source.

Small files: multipart POST /v1/assets with project_id and file, up to 8 MiB. Larger files: POST /v1/uploads, PUT exact 8 MiB parts with X-Content-SHA256, then POST /v1/uploads/{id}/complete. A 202 response means queued, not stored: wait for asset status ready. Content is GET/HEAD /v1/assets/{id}/content. Authorized signers can issue temporary links; project managers control public visibility. Private URLs require authorization.

The full contract is api/openapi.yaml and docs/API_SPEC.md. Detailed deployment, backup and recovery instructions are in docs/OPERATIONS.md.

## Development and verification

GitHub Actions verifies frontend/backend changes, publishes native ARM64 images to private GHCR packages and deploys to `/srv/projects/asset-hub` over the existing Cloudflare Access SSH connection. See [docs/GITHUB_ACTIONS.md](docs/GITHUB_ACTIONS.md) for repository secrets, one-time server preparation and release/recovery behavior. Main pushes deploy only after all required checks pass.

For deployments built and verified on your own PC, use `pnpm run deploy:local`. It transfers ARM64 images directly over SSH and handles the Orange Pi rollout without Actions minutes or registry credentials. See [docs/LOCAL_DEPLOYMENT.md](docs/LOCAL_DEPLOYMENT.md).

Use pnpm **12.15.0**, pinned by packageManager in package.json. Install that version with `npm install --global pnpm@12.15.0` if pnpm is not available. The workspace contains apps/web and packages/api-client; workspace dependencies always resolve locally.

From the repository root: `pnpm install --frozen-lockfile`, `pnpm run generate:api`, `pnpm run format:check`, `pnpm run typecheck`, `pnpm run lint`, `pnpm run build`. Use `pnpm run format` to apply frontend/tooling formatting, `pnpm run dev` for the frontend on port 3005, `pnpm test` (or `pnpm run test:e2e`) for the browser suite and `pnpm audit --audit-level=high` for dependency checks. pnpm-lock.yaml is the dependency lockfile; update it deliberately with `pnpm install` when changing package manifests. Docker installs the pinned pnpm release and uses frozen workspace installation.

Dashboard TypeScript remains 7.0.2; the OpenAPI generator requires its compatible 5.9.3 compiler at the root, documented in docs/DEPENDENCIES.md. Strict peer resolution is enabled. Native build-script permissions are explicit in pnpm-workspace.yaml.

With Go 1.27.3, format changed Go files with `gofmt -w`, verify `gofmt -l .` has no output, then run `go build ./...`, `go vet ./...` and `go test ./...` in services/backend. Integration tests require dedicated PostgreSQL/Redis instances and create disposable databases. Browser tests use real persistent APIs and a clearly labeled development provider. Start the browser fixture before running the combined `sh scripts/test.sh` checks. See docs/VERIFICATION.md for fixture setup. Repository agents must follow AGENTS.md and run formatting, type/compile checks, lint and tests for each affected component before completing changes.

Domain/application have no framework, SQL, Telegram or queue-library dependencies. Inbound adapters invoke use cases; outbound adapters implement application ports; composition roots wire concrete dependencies. Typed SQL mappings are isolated in the PostgreSQL adapter and verified with real databases. No sqlc-generated persistence types enter the domain.

## Temporarily operate without independent backups

Independent encrypted backup remains the default. Setting `BACKUP_ENABLED=false`
explicitly opts out of retained asset backups, backup download fallback and deleted
asset restoration. Uploads still require verified encrypted primary storage. Temporary
encrypted retry checkpoints remain under the staging SSD alongside upload parts and
are removed by the worker after completion/cancellation cleanup; they are not recovery
backups. Logical staging reservations are halved in this mode to leave room for both
plaintext parts and temporary ciphertext. Production disk reserve checks remain active.

For GitHub deployments, set repository **Actions variable** `BACKUP_ENABLED` to
`false`. The workflow selects `deploy/compose.no-backup.yaml`, omits the backup bind
mounts and records the active policy in ignored `deploy/backup.env`. It preserves the
server `.env`. The dashboard reports backups disabled and offers no restore action.
The storage initializer prepares only staging/cache/local directories with application
ownership; it never creates a backup marker or claims a same-disk directory is independent.

For source-based manual deployment, set `BACKUP_ENABLED=false` in the protected
`.env` and include `-f deploy/compose.no-backup.yaml` after the base Compose file.
After an Actions deployment, use `bash deploy/compose-release.sh` as usual; it loads
the recorded policy automatically.

To enable backups later, provision and verify the independent mount using
`scripts/prepare-host.sh`, then set the Actions variable to `true` and deploy again.
Drain unfinished uploads before changing modes. Enabling backups protects new uploads;
it does not retroactively create backups for assets uploaded while disabled. Re-upload
those assets through AssetHub to create verified backups. Existing encrypted backup
files are not deleted by disabling the feature. PostgreSQL metadata and offline master
keys remain essential for recovery; without independent copies, primary storage or SSD
loss can cause permanent data loss. Database snapshots still require a configured destination.
