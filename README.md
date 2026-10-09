# AssetHub

A self-hosted asset workspace with Clean Architecture: Next.js/TypeScript/Tailwind dashboard, Go API and workers, PostgreSQL metadata, Redis background execution, encrypted Telegram storage and an independent encrypted backup.

## Implemented

Invited accounts, secure sessions/CSRF, project roles, scoped/revocable keys, resumable multi-file uploads, quota/staging reservations, durable jobs and chunk checkpoints, Telegram and development Local providers, verified encrypted backups, authorized/public/signed content, video byte ranges, file search/type/status/project filters, grid/list previews, rename/delete/administrator restore, audit and detailed health. Dashboard data comes from persistent APIs; no example assets or counters are seeded.

The implementation and automated checks are complete through the ingestion/content phases. Operational tooling is included. Production sign-off still requires the live Telegram channel, independent physical backup storage, HTTPS/tunnel configuration, native Orange Pi execution and an operator-run endurance/recovery drill. Read docs/IMPLEMENTATION_PHASES.md and docs/VERIFICATION.md for evidence and remaining gates.

## Run on ARM64 Linux

1. Copy `.env.example` to `.env`. Generate distinct 64-character hexadecimal secrets with `openssl rand -hex 32` for POSTGRES_PASSWORD, APP_DATABASE_PASSWORD, REDIS_PASSWORD, ASSET_MASTER_KEY and SIGNED_URL_KEY. Set PUBLIC_ORIGIN to your HTTPS origin. Keep the real file private and store recovery keys offline.
2. Mount the SSD under `/srv` and an independent backup filesystem at `/mnt/assethub-backup`. Run `sudo bash scripts/prepare-host.sh /srv/assethub /mnt/assethub-backup`. Set matching DATA_ROOT/BACKUP_PATH. The helper verifies the mount and prepares restricted UID/GID 65532 directories.
3. Create a Telegram bot with posting/deletion rights in a private storage channel. Set TELEGRAM_BOT_TOKEN and TELEGRAM_CHANNEL_ID in `.env`.
4. Run `docker compose --env-file .env -f deploy/compose.yaml up -d --build`. The migrator applies schemas and provisions the limited runtime database role; owner credentials are absent from API/worker containers.
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

`npm ci`, `npm run generate:api`, `npm run typecheck`, `npm run build`. Dashboard TypeScript is 7.0.2; the latest OpenAPI generator requires its compatible 5.9.3 compiler at the root, documented in docs/DEPENDENCIES.md. Normal clean peer resolution is supported.

With Go 1.27.2, run `go test ./...` and `go vet ./...` in services/backend. Integration tests require dedicated PostgreSQL/Redis instances and create disposable databases. Browser tests use real persistent APIs and a clearly labeled development provider. See docs/VERIFICATION.md.

Domain/application have no framework, SQL, Telegram or queue-library dependencies. Inbound adapters invoke use cases; outbound adapters implement application ports; composition roots wire concrete dependencies. Typed SQL mappings are isolated in the PostgreSQL adapter and verified with real databases. No sqlc-generated persistence types enter the domain.
