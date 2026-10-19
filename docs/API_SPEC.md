# REST API specification

Base path `/v1`. Same-origin browser sessions use secure HttpOnly cookies. Applications use `Authorization: Bearer <project-api-key>`. Never put API keys in URLs. Signed content tokens are separate, short-lived capabilities. API JSON errors contain a stable code/message and request ID, without SQL/provider error text. Lists have bounded pagination and filters; object authorization failures return 404.

## Required endpoint roadmap

All routes below are implemented. Uploads require configured storage and a wrapping key. Independent backup is required by default; BACKUP_ENABLED=false explicitly disables that requirement, download fallback and restore. Signing additionally requires SIGNED_URL_KEY. The OpenAPI contract describes the shipped identity, ingestion, content and management endpoints; unavailable configuration returns a real error rather than simulated success.

| Method/path                       | Behavior                                                                                              |
| --------------------------------- | ----------------------------------------------------------------------------------------------------- |
| POST /v1/assets                   | Small multipart upload (8 MiB maximum); project_id and file; returns 202 queued asset                 |
| GET /v1/assets                    | Authorized project assets; name/MIME-prefix/status/project filters; administrator-only deleted filter |
| GET /v1/assets/{id}               | Safe metadata only                                                                                    |
| PATCH /v1/assets/{id}             | Rename; manager-controlled public/private visibility                                                  |
| DELETE /v1/assets/{id}            | Immediate tombstone/access revocation; async physical cleanup                                         |
| GET, HEAD /v1/assets/{id}/content | Authorized/public/signed full or single-range content                                                 |
| POST /v1/assets/{id}/signed-url   | Ready asset capability; default 900 seconds; maximum 24 hours                                         |
| POST /v1/assets/{id}/retry        | Requeue failed asset                                                                                  |
| POST /v1/assets/{id}/restore      | Installation administrator restores retained backup                                                   |
| GET, POST /v1/projects            | List authorized projects; create project and manager membership                                       |
| PATCH /v1/projects/{id}           | Project manager updates name/description/quota                                                        |
| GET, POST /v1/api-keys            | Authorized key metadata; create a single-project scoped secret                                        |
| POST /v1/api-keys/{id}/reveal     | Session manager confirms current dashboard password to retrieve an active encrypted key secret        |
| DELETE /v1/api-keys/{id}          | Revoke key immediately                                                                                |
| GET /v1/storage/health            | Authorized, sanitized provider/backup health                                                          |

## Browser identity

POST /v1/auth/login, POST /v1/auth/logout, GET /v1/auth/me. Admin invitation creation and invitation acceptance endpoints. Cookie mutations require X-CSRF-Token and a matching Origin. No public registration. Login is rate limited; invitation acceptance is atomic and one-time.

## Resumable uploads

POST /v1/uploads declares project_id, name and size (up to 512 MiB); returns upload ID, 8 MiB part size and expiry based on the saved session lifetime (default 24 hours). PUT /v1/uploads/{id}/parts/{index} sends raw bytes; verify length and SHA-256 and permit retries only for identical bytes. GET returns received part indices. POST complete verifies all parts and checksum, atomically creates a queued asset and outbox event, and returns 202. DELETE cancels an unfinished session. The dashboard uses resumable sessions for multi-file uploads; client transfer progress is distinct from provider-job progress.

## Dashboard

GET /v1/overview supplies authorized total files/logical bytes/projects, daily upload activity and recent assets. GET /v1/activity supplies audit events. GET /v1/uploads supplies upload/job progress. Project membership endpoints expose explicit roles. GET /v1/settings exposes effective non-secret configuration, feature availability and upload_concurrency. An administrator session can PATCH /v1/settings with {"upload_concurrency":20} to persist a limit from 1 to 20 (default 20); session CSRF and Origin checks apply. The limit applies to concurrent multipart, raw-part and completion requests on each API process, including project-key uploads. Changes apply without restart; lowering the limit lets existing transfers finish. Excess requests return 429 busy with Retry-After: 2. Other deployment settings remain environment-based. The browser uploads files in parallel up to the saved limit.

The same Settings page and PATCH endpoint manage upload_request_timeout_seconds (30–1800, default 120), upload_response_timeout_seconds (60–1830, default 150) and upload_session_lifetime_hours (1–168, default 24). Response timeout must be at least 30 seconds longer than request timeout. PATCH accepts one or more fields, atomically preserving omitted values. Nulls, non-integers, unknown fields and invalid combinations are rejected. Request deadlines start after authentication and headers; the response deadline includes receiving and validation. Timeouts apply to new upload requests, and session lifetime sets expiry for new sessions. Active requests and existing session expiry retain their recorded values. The shipped Nginx upload location permits up to 1860 seconds while the API enforces the configured deadlines. Other API routes and downloads retain their existing limits; external proxies and caller deadlines remain separately configured.

## HTTP content semantics

Ready assets only. HEAD contains content metadata without retrieving file bytes. Content-Length equals the selected original byte count; SHA-256 ETag identifies immutable bytes. Range supports one range, including open-ended/suffix ranges. Unsatisfiable requests return 416 with `Content-Range: bytes */size`. Multi-range headers are ignored. Download filename headers are sanitized. Content/API responses use no-store; public/signed requests disclose content only, not protected metadata.

## Health

GET /healthz reports process liveness; GET /readyz checks required metadata/queue dependencies without disclosing configuration. Detailed storage health is protected. Telegram checks actual bot/channel access; absent credentials report unavailable. Protected health also reports worker heartbeat, durable jobs and staging disk availability. Missing backup mount blocks asset readiness when backups are enabled. With backups explicitly disabled, health reports backup status disabled; GET /v1/settings exposes features.backups=false and backup_retention_days=0. Primary chunk retrieval/integrity verification still gates readiness.

The OpenAPI contract is maintained in `api/openapi.yaml`; effective configuration can disable storage or signed links when their required secrets/providers are absent.
