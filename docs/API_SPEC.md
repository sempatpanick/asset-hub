# REST API specification

Base path `/v1`. Same-origin browser sessions use secure HttpOnly cookies. Applications use `Authorization: Bearer <project-api-key>`. Never put API keys in URLs. Signed content tokens are separate, short-lived capabilities. API JSON errors contain a stable code/message and request ID, without SQL/provider error text. Lists have bounded pagination and filters; object authorization failures return 404.

## Required endpoint roadmap

All routes below are implemented. Uploads require configured storage, a wrapping key and the required backup. Signing additionally requires SIGNED_URL_KEY. The OpenAPI contract describes the shipped identity, ingestion, content and management endpoints; unavailable configuration returns a real error rather than simulated success.

| Method/path | Behavior |
|---|---|
| POST /v1/assets | Small multipart upload (8 MiB maximum); project_id and file; returns 202 queued asset |
| GET /v1/assets | Authorized project assets; name/MIME-prefix/status/project filters; administrator-only deleted filter |
| GET /v1/assets/{id} | Safe metadata only |
| PATCH /v1/assets/{id} | Rename; manager-controlled public/private visibility |
| DELETE /v1/assets/{id} | Immediate tombstone/access revocation; async physical cleanup |
| GET, HEAD /v1/assets/{id}/content | Authorized/public/signed full or single-range content |
| POST /v1/assets/{id}/signed-url | Ready asset capability; default 900 seconds; maximum 24 hours |
| POST /v1/assets/{id}/retry | Requeue failed asset |
| POST /v1/assets/{id}/restore | Installation administrator restores retained backup |
| GET, POST /v1/projects | List authorized projects; create project and manager membership |
| PATCH /v1/projects/{id} | Project manager updates name/description/quota |
| GET, POST /v1/api-keys | Authorized key metadata; create single-project scoped secret shown once |
| DELETE /v1/api-keys/{id} | Revoke key immediately |
| GET /v1/storage/health | Authorized, sanitized provider/backup health |

## Browser identity

POST /v1/auth/login, POST /v1/auth/logout, GET /v1/auth/me. Admin invitation creation and invitation acceptance endpoints. Cookie mutations require X-CSRF-Token and a matching Origin. No public registration. Login is rate limited; invitation acceptance is atomic and one-time.

## Resumable uploads

POST /v1/uploads declares project_id, name and size (up to 512 MiB); returns upload ID, 8 MiB part size and 24-hour expiry. PUT /v1/uploads/{id}/parts/{index} sends raw bytes; verify length and SHA-256 and permit retries only for identical bytes. GET returns received part indices. POST complete verifies all parts and checksum, atomically creates a queued asset and outbox event, and returns 202. DELETE cancels an unfinished session. The dashboard uses resumable sessions for multi-file uploads; client transfer progress is distinct from provider-job progress.

## Dashboard

GET /v1/overview supplies authorized total files/logical bytes/projects, daily upload activity and recent assets. GET /v1/activity supplies audit events. GET /v1/uploads supplies upload/job progress. Project membership endpoints expose explicit roles. Settings expose effective non-secret configuration and feature availability. Configuration changes are environment-based; this endpoint is read-only.

## HTTP content semantics

Ready assets only. HEAD contains content metadata without retrieving file bytes. Content-Length equals the selected original byte count; SHA-256 ETag identifies immutable bytes. Range supports one range, including open-ended/suffix ranges. Unsatisfiable requests return 416 with `Content-Range: bytes */size`. Multi-range headers are ignored. Download filename headers are sanitized. Content/API responses use no-store; public/signed requests disclose content only, not protected metadata.

## Health

GET /healthz reports process liveness; GET /readyz checks required metadata/queue dependencies without disclosing configuration. Detailed storage health is protected. Telegram checks actual bot/channel access; absent credentials report unavailable. Protected health also reports worker heartbeat, durable jobs and staging disk availability. Missing backup mount blocks asset readiness.

The OpenAPI contract is maintained in `api/openapi.yaml`; effective configuration can disable storage or signed links when their required secrets/providers are absent.
