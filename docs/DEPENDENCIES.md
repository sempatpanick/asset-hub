# Dependency verification

Resolved against npm, the Go module proxy, Docker Hub tag metadata and the public GCR manifest API on 2026-10-09. Exact package pins and lockfiles are included in the repository. Image digests are multi-platform references; Docker selects the matching host architecture.

| Dependency | Pinned release |
|---|---|
| Next.js | 16.4.0 |
| React / React DOM | 19.3.0 |
| Dashboard TypeScript | 7.0.2 |
| Tailwind / PostCSS plugin | 4.3.3 |
| Lucide React | 1.53.0 |
| Playwright | 1.64.0 |
| OpenAPI TypeScript | 7.13.0 |
| Go | 1.27.2 |
| chi | 5.3.2 |
| pgx | 5.11.0 |
| Goose | 3.28.0 |
| go-redis | 9.23.0 |
| x/crypto | 0.57.0 |
| x/term | 0.46.0 |
| Node container | 26.11.1 |
| PostgreSQL | 18.6 |
| Redis | 8.10.2 |
| Nginx stable branch | 1.30.5 |
| cloudflared | 2026.10.0 |

## Compatibility exception

OpenAPI TypeScript 7.13.0 uses the JavaScript TypeScript compiler API (`ts.factory`). The latest TypeScript 7.0.2 compiler no longer exposes that API and generation fails with `Cannot read properties of undefined (reading createKeywordTypeNode)`. The root code-generation tool therefore uses TypeScript **5.9.3**, the latest release matching the generator's declared `^5.x` peer dependency. TypeScript 6 could generate types locally but failed a clean container `npm ci`; it is not a supported dependency combination. The dashboard retains 7.0.2 as its own exact dependency. Remove this exception when the generator supports TypeScript 7. Clean installations use normal npm peer resolution without force or legacy-peer-deps.

Nginx uses its latest stable branch rather than the mainline branch. The distroless runtime is pinned by digest. Recheck release notes, audits and vulnerability advisories before internet deployment; registry freshness alone is not proof of security.

The dashboard references the internal API client by local workspace path, preventing version bumps from resolving a same-named registry package.
