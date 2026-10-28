import { readFile, mkdir, writeFile } from "node:fs/promises";
import { load } from "js-yaml";
import { format } from "prettier";

// The website imports this same downloadable contract; no independent endpoint list.
const source = await readFile(
  new URL("../api/openapi.yaml", import.meta.url),
  "utf8",
);
const contract = load(source);
for (const item of Object.values(contract.paths)) {
  for (const operation of Object.values(item)) {
    if (!operation.operationId) continue;
    if (!operation.summary || !operation.description || !operation["x-access"])
      throw new Error(`Missing documentation for ${operation.operationId}`);
    for (const [status, response] of Object.entries(operation.responses)) {
      if (response.$ref)
        throw new Error(
          `Missing concrete response for ${operation.operationId}: ${status}`,
        );
      if (
        !response["x-response-example"] &&
        !Object.values(response.content ?? {}).some(
          (media) => media.example !== undefined,
        )
      )
        throw new Error(
          `Missing response example for ${operation.operationId}: ${status}`,
        );
    }
  }
}
const target = new URL("../apps/web/public/api-docs/", import.meta.url);
const artifacts = {
  "openapi.json": await format(JSON.stringify(contract), { parser: "json" }),
  "openapi.yaml": source,
};
const check = process.argv.includes("--check");
if (!check) await mkdir(target, { recursive: true });
for (const [name, content] of Object.entries(artifacts)) {
  if (check) {
    const saved = await readFile(new URL(name, target), "utf8");
    if (saved.replaceAll("\r\n", "\n") !== content.replaceAll("\r\n", "\n"))
      throw new Error(`${name} is outdated. Run pnpm run generate:api.`);
  } else await writeFile(new URL(name, target), content);
}
console.log(
  `${check ? "Verified" : "Generated"} downloadable API Docs contract with complete operation examples.`,
);
