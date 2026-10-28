# Dependency verification

JavaScript packages, Go modules, pnpm, Node.js and Go releases were rechecked against their official registries on 2026-10-28. Docker Node.js, Go and Playwright tags and multi-platform digests were refreshed on the same date. Other service image pins below retain their previous verification. Exact package pins and lockfiles are included in the repository; Docker selects the matching host architecture.

| Dependency                | Pinned release |
| ------------------------- | -------------- |
| pnpm                      | 12.15.0        |
| Next.js                   | 16.4.3         |
| React / React DOM         | 19.3.1         |
| Dashboard TypeScript      | 7.0.2          |
| Tailwind / PostCSS plugin | 4.3.3          |
| Lucide React              | 1.61.0         |
| Playwright                | 1.64.1         |
| ESLint                    | 9.39.5         |
| ESLint Next.js config     | 16.4.3         |
| OpenAPI TypeScript        | 7.13.0         |
| Root TypeScript           | 5.9.3          |
| Prettier                  | 3.9.10         |
| js-yaml                   | 5.5.0          |
| Go                        | 1.27.3         |
| chi                       | 5.5.0          |
| pgx                       | 5.11.0         |
| Goose                     | 3.29.0         |
| go-redis                  | 9.23.0         |
| x/crypto                  | 0.58.0         |
| x/sys                     | 0.49.0         |
| x/term                    | 0.47.0         |
| Node container and CI     | 26.12.1        |
| Node tooling types        | 26.6.5         |
| PostgreSQL                | 18.6           |
| Redis                     | 8.10.2         |
| Nginx stable branch       | 1.30.5         |
| cloudflared               | 2026.10.0      |

## Compatibility exception

ESLint 9.39.5 is the latest compatible ESLint 9 release. ESLint 10.13.0 is the latest overall release, but the React/import/accessibility plugins used by eslint-config-next 16.4.3 still declare support through ESLint 9. Strict peer resolution rejects ESLint 10 for this combination. The root TypeScript 5.9.3 compiler also satisfies the lint parser's supported version range; dashboard compilation uses its separate TypeScript 7.0.2 dependency. Upgrade ESLint when the Next.js plugin dependencies support it. No peer-check bypasses are enabled.

OpenAPI TypeScript 7.13.0 uses the JavaScript TypeScript compiler API (`ts.factory`). The latest TypeScript 7.0.2 compiler no longer exposes that API and generation fails with `Cannot read properties of undefined (reading createKeywordTypeNode)`. The root code-generation tool therefore uses TypeScript **5.9.3**, the latest release matching the generator's declared `^5.x` peer dependency. TypeScript 6 could generate types locally but failed a clean container npm installation before the pnpm migration; it is not a supported dependency combination. The dashboard retains 7.0.2 as its own exact dependency. Remove this exception when the generator supports TypeScript 7. pnpm strict peer resolution preserves this supported combination without force or peer-check bypasses.

Nginx uses its latest stable branch rather than the mainline branch. The distroless runtime is pinned by digest. Recheck release notes, audits and vulnerability advisories before internet deployment; registry freshness alone is not proof of security.

Node.js 26.12.1 is the latest stable release in the official release index on 2026-10-28 and is marked as a security release. Both Node Docker stages and CI use it; the web production image serves the static export with Nginx. The Alpine and Bookworm variants retain their existing distributions and use verified multi-platform digests.

Release sources: [Node.js release index](https://nodejs.org/dist/index.json), [Go releases](https://go.dev/dl/?mode=json), [npm registry](https://registry.npmjs.org/), and [Go module proxy](https://proxy.golang.org/). The npm `latest` tags select stable packages; existing compatibility exceptions above remain explicit.

The dashboard references the internal API client with `workspace:*`, preventing version bumps from resolving a same-named registry package. pnpm-workspace.yaml declares the packages and native lifecycle-script approvals. The root packageManager pin and pnpm-lock.yaml control reproducible local/container installs. Installations use `pnpm install --frozen-lockfile`.

The exact Node tooling type release 26.6.5 was explicitly selected to match the latest dependency requirement. pnpm recorded a release-age exception for that single type-only package; general release-age policy and strict peer checks remain enabled.
