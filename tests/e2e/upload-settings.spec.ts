import { test, expect } from "./fixtures";
import { selectOption } from "./select";

test("saved upload limits control parallel transfers and pause/resume", async ({
  page,
  signInAdmin,
}) => {
  test.setTimeout(120000);
  const csrf = await signInAdmin();
  const origin = "http://127.0.0.1:18080";
  const projectName = `Parallel images ${Date.now()}`;
  const created = await page.request.post("/v1/projects", {
    headers: { Origin: origin, "X-CSRF-Token": csrf },
    data: {
      name: projectName,
      description: "Concurrency browser test",
      quota_bytes: 1 << 25,
    },
  });
  expect(created.status()).toBe(201);
  const project = (await created.json()) as { id: string };
  const png = Buffer.from(
    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aWQAAAABJRU5ErkJggg==",
    "base64",
  );
  const images = (count: number, prefix: string) =>
    Array.from({ length: count }, (_, index) => ({
      name: `${prefix}-${index}.png`,
      mimeType: "image/png",
      buffer: png,
    }));
  const saveLimit = async (limit: number) => {
    await page.goto("/settings/");
    await page
      .getByLabel("Maximum concurrent uploads", { exact: true })
      .fill(String(limit));
    await page.getByRole("button", { name: "Save upload settings" }).click();
    await expect(page.getByRole("status")).toHaveText("Upload settings saved.");
    await page.reload();
    await expect(
      page.getByLabel("Maximum concurrent uploads", { exact: true }),
    ).toHaveValue(String(limit));
  };
  const openUpload = async (files: ReturnType<typeof images>) => {
    await page.getByRole("link", { name: "File Manager", exact: true }).click();
    await page
      .getByRole("button", { name: "Upload files", exact: true })
      .click();
    await selectOption(page, "Upload project", projectName);
    await page.locator('input[type="file"]').setInputFiles(files);
  };
  await saveLimit(2);
  await page
    .getByLabel("Maximum concurrent uploads", { exact: true })
    .fill("21");
  await page.getByRole("button", { name: "Save upload settings" }).click();
  await expect(
    page.locator(".upload-settings").getByRole("alert"),
  ).toContainText("Enter a whole number from 1 to 20.");
  expect(
    (await (await page.request.get("/v1/settings")).json()).upload_concurrency,
  ).toBe(2);
  await page.reload();
  const pausedImages = images(4, `paused-${Date.now()}`);
  await openUpload(pausedImages);
  let entered = 0;
  let release!: () => void;
  let barrier = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/v1/uploads/*/parts/*", async (route) => {
    entered++;
    await barrier;
    await route.continue();
  });
  try {
    await page.getByRole("button", { name: "Start / resume uploads" }).click();
    await expect.poll(() => entered).toBe(2);
    await page.getByRole("button", { name: "Pause transfer" }).click();
    await expect(
      page.getByRole("button", { name: "Start / resume uploads" }),
    ).toBeVisible();
    await expect(
      page.locator(".transfer").getByText("paused", { exact: true }),
    ).toHaveCount(2);
    expect(entered).toBe(2);
  } finally {
    release();
    await page.unrouteAll({ behavior: "wait" });
  }
  await page.getByRole("button", { name: "Start / resume uploads" }).click();
  await expect(
    page.locator(".transfer").getByText("queued", { exact: true }),
  ).toHaveCount(4, { timeout: 30000 });
  await page.getByRole("button", { name: "Close dialog", exact: true }).click();
  await saveLimit(20);
  await page.screenshot({
    path: ".local/screenshots/upload-settings-desktop.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(
    page.getByLabel("Maximum concurrent uploads", { exact: true }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: ".local/screenshots/upload-settings-mobile.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 1440, height: 1000 });
  const files = images(20, `twenty-${Date.now()}`);
  await openUpload(files);
  entered = 0;
  barrier = new Promise<void>((resolve) => {
    release = resolve;
  });
  // Hold real part requests to observe overlap; every request is then sent to
  // the persistent API. The Go integration test also holds 20 server bodies.
  await page.route("**/v1/uploads/*/parts/*", async (route) => {
    entered++;
    await barrier;
    await route.continue();
  });
  try {
    await page.getByRole("button", { name: "Start / resume uploads" }).click();
    await expect.poll(() => entered, { timeout: 15000 }).toBe(20);
    await expect(
      page.locator(".transfer").getByText("uploading", { exact: true }),
    ).toHaveCount(20);
  } finally {
    release();
    await page.unrouteAll({ behavior: "wait" });
  }
  await expect(
    page.locator(".transfer").getByText("queued", { exact: true }),
  ).toHaveCount(20, { timeout: 30000 });
  await page.getByRole("button", { name: "Close dialog", exact: true }).click();
  await expect
    .poll(
      async () => {
        const response = await page.request.get(
          `/v1/assets?project_id=${project.id}&limit=100`,
        );
        expect(response.status()).toBe(200);
        const { items } = (await response.json()) as {
          items: { name: string; status: string }[];
        };
        return items.filter(
          (asset) =>
            files.some((file) => file.name === asset.name) &&
            asset.status === "ready",
        ).length;
      },
      { timeout: 45000 },
    )
    .toBe(20);
  await page.reload();
  expect(
    (await (await page.request.get("/v1/settings")).json()).upload_concurrency,
  ).toBe(20);
});
