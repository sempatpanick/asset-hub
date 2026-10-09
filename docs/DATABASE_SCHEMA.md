# Database schema

PostgreSQL is authoritative. UUIDs identify records; timestamps are UTC; sizes are BIGINT bytes. Versioned Goose migrations run before services. Mutations and their audit/outbox state commit atomically.

| Table | Implemented purpose |
|---|---|
| users | Unique lowercase email, Argon2id hash, admin/member role and disabled flag |
| invitations | Hashed single-use token, role, expiry and accepted timestamp |
| sessions | Hashed cookie token, user FK, CSRF token and expiry |
| projects | Name, description, creator and logical quota |
| project_members | Unique project/user with manager/editor/viewer role |
| api_keys | Scoped project key hash, visible prefix, expiry and revocation |
| uploads | Reserved size, project/actor, name, optional expected hash, state, 24-hour expiry, completion lease and cleanup timestamps |
| upload_parts | Immutable unique upload/index with exact size and SHA-256; staging paths derive from server UUID/index/hash |
| assets | Detected MIME, original size/hash, lifecycle/visibility, upload/provider, versioned wrapped key, sanitized error, content version and deletion/backup-purge state |
| asset_chunks | Unique asset/index, plaintext/ciphertext sizes, ciphertext hash, encrypted provider reference and best-effort purge status |
| asset_jobs | Durable outbox/execution state for upload/deletion, attempts, next attempt, lease token and expiry |
| audit_events | Actor, project/object, action, safe detail and timestamp |
| installation_keys | Private master-key fingerprint preventing accidental wrapping-key replacement |

Encrypted recovery manifests live on the independent backup filesystem; no plaintext channel/message/file locators or token-bearing download URLs are persisted. Ciphertext carries its random 12-byte nonce plus a 16-byte AES-GCM authentication tag. Key envelopes start with version 1.

Indices cover project/time, lifecycle, upload expiry, due jobs, audit time and deleted-asset retention. Runtime API/worker connections use assethub_api with CRUD/sequence permissions and no database/role creation or superuser rights. The owner connection is used only by migration/backup operations.

Quota reservation serializes project updates and counts active sessions plus non-deleted assets. Completion transfers the reservation atomically into an asset/job. Global staging reservations include terminal sessions until files are cleaned. Cleanup has a three-minute grace period for bounded in-flight transfers. Worker checkpoints compare live lease ownership; expired leases are recoverable and stale writers are rejected.

Ready requires all primary references, verified byte integrity and a committed encrypted backup manifest. Deletion increments the content version and tombstones immediately. Remote purge is best effort, while backup chunks remain available to administrator restore for 30 days. Restore verifies all plaintext checksums and rechecks quota/state/retention before committing; old signed links remain revoked.

Restore PostgreSQL first using the offline master key and retained chunks. Database snapshots preserve tombstones/revocations only up to their snapshot time: reconcile newer audit records before exposing recovery. Original ingestion manifests may be older than rename/visibility changes and are not automatically imported over authoritative database state.
