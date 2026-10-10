import { selectOption } from "./select";
import { test, expect } from "@playwright/test";

test("backups disabled allows real uploads and hides restore", async ({
  page,
}) => {
  test.skip(
    process.env.BACKUP_ENABLED !== "false",
    "requires primary-only disposable fixture",
  );
  test.setTimeout(60000);
  const name = `primary-only-${Date.now()}.txt`;
  const project = `Primary only ${Date.now()}`;
  const data = Buffer.from("Persistent upload without an independent backup.");
  await page.goto("/login/");
  await page.getByLabel("Email address").fill("browser@example.test");
  await page
    .getByLabel("Password", { exact: true })
    .fill("local-browser-test-password");
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(
    page.getByText("Backups disabled", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "New project", exact: true }).click();
  await page.getByLabel("Project name").fill(project);
  await page
    .getByRole("button", { name: "Create project", exact: true })
    .click();
  await expect(page.getByRole("dialog")).not.toBeVisible();
  await page.getByRole("link", { name: "Settings", exact: true }).click();
  await expect(
    page.getByText(
      "Backups are disabled. Ready assets rely on primary storage. Deleted assets cannot be restored.",
    ),
  ).toBeVisible();
  const settings = await (await page.request.get("/v1/settings")).json();
  expect(settings.features.backups).toBe(false);
  expect(settings.backup_retention_days).toBe(0);
  await page.getByRole("link", { name: "File Manager", exact: true }).click();
  await page.getByRole("button", { name: "Upload files", exact: true }).click();
  await selectOption(page, "Upload project", project);
  await page
    .locator('input[type="file"]')
    .setInputFiles({ name, mimeType: "text/plain", buffer: data });
  await page.getByRole("button", { name: "Start / resume uploads" }).click();
  await expect(
    page
      .locator(".transfer")
      .filter({ hasText: name })
      .getByText("queued", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Close dialog", exact: true }).click();
  await page.getByRole("link", { name: "Uploads", exact: true }).click();
  await expect(
    page
      .getByRole("row")
      .filter({ hasText: name })
      .getByText("ready", { exact: true }),
  ).toBeVisible({ timeout: 30000 });
  await page.reload();
  await page.getByRole("link", { name: "File Manager", exact: true }).click();
  await page.getByRole("button", { name, exact: true }).click();
  await page.getByRole("button", { name: "Create 15-minute link" }).click();
  const signed = await page
    .getByLabel("Signed URL", { exact: true })
    .inputValue();
  const download = await page.request.get(signed, {
    headers: { Range: "bytes=0-9" },
  });
  expect(download.status()).toBe(206);
  expect(await download.body()).toEqual(data.subarray(0, 10));
  await page.getByRole("button", { name: "Delete asset", exact: true }).click();
  await expect(
    page.getByText(
      "Delete this asset? Access stops immediately. Backups are disabled, so this asset cannot be restored.",
    ),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Confirm deletion", exact: true })
    .click();
  await expect(page.getByRole("dialog")).not.toBeVisible();
  expect((await page.request.get(signed)).status()).toBe(404);
  await selectOption(page, "Status filter", "deleted");
  await page.getByRole("button", { name, exact: true }).click();
  await expect(
    page.getByText(
      "Backups are disabled. This deleted asset cannot be restored.",
    ),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Restore retained backup" }),
  ).not.toBeVisible();
});
