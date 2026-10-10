# Deployment and recovery

## Provision the Orange Pi

Use a 64-bit Linux OS and Docker Compose. Mount the SSD under `/srv` and a separate backup filesystem at `/mnt/assethub-backup` using stable filesystem UUIDs. Check the physical devices: different filesystem IDs do not prove different physical disks. The application rejects the same filesystem in production and checks the backup marker on every operation.

Run `sudo bash scripts/prepare-host.sh /srv/assethub /mnt/assethub-backup`. Set matching DATA_ROOT and BACKUP_PATH in `.env`. This prepares restricted directories for UID/GID 65532; PostgreSQL initializes its own persistent directory. Do not create the backup marker on an unmounted directory. Preserve ownership when copying data.

Generate independent secrets with `openssl rand -hex 32` for PostgreSQL owner, APP_DATABASE_PASSWORD, Redis, ASSET_MASTER_KEY and SIGNED_URL_KEY. Store an offline recovery copy of the master/signing keys separately from the SSD, PostgreSQL and backup mount. Losing the master key makes ciphertext unrecoverable. The installation binds a private master-key fingerprint and rejects an accidental key change at startup. The initial key envelope is version 1; key rotation requires a controlled rewrap migration, not replacing the environment value.

Create a Telegram bot, grant it posting/deletion rights in a private storage channel, and configure TELEGRAM_BOT_TOKEN and TELEGRAM_CHANNEL_ID only in the protected environment file. The production provider is Telegram; Local is accepted only in development. No secret enters the frontend build. Hosted Bot API chunks are 16 MiB plaintext plus 28 authenticated-encryption bytes, below the 20 MB retrieval ceiling. Sends/deletes use a Redis-shared three-second gate and honor retry_after. Ambiguous sends can leave remote orphans; audit records identify ambiguity, and physical deletion is best effort.

Run Docker Compose with `.env` and bootstrap the first administrator interactively. The frontend uses host port 3005; the API, PostgreSQL and Redis use localhost host ports 8085, 5435 and 6385. Their environment overrides are HTTP_PORT, API_PORT, POSTGRES_PORT and REDIS_PORT. Inside Compose, Nginx listens on 3005 and proxies API traffic to api:8085; PostgreSQL/Redis listen on 5432/6379. These internal database addresses are independent of host port mappings.

Use the existing Orange Pi tunnel: when cloudflared runs directly on the host, add the AssetHub hostname route to http://127.0.0.1:3005. Keep TUNNEL_TOKEN blank and run Compose without the tunnel profile. If using AssetHub's optional tunnel container, configure its token, enable the tunnel profile and route it to http://web:3005. A separately containerized connector needs a reachable shared-network or host-network address. Only publish the frontend through the tunnel. Migrations use the database owner; API/worker containers use the non-superuser assethub_api role with table/sequence access and no database/role creation privileges. The migrator provisions that role with APP_DATABASE_PASSWORD. Set Cloudflare caching to bypass `/v1/*`; do not enable query-string logging for signed links. Nginx access/error logs are disabled because native errors can include signed URLs; API logs use sanitized structured events. Configure monitoring from protected health/audit endpoints.

## Local Windows configuration

The root compose.yaml supports `docker compose up --build -d` and includes the deployment model without duplicating it. Use Docker Compose 2.20 or newer. This finds the configuration; it does not convert production settings into development settings automatically.

For a local HTTP run, set APP_ENV=development and PUBLIC_ORIGIN=http://localhost:3005 in the private `.env`. Set DATA_ROOT and BACKUP_PATH to real absolute Windows directories using forward slashes, for example C:/Users/muhda/Documents/projects/asset-hub/.local/data and C:/Users/muhda/Documents/projects/asset-hub/.local/backup. Create staging/cache/local under DATA_ROOT and a regular .assethub-backup marker file under BACKUP_PATH. Development permits a shared filesystem. Production still requires a separate mounted backup filesystem. Keep Telegram as the provider when testing actual Telegram storage; choose Local explicitly only for local storage tests. Keep existing secrets and encryption keys.

DISK_RESERVE_PERCENT defaults to 10. For development on a shared disk that falls below this reserve, it may be lowered deliberately; this is not production policy. Verify ports are available before startup. The Discord worker uses localhost port 8095, leaving 8085 for AssetHub. API_PORT controls AssetHub's host mapping, while the API still listens on 8085 inside its container.

## Capacity and service health

