import { readFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
import type { Page } from "@playwright/test";
import { test, expect } from "./fixtures";
import { selectOption } from "./select";

test.beforeAll(() => {
  execFileSync(process.execPath, ["scripts/generate-api-docs.mjs", "--check"]);
});

const spec = JSON.parse(
  readFileSync("apps/web/public/api-docs/openapi.json", "utf8"),
) as {
  paths: Record<
    string,
    Record<
      string,
      {
        operationId?: string;
        summary?: string;
        responses?: Record<string, unknown>;
      }
    >
  >;
};
const documented = Object.entries(spec.paths).flatMap(([path, item]) =>
  Object.entries(item)
    .filter(([, op]) => op.operationId)
    .map(([method, op]) => ({ path, method: method.toUpperCase(), ...op })),
);
const consolePanel = (page: Page) =>
  page.getByRole("region", { name: "Request console", exact: true });
async function operation(page: Page, id: string) {
  await page
    .getByRole("navigation", { name: "API endpoints" })
    .locator(`a[href="#${id}"]`)
    .click();
  await expect(consolePanel(page)).toBeVisible();
}
async function send(page: Page, status: number) {
  await consolePanel(page)
    .getByRole("button", { name: "Send request", exact: true })
    .click();
  await expect(
    consolePanel(page).getByRole("heading", {
      name: new RegExp(`Live response: HTTP ${status} `),
    }),
  ).toBeVisible();
}
async function body(page: Page) {
  return JSON.parse(
    (await consolePanel(page)
      .locator(".docs-live .docs-code")
      .last()
      .locator("code")
      .textContent())!,
  );
}
test("public API Docs cover every backend route and open from the workspace in a new tab", async ({
  page,
  context,
  signInAdmin,
}) => {
  test.setTimeout(90000);
  const server = readFileSync(
    "services/backend/internal/adapters/inbound/httpapi/server.go",
    "utf8",
  );
  const routes = Array.from(
    server.matchAll(
      /^\s*r(?:\.With\([^\n]*\))?\.(Get|Head|Post|Patch|Put|Delete)\("([^"]+)"/gm,
    ),
    (match) => `${match[1].toUpperCase()} ${match[2]}`,
  );
  expect(documented.map((op) => `${op.method} ${op.path}`).sort()).toEqual(
    routes.sort(),
  );
  await page.goto("/api-docs/");
  await expect(
    page.getByRole("heading", { name: "AssetHub API Docs", exact: true }),
  ).toBeVisible();
  expect(
    await page
      .getByRole("navigation", { name: "API endpoints" })
      .getByRole("link")
      .count(),
  ).toBe(documented.length);
  for (const op of documented) {
    await operation(page, op.operationId!);
    await expect(
      page.getByRole("heading", { name: op.summary!, exact: true }),
    ).toBeVisible();
    for (const status of Object.keys(op.responses!)) {
      const sample = page
        .getByRole("region", { name: "Response examples" })
        .locator("details")
        .filter({
          has: page.locator(`summary strong:text-is("HTTP ${status}")`),
        })
        .first();
      if ((await sample.getAttribute("open")) === null)
        await sample.locator("summary").first().click();
      await expect(sample.locator(".docs-code").first()).toBeVisible();
    }
  }
  const exported = await page.request.get("/api-docs/openapi.json");
  expect(exported.status()).toBe(200);
  expect(await exported.json()).toEqual(spec);
  const exportedYaml = await page.request.get("/api-docs/openapi.yaml");
  expect(exportedYaml.status()).toBe(200);
  expect(await exportedYaml.text()).toBe(
    readFileSync("api/openapi.yaml", "utf8"),
  );
  await page.getByLabel("Search API endpoints").fill("signed-url");
  await expect(
    page.getByRole("navigation", { name: "API endpoints" }).getByRole("link"),
  ).toHaveCount(1);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/api-docs/#createUpload");
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  await signInAdmin();
  await page.goto("/");
  const link = page
    .getByRole("navigation", { name: "Main navigation" })
    .getByRole("link", { name: "API Docs", exact: true });
  await expect(link).toHaveAttribute("target", "_blank");
  await expect(link).toHaveAttribute("rel", "noopener noreferrer");
  const opened = context.waitForEvent("page");
  // At mobile width, reveal the workspace sidebar first.
  await page
    .getByRole("button", { name: "Open navigation", exact: true })
    .click();
  await link.click();
  const docsTab = await opened;
  await expect(docsTab).toHaveURL(/\/api-docs\/$/);
  await expect(
    docsTab.getByRole("heading", { name: "AssetHub API Docs", exact: true }),
  ).toBeVisible();
  await expect(page).toHaveURL("http://127.0.0.1:18080/");
  await docsTab.close();
});

