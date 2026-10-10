import { test, expect } from "@playwright/test";
test("multi-file upload persists through chunk verification and reload", async ({
  page,
}) => {
  test.setTimeout(60000);
  const project = `Upload browser ${Date.now()}`;
  const files = [
    {
      name: `preview-${Date.now()}.png`,
      mimeType: "image/png",
      buffer: Buffer.from(
        "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aWQAAAABJRU5ErkJggg==",
        "base64",
      ),
    },
    {
      name: `large-${Date.now()}.bin`,
      mimeType: "application/octet-stream",
      buffer: Buffer.alloc(8 * 1024 * 1024 + 301, 37),
    },
    {
      name: `notes-${Date.now()}.txt`,
      mimeType: "text/plain",
      buffer: Buffer.from(
        "Browser transfer to verified primary and encrypted backup.",
      ),
    },
  ];
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto("/login/");
  await page.getByLabel("Email address").fill("browser@example.test");
  await page
    .getByLabel("Password", { exact: true })
    .fill("local-browser-test-password");
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await page.getByRole("button", { name: "New project", exact: true }).click();
  await page.getByLabel("Project name").fill(project);
  await page
    .getByRole("button", { name: "Create project", exact: true })
    .click();
  await expect(page.getByRole("dialog")).not.toBeVisible();
  await page.getByRole("link", { name: "File Manager", exact: true }).click();
  await page.getByRole("button", { name: "Upload files", exact: true }).click();
  await page.getByLabel("Upload project").selectOption({ label: project });
  await page.locator('input[type="file"]').setInputFiles(files);
  await page.getByRole("button", { name: "Start / resume uploads" }).click();
  for (const file of files) {
    const transfer = page.locator(".transfer").filter({ hasText: file.name });
    await expect(transfer.getByText("queued", { exact: true })).toBeVisible({
      timeout: 30000,
    });
  }
  await page.screenshot({
    path: ".local/screenshots/upload-dialog.png",
    fullPage: true,
  });
  await page.getByRole("button", { name: "Close dialog", exact: true }).click();
  await page.getByRole("link", { name: "Uploads", exact: true }).click();
  for (const file of files) {
    const row = page.getByRole("row").filter({ hasText: file.name });
    await expect(row.getByText("ready", { exact: true })).toBeVisible({
      timeout: 30000,
    });
  }
  await page.reload();
  for (const file of files)
    await expect(
      page
        .getByRole("row")
        .filter({ hasText: file.name })
        .getByText("ready", { exact: true }),
    ).toBeVisible();
  await page.screenshot({
    path: ".local/screenshots/uploads.png",
    fullPage: true,
  });
  await page.getByRole("link", { name: "File Manager", exact: true }).click();
  for (const file of files)
    await expect(page.getByText(file.name, { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Grid view", exact: true }).click();
  const preview = page.getByRole("img", { name: files[0].name, exact: true });
  await expect(preview).toBeVisible();
  await expect
    .poll(() =>
      preview.evaluate((image: HTMLImageElement) => image.naturalWidth),
    )
    .toBeGreaterThan(0);
  await page.getByRole("button", { name: files[0].name, exact: true }).click();
  const detailsPreview = page
    .getByRole("dialog")
    .getByRole("img", { name: files[0].name, exact: true });
  await expect
    .poll(() =>
      detailsPreview.evaluate((image: HTMLImageElement) => image.naturalWidth),
    )
    .toBeGreaterThan(0);
  await page.getByRole("button", { name: "Create 15-minute link" }).click();
  const signed = await page
    .getByLabel("Signed URL", { exact: true })
    .inputValue();
  const signedResponse = await page.request.get(signed, {
    headers: { Range: "bytes=0-8" },
  });
  expect(signedResponse.status()).toBe(206);
  expect(
    (await signedResponse.body()).equals(files[0].buffer.subarray(0, 9)),
  ).toBe(true);
  const renamed = `renamed-${files[0].name}`;
  await page.getByLabel("File name", { exact: true }).fill(renamed);
  await page.getByRole("button", { name: "Save changes", exact: true }).click();
  await expect(page.getByRole("dialog")).not.toBeVisible();
  await page.getByRole("button", { name: renamed, exact: true }).click();
  await page.getByRole("button", { name: "Delete asset", exact: true }).click();
  await page
    .getByRole("button", { name: "Confirm deletion", exact: true })
    .click();
  await expect(page.getByRole("dialog")).not.toBeVisible();
  expect((await page.request.get(signed)).status()).toBe(404);
  await page.getByLabel("Status filter").selectOption("deleted");
  await page.getByRole("button", { name: renamed, exact: true }).click();
  await page.getByRole("button", { name: "Restore retained backup" }).click();
  await expect(page.getByRole("dialog")).not.toBeVisible();
  await page.getByLabel("Status filter").selectOption("ready");
  await expect(
    page.getByRole("button", { name: renamed, exact: true }),
  ).toBeVisible();
  expect((await page.request.get(signed)).status()).toBe(401);
  expect(errors).toEqual([]);
});
