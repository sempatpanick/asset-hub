# Dependency verification

JavaScript packages, Go modules, container images and GitHub Actions were rechecked against their upstream registries on 2026-11-23. The repository pins packages and actions to exact releases and container images to multi-platform digests.

| Dependency                             | Pinned release |
| -------------------------------------- | -------------- |
| pnpm                                   | 12.21.0        |
| Next.js                                | 16.4.6         |
| React / React DOM                      | 19.3.1         |
| Dashboard TypeScript                   | 7.0.2          |
| Tailwind / PostCSS plugin              | 4.3.5          |
| Lucide React                           | 1.77.0         |
| Playwright                             | 1.65.1         |
| ESLint                                 | 9.39.5         |
| ESLint Next.js config                  | 16.4.6         |
| OpenAPI TypeScript                     | 7.13.0         |
| Root TypeScript                        | 5.9.3          |
| Prettier                               | 3.10.1         |
| js-yaml                                | 5.5.0          |
| Go                                     | 1.27.4         |
| chi                                    | 5.5.0          |
| pgx                                    | 5.13.0         |
| Goose                                  | 3.30.1         |
| go-redis                               | 9.23.0         |
| x/crypto                               | 0.59.0         |
| x/sys                                  | 0.50.0         |
| x/term                                 | 0.48.0         |
| Node container and CI                  | 26.13.1        |
| Node tooling types                     | 26.6.6         |
| PostgreSQL (default)                   | 18.7           |
| PostgreSQL (after migration and in CI) | 19.0           |
| Redis                                  | 8.10.3         |
| Nginx stable branch                    | 1.30.7         |
| cloudflared                            | 2026.11.1      |

## Compatibility exception

ESLint 9.39.5 is the latest compatible ESLint 9 release. The React, import and accessibility plugins used by eslint-config-next 16.4.6 declare support through ESLint 9, so strict peer resolution rejects ESLint 10.15.0. The root TypeScript 5.9.3 compiler also satisfies the lint parser's supported version range. Dashboard compilation uses a separate TypeScript 7.0.2 dependency.

OpenAPI TypeScript 7.13.0 uses the JavaScript TypeScript compiler API (`ts.factory`). The latest TypeScript 7.0.2 compiler no longer exposes that API and generation fails with `Cannot read properties of undefined (reading createKeywordTypeNode)`. The root code-generation tool therefore uses TypeScript **5.9.3**, the latest release matching the generator's declared `^5.x` peer dependency. TypeScript 6 could generate types locally but failed a clean container npm installation before the pnpm migration; it is not a supported dependency combination. The dashboard retains 7.0.2 as its own exact dependency. Remove this exception when the generator supports TypeScript 7. pnpm strict peer resolution preserves this supported combination without force or peer-check bypasses.

Nginx follows the stable release branch. The PostgreSQL 18.7 default preserves the data format of existing installations. See the [PostgreSQL 19 migration procedure](OPERATIONS.md#postgresql-18-to-19) for enabling the new major release.

The web and verification builds and CI use the Node.js 26.13.1 security release. The web build stage uses Alpine 3.24 and the verification Node stage uses Bookworm.

Release sources: [Node.js release index](https://nodejs.org/dist/index.json), [Go releases](https://go.dev/dl/?mode=json), [npm registry](https://registry.npmjs.org/), and [Go module proxy](https://proxy.golang.org/). The npm `latest` tags select stable packages; existing compatibility exceptions above remain explicit.

The dashboard references the internal API client with `workspace:*`, preventing version bumps from resolving a same-named registry package. pnpm-workspace.yaml declares the packages and native lifecycle-script approvals. The root packageManager pin and pnpm-lock.yaml control reproducible local/container installs. Installations use `pnpm install --frozen-lockfile`.