Defaults: two workers, four downloads, four browser transfers, 20 GiB staging reservations, 10 GiB encrypted cache and 10% disk reserve. Production reserve cannot be lowered below 10%. API/worker processes use a 384 MiB Go memory target inside 512 MiB container limits; password hashing has two concurrent slots. Development fixtures explicitly use zero reserve on a shared test disk; this is not a production policy. Logical project quotas count assets and active receiving/completing sessions; global staging reservations remain held until cleanup. Terminal staging cleanup waits three minutes for bounded HTTP transfers to finish. Failed uploads retain staging for retries and consume capacity until deleted.

`/healthz` is process liveness, `/readyz` checks PostgreSQL/Redis. Protected storage health reports the primary, Redis, recent worker heartbeat, durable job counts, backup and staging disk status. Missing backup blocks acceptance/readiness of new assets; existing verified cached content may still be served. Never infer a working primary from successful database readiness. Alerts should cover missing worker heartbeat, failed assets, disk reserve, backup loss and sustained pending jobs.

Redis uses AOF/noeviction and PostgreSQL is the durable outbox. Dispatch re-creates missing Redis queue entries every five seconds. A ten-minute lease and per-chunk checkpoints permit restart recovery; each network operation has a shorter timeout. Bounded exponential retries stop upload jobs after eight attempts; authorized users can retry. Deletion records unavailable remote objects without pretending they were purged.

Authentication throttling uses the direct network peer and intentionally ignores untrusted forwarded IP headers. Behind Nginx this applies a shared ten-attempt-per-minute budget to login/invitation acceptance. Configure an additional edge login limit when needed; do not expose the API container or trust arbitrary client headers. Signed links are independent bearer capabilities until expiry or an asset-version change; revoking the issuing API key prevents new issuance but does not revoke an existing link. Rename, visibility changes, deletion and restore invalidate previous links.

Downloads refresh a 150-second write deadline as bytes flow, allowing large active streams to outlive ordinary request timeouts while bounding stalled clients. The reverse proxy also enforces idle timeouts. Streaming remains limited to four concurrent requests and decrypts complete authenticated chunks before serving their bytes.

## Backup schedule

Run `sudo bash scripts/backup.sh .env /mnt/assethub-backup` nightly using an operator-managed systemd timer. It writes an atomic custom-format PostgreSQL snapshot with a checksum and restricted permissions. It does not delete previous snapshots automatically. Manage snapshot retention against verified recovery requirements. Keep a separate protected copy of Compose/configuration and offline keys. Never publish `.env` or snapshots.

Ready assets have encrypted chunks and an encrypted recovery manifest on the backup filesystem. Manifests retain original ingestion metadata; PostgreSQL is authoritative for later rename/visibility/tombstone changes. Always restore the database snapshot first and review audit/deletion history before importing older manifests. An asset deleted in a newer database state must not be resurrected from an old backup.

## Recovery drill

1. Verify the snapshot checksum. Restore into a new isolated database with `bash scripts/restore-database.sh .env SNAPSHOT assethub_restore_drill_20261009`. The script refuses to overwrite an existing database.
2. Use the same offline master key and an isolated API instance with the retained encrypted backup mounted read-only. Leave the production database and services intact.
3. Verify private access, representative full files/checksums, cross-chunk video ranges, project membership, API-key revocation and tombstones. Disconnect the primary to exercise backup fallback. Restore a deleted asset through the administrator-only endpoint while within its 30-day retention window.
4. Only after verification, stop traffic/workers, preserve the existing database and deliberately switch to the verified database. Run migrations using the owner connection before starting services; this reapplies runtime-role grants to a restored database. Rotate session/API credentials according to incident scope, and review configuration/secrets independently.

Restoring a PostgreSQL snapshot can roll back deletions and revocations that occurred after that snapshot. Reconcile newer audit/deletion records before exposing the recovered instance. Encrypted manifests are recovery evidence, not permission to make an asset public. Recovery beyond the snapshot remains a supervised operation; automated orphan-manifest import is not shipped.

## Required production release gates

Live Telegram upload/retrieval and rate-limit behavior with the actual bot/channel; independent backup mount disappearance and recovery; native Orange Pi ARM64 execution; sustained upload/download resource measurements; an operator-run database/key/asset restore drill; HTTPS/tunnel and caching validation. Local-provider tests and ARM64 cross-compilation do not satisfy these gates. S3/R2 and direct MTProto transports remain future providers. A custom Bot API gateway can be configured, but local-server absolute file paths need a dedicated transport and are not accepted by the hosted-compatible adapter.
