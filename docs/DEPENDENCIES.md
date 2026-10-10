# Dependency verification

Resolved against npm, the Go module proxy, Docker Hub tag metadata and the public GCR manifest API on 2026-10-09. Exact package pins and lockfiles are included in the repository. Image digests are multi-platform references; Docker selects the matching host architecture.

| Dependency                               | Pinned release |
| ---------------------------------------- | -------------- |
| pnpm (verified 2026-10-10)               | 12.10.1        |
| Next.js                                  | 16.4.0         |
| React / React DOM                        | 19.3.0         |
| Dashboard TypeScript                     | 7.0.2          |
| Tailwind / PostCSS plugin                | 4.3.3          |
| Lucide React                             | 1.53.0         |
| Playwright                               | 1.64.0         |
| ESLint (verified 2026-10-10)             | 9.39.5         |
| ESLint Next.js config                    | 16.4.0         |
| OpenAPI TypeScript                       | 7.13.0         |
| Go                                       | 1.27.2         |
| chi                                      | 5.3.2          |
| pgx                                      | 5.11.0         |
| Goose                                    | 3.28.0         |
| go-redis                                 | 9.23.0         |
| x/crypto                                 | 0.57.0         |
| x/term                                   | 0.46.0         |
| Node container                           | 26.11.1        |
| Node tooling types (verified 2026-10-10) | 26.6.5         |
| PostgreSQL                               | 18.6           |
| Redis                                    | 8.10.2         |
| Nginx stable branch                      | 1.30.5         |
| cloudflared                              | 2026.10.0      |

## Compatibility exception

ESLint 9.39.5 is the latest compatible ESLint 9 release. ESLint 10.12.0 is the latest overall release, but the React/import/accessibility plugins used by eslint-config-next 16.4.0 declare support through ESLint 9. Strict peer resolution rejects ESLint 10 for this combination. The root TypeScript 5.9.3 compiler also satisfies the lint parser's supported version range; dashboard compilation uses its separate TypeScript 7.0.2 dependency. Upgrade ESLint when the Next.js plugin dependencies support it. No peer-check bypasses are enabled.

OpenAPI TypeScript 7.13.0 uses the JavaScript TypeScript compiler API (`ts.factory`). The latest TypeScript 7.0.2 compiler no longer exposes that API and generation fails with `Cannot read properties of undefined (reading createKeywordTypeNode)`. The root code-generation tool therefore uses TypeScript **5.9.3**, the latest release matching the generator's declared `^5.x` peer dependency. TypeScript 6 could generate types locally but failed a clean container npm installation before the pnpm migration; it is not a supported dependency combination. The dashboard retains 7.0.2 as its own exact dependency. Remove this exception when the generator supports TypeScript 7. pnpm strict peer resolution preserves this supported combination without force or peer-check bypasses.

Nginx uses its latest stable branch rather than the mainline branch. The distroless runtime is pinned by digest. Recheck release notes, audits and vulnerability advisories before internet deployment; registry freshness alone is not proof of security.

The dashboard references the internal API client with `workspace:*`, preventing version bumps from resolving a same-named registry package. pnpm-workspace.yaml declares the packages and native lifecycle-script approvals. The root packageManager pin and pnpm-lock.yaml control reproducible local/container installs. The previous npm lockfile is retired after import; application dependency versions are preserved during migration.

The exact Node tooling type release 26.6.5 was explicitly selected to match the latest dependency requirement. pnpm recorded a release-age exception for that single type-only package; general release-age policy and strict peer checks remain enabled.