test("API Docs console sends real session and key requests, multipart and raw bytes, with live success and errors", async ({
  page,
  signInAdmin,
}) => {
  test.setTimeout(90000);
  const pageErrors: string[] = [];
  page.on("pageerror", (error) => pageErrors.push(error.message));
  await page.goto("/api-docs/#settings");
  await selectOption(
    page,
    "Console authentication",
    "Anonymous (no credentials)",
  );
  await send(page, 401);
  expect((await body(page)).error.code).toBe("unauthorized");
  await signInAdmin();
  await selectOption(page, "Console authentication", "Dashboard session");
  await send(page, 200);
  expect((await body(page)).asset_limit_bytes).toBe(536870912);
  await operation(page, "createProject");
  await page.getByLabel("JSON request body").fill(
    JSON.stringify({
      name: `Docs integration ${Date.now()}`,
      description: "Created through API Docs",
      quota_bytes: 10485760,
    }),
  );
  await send(page, 201);
  const project = await body(page);
  expect(project.id).toMatch(/^[0-9a-f-]{36}$/);
  await page.getByLabel("JSON request body").fill('{"unexpected":true}');
  await send(page, 400);
  expect((await body(page)).error.code).toBe("invalid_input");
  await operation(page, "createKey");
  await page.getByLabel("JSON request body").fill(
    JSON.stringify({
      project_id: project.id,
      name: "Docs integration",
      scopes: ["assets:read", "assets:write", "assets:sign"],
    }),
  );
  await send(page, 201);
  const key = await body(page);
  await selectOption(page, "Console authentication", "Project API key");
  await page.getByLabel("Project API key", { exact: true }).fill(key.secret);
  await operation(page, "uploadSmallAsset");
  await page.getByLabel("form: project_id").fill(project.id);
  const bytes = Buffer.from("hello world\n");
  await page.getByLabel("Request file").setInputFiles({
    name: "docs-multipart.txt",
    mimeType: "text/plain",
    buffer: bytes,
  });
  await send(page, 202);
  const small = await body(page);
  expect(small.size_bytes).toBe(bytes.length);
  await expect
    .poll(
      async () =>
        (await (await page.request.get(`/v1/assets/${small.id}`)).json())
          .status,
    )
    .toBe("ready");
  await operation(page, "headContent");
  await page.getByLabel("path: id").fill(small.id);
  await send(page, 200);
  await expect(consolePanel(page).locator(".docs-live")).toContainText(
    "(no response body)",
  );
  await operation(page, "getContent");
  await page.getByLabel("path: id").fill(small.id);
  await page.getByLabel("header: Range").fill("bytes=0-4");
  await send(page, 206);
  await expect(consolePanel(page).locator(".docs-live")).toContainText("hello");
  await page.getByLabel("header: Range").fill("bytes=100-200");
  await send(page, 416);
  expect((await body(page)).error).toBe("range_not_satisfiable");
  await operation(page, "createUpload");
  await page.getByLabel("JSON request body").fill(
    JSON.stringify({
      project_id: project.id,
      name: "docs-resumable.txt",
      size_bytes: bytes.length,
    }),
  );
  await send(page, 201);
  const upload = await body(page);
  await operation(page, "completeUpload");
  await page.getByLabel("path: id").fill(upload.id);
  await send(page, 409);
  expect((await body(page)).error.code).toBe("conflict");
  await operation(page, "uploadPart");
  await page.getByLabel("path: id").fill(upload.id);
  await page.getByLabel("path: index").fill("0");
  await page.getByLabel("Request file").setInputFiles({
    name: "part.bin",
    mimeType: "application/octet-stream",
    buffer: bytes,
  });
  await page.getByLabel("header: X-Content-SHA256").fill("0".repeat(64));
  await send(page, 422);
  expect((await body(page)).error.code).toBe("integrity_failed");
  await page.getByLabel("header: X-Content-SHA256").fill("");
  await send(page, 200);
  expect((await body(page)).received).toBe(true);
  await operation(page, "completeUpload");
  await page.getByLabel("path: id").fill(upload.id);
  await send(page, 202);
  expect((await body(page)).size_bytes).toBe(bytes.length);
  await operation(page, "settings");
  await send(page, 403);
  expect((await body(page)).error.code).toBe("forbidden");
  // Keys typed into the console are held only in React state.
  expect(
    await page.evaluate(
      (secret) =>
        JSON.stringify([localStorage, sessionStorage]).includes(secret),
      key.secret,
    ),
  ).toBe(false);
  await operation(page, "getContent");
  await page
    .getByLabel("path: id")
    .fill("99999999-9999-4999-8999-999999999999");
  await send(page, 404);
  expect((await body(page)).error.code).toBe("not_found");
  await selectOption(page, "Console authentication", "Dashboard session");
  await operation(page, "revokeKey");
  await page.getByLabel("path: id").fill(key.key.id);
  await page.getByLabel("query: project_id").fill(project.id);
  await send(page, 200);
  await selectOption(page, "Console authentication", "Project API key");
  await page.getByLabel("Project API key", { exact: true }).fill(key.secret);
  await operation(page, "listAssets");
  await send(page, 401);
  expect((await body(page)).error.code).toBe("unauthorized");
  expect(pageErrors).toEqual([]);
  await page.screenshot({
    path: ".local/screenshots/api-docs-console.png",
    fullPage: true,
  });
});
