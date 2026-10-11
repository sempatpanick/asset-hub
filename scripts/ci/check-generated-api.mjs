import { createHash } from "node:crypto";
import { readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { pathToFileURL } from "node:url";

const artifacts = [
  "packages/api-client/src/schema.d.ts",
  "apps/web/public/api-docs/openapi.json",
  "apps/web/public/api-docs/openapi.yaml",
];

export function snapshotGeneratedAPI(root) {
  return Object.fromEntries(
    artifacts.map((path) => [
      path,
      createHash("sha256")
        .update(
          readFileSync(resolve(root, path), "utf8").replaceAll("\r\n", "\n"),
        )
        .digest("hex"),
    ]),
  );
}

export function checkGeneratedAPI(root, snapshot) {
  const current = snapshotGeneratedAPI(root);
  const changed = artifacts.filter((path) => current[path] !== snapshot[path]);
  if (changed.length)
    throw new Error(
      `Generated API artifacts are outdated: ${changed.join(", ")}. Run pnpm run generate:api and commit the results.`,
    );
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(resolve(process.argv[1])).href
) {
  const [mode, snapshotPath] = process.argv.slice(2);
  if (mode === "--snapshot" && snapshotPath) {
    writeFileSync(
      snapshotPath,
      JSON.stringify(snapshotGeneratedAPI(process.cwd())),
    );
  } else if (mode === "--check" && snapshotPath) {
    checkGeneratedAPI(
      process.cwd(),
      JSON.parse(readFileSync(snapshotPath, "utf8")),
    );
    console.log("Generated API artifacts are current.");
  } else {
    throw new Error("Usage: check-generated-api.mjs --snapshot|--check FILE");
  }
}
