# AssetHub master plan

AssetHub is a self-hosted, single-installation asset platform. Applications use its versioned REST API and never receive Telegram credentials or channel references. Production targets ARM64 Linux with 12 GB RAM and SSD storage under `/srv`.

## Accepted decisions

- Clean Architecture with inward dependencies enforced by tests.
- Next.js/TypeScript/Tailwind static dashboard, Go API and worker, PostgreSQL metadata, Redis jobs, Nginx and optional Cloudflare Tunnel.
- Latest stable direct dependencies at implementation time, exact lockfiles and immutable deployment image references. Never use unpinned `latest` in committed deployment configuration.
- Invited team accounts. Installation roles admin/member; project roles manager/editor/viewer; single-project, explicitly scoped application keys.
- Telegram hosted Bot API is the initial production provider. Store encrypted documents in 16 MiB plaintext chunks; cap logical assets at 512 MiB. The hosted API's 20 MB retrieval ceiling governs chunk size, not its higher upload ceiling.
- Client uploads use 8 MiB resumable requests. Small multipart uploads remain supported.
- Encrypt each asset with an independent random key; AES-GCM authenticates each chunk. Keep master/signing keys outside PostgreSQL and back them up offline.
- A separate mounted filesystem must contain verified encrypted content and recovery metadata before an asset becomes ready. Default mount `/mnt/assethub-backup` requires an operator-owned marker.
- Private assets by default; short-lived signed content links; explicit public visibility.
- Deletion immediately revokes access. Backups are recoverable by administrators for 30 days. Telegram deletion is best effort and may fail for messages older than 48 hours.
- Light dashboard based on supplied sketches. Illustrative filenames/counts are never seeded as operational data.

## Acceptance

Persistent authentication, project isolation, scoped/revocable keys, resumable uploads, byte-identical retrieval, chunk-boundary HTTP ranges, queue recovery, auditable mutations and recoverable backups. No credentials, Telegram URLs or references appear in client responses or logs. Service restarts preserve data. Docker images support ARM64.

## Capacity defaults

10 users, 20 projects, about 100 GB logical assets; 512 MiB maximum asset, two upload workers, four content streams, 20 GiB staging, 10 GiB cache, 10% disk reserve. Logical quota accounting is independent of backup/ciphertext overhead. Defaults are configurable, not advertised capacity guarantees.

## Delivery and evidence

See IMPLEMENTATION_PHASES.md for progress and verification evidence. A local-provider deployment is development only. Production sign-off requires operator-provided Telegram access, backup mount, hostname/tunnel, offline key backup, native ARM64 endurance testing and a restore drill. Existing backups are not guaranteed independent if an operator mounts a directory on the same physical disk.
