# Architecture

## Dependency direction

`cmd` composition roots → adapters → application/usecases → domain.

Domain imports only Go standard library. Application imports domain and application-owned ports, never database/HTTP/job/storage libraries. Inbound HTTP/queue/CLI adapters translate transport values and invoke use cases. Outbound adapters implement ports. Constructor injection wires concrete dependencies only at composition roots. SQL rows, Telegram response models and API DTOs remain at the adapter boundary. Automated import checks enforce these rules.

## Layout

`apps/web` is the static Next.js dashboard. `services/backend` is one Go module with API, worker and admin entry points; `internal/domain`, `internal/application/{ports,usecases}`, `internal/adapters/{inbound,outbound}`. `api/openapi.yaml` is the HTTP contract; `deploy` holds Compose/Nginx/build configuration. Frontend features consume a typed API adapter, never access Telegram.

## Trust boundaries

Nginx serves static pages and proxies `/v1` to Go. No API secrets enter a Next.js build. Go is authoritative for identity, authorization, validation and quotas. Session secrets and API keys are stored only as SHA-256 digests; passwords use Argon2id. Cookie-authenticated mutations require a CSRF header and matching configured origin. API keys cannot administer identities. Authorization precedes cache access and applies to queued work as well as HTTP. Unauthorized project objects are indistinguishable from missing objects.

## Persistence and asynchronous work

PostgreSQL is the source of truth. Upload completion atomically creates the asset and an outbox record. Redis is an execution transport, not the sole record of unfinished work. An outbox dispatcher re-enqueues outstanding assets after Redis loss. Redis sorted-set entries are execution hints; no extra job framework is needed. Jobs and use cases are idempotent; persist each completed chunk and use an asset lease to prevent concurrent processors. Lease expiry enables crash recovery. Redis uses AOF and noeviction.

Upload sessions transition receiving/completing/queued/complete/cancelled/expired. Assets transition staged → queued → uploading → verifying → ready; failures are retriable; deleting/deleted revoke serving immediately. Provider availability is separate from logical state. Retry upstream 429 using retry_after; exponential backoff with jitter for transient failures; classify permanent errors. Telegram sends cannot guarantee exactly-once delivery: a crash or timeout after accepted send may leave a remote orphan. Record ambiguity rather than claiming physical deduplication.

## Storage ports

Provider capabilities, Put, Open, Delete, Health operate on opaque ciphertext chunks and private references. Telegram stores documents; Local is a development adapter. A Telegram transport boundary permits hosted/local Bot API and future MTProto replacement. S3/R2 primary providers remain future work.

Each asset has a random 256-bit key wrapped by a versioned master key. Chunk AES-GCM uses random nonces and associated data binding asset ID and chunk index. Hash original content with SHA-256; hash ciphertext separately. Persist nonces and ciphertext length, encrypt Telegram locators with the master key, and never persist expiring token-bearing Telegram download URLs.

Backup chunks and an encrypted recovery manifest are atomically written and verified on the independent mount. A marker check fails closed if that mount disappears. Ready requires both primary references and the committed backup manifest. Staging survives retry; cache is disposable. Deleted backup records persist for 30 days and are unavailable through ordinary content endpoints. Recovery keys must be stored independently from backups.

## Retrieval

Authorize → encrypted cache → primary provider → verified backup fallback → authenticated decryption → streaming. A single-flight cache lock prevents duplicate retrievals. Download only chunks needed for a range; authenticate a complete chunk before writing its plaintext. Never trust upstream HTTP Range behavior. HEAD uses metadata. Serve full/single-range content with Content-Length, ETag, Accept-Ranges and Content-Range. Unsatisfiable ranges return 416. Multi-range requests fall back to full content. Abort a partially written response on later retrieval failure; do not append an error document to file bytes.

Only safe images/video receive inline previews. HTML/SVG and unknown types force attachment with nosniff. No archive extraction. Content/API responses use no-store; Cloudflare cache rules must bypass `/v1/*`. Signed links bind asset/version/expiry and are checked against current asset deletion state.

## Operations

Minimal unauthenticated liveness/readiness; detailed provider, queue and disk health requires authorization. Readiness distinguishes metadata service availability from provider degradation. Structured logs omit token-bearing URLs and signed query strings. Resource limits and bounded request bodies prevent memory/disk exhaustion. Back up PostgreSQL/config nightly, preserve keys offline and rehearse restore. Native hardware and real Telegram checks are release gates, not inferred from local tests.

## Implemented operations

The migrator uses the database owner and grants a restricted runtime role table/sequence access. API/worker containers carry only that runtime password. A private installation fingerprint rejects accidental master-key replacement. Wrapped-key envelopes are v1; deliberate rotation requires a rewrap migration. Storage health distinguishes Redis connectivity, the worker heartbeat, durable job failures, disk and primary/backup availability.

Backup manifests contain original ingestion metadata; later rename/visibility/tombstone state remains authoritative in PostgreSQL. Snapshot restoration must reconcile newer deletion/revocation history before exposing assets. Snapshot and isolated restore scripts are supplied; native hardware/live-provider release gates are explicitly separate from test fixtures.
