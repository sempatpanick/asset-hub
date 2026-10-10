import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTypeScript from "eslint-config-next/typescript";

export default defineConfig([
  ...nextVitals,
  ...nextTypeScript,
  { settings: { next: { rootDir: "apps/web/" } } },
  globalIgnores([
    "**/node_modules/**",
    "apps/web/.next/**",
    "apps/web/out/**",
    "apps/web/next-env.d.ts",
    "packages/api-client/src/schema.d.ts",
    ".local/**",
    "test-results/**",
    "playwright-report/**",
  ]),
]);
