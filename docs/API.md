# API documentation

Open **API Docs** from the workspace sidebar. It opens `/api-docs/` in a new tab;
the guide and reference are public, while API requests still enforce the caller’s
authentication and permissions.

The website documents all 37 HTTP operations: sessions, invitations, projects,
memberships, API keys, files, resumable uploads, content, workspace settings,
activity, dependency health, liveness and readiness. Each operation includes its
access requirements, parameters, body fields, success examples and error examples.
The guide explains server integrations, session CSRF, key scopes, pagination,
upload progress, checksums, downloads, signed links, backup retention and retries.

The API Keys dashboard shows the selected Project ID and the ID on each key row,
with Copy ID buttons. Copy API key requires the signed-in user's existing dashboard
password on every attempt, including immediately after creation. Project managers
can retrieve active keys through `POST /v1/api-keys/{id}/reveal?project_id=...` with
a session, CSRF token, matching Origin and a JSON `password`. New secrets are
encrypted using the existing `ASSET_MASTER_KEY`; apply migration 00005 before
running this version. Older hash-only keys remain usable but cannot be recovered:
create a replacement, update the consuming application and then revoke the old key.

The console sends requests to the same installation. Select a dashboard session,
a project API key, or anonymous access. Session writes obtain the current CSRF
token before sending. Key/anonymous requests omit session cookies. A typed key is
held in memory, and generated cURL examples use credential placeholders. Request
results show the actual status, headers and response body separately from samples.

The console supports JSON, multipart files, raw upload parts, HEAD and byte-range
requests. Raw-part checksums are calculated automatically on HTTPS/localhost, or
can be supplied manually. Requests can be cancelled and time out after 150 seconds;
check server state before retrying a cancelled write. Response previews stop at
64 KiB; use cURL or an application client to save complete large downloads.

Download the machine-readable contract at:

- `/api-docs/openapi.json`
- `/api-docs/openapi.yaml`

## Maintaining the reference

[api/openapi.yaml](../api/openapi.yaml) is the source of truth. Add an operation’s
summary, plain-language description, tags, `x-access`, parameter descriptions,
schemas and concrete response examples when changing the API. Binary and HEAD
responses use `x-response-example` for an illustrative HTTP response. Describe
authentication differences and exceptional error shapes explicitly.

Run `pnpm run generate:api` after contract changes. It regenerates the typed client
and downloadable website contracts. The website imports that same JSON artifact,
so it does not maintain a separate endpoint catalog. `pnpm run generate:docs`
regenerates only the documentation artifacts. Verify artifact freshness without
writing files with `node scripts/generate-api-docs.mjs --check`.

Run the frontend checks and the real PostgreSQL/Redis-backed Playwright workflows
described in [VERIFICATION.md](VERIFICATION.md). `tests/e2e/api-docs.spec.ts` compares
the documented operations to the backend routes, visits every reference and
response example, checks downloads/mobile/new-tab navigation, and exercises real
session/key reads, writes, multipart and resumable transfers, range/HEAD requests,
checksum failures, incomplete uploads, permission errors and revoked keys.

Deployment serves the docs as part of the existing static web export. No external
documentation service, CDN, browser CORS changes or separate runtime is required.
