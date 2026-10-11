import { test, expect } from "./fixtures";
import { selectOption } from "./select";

test("project IDs and password-confirmed key copying persist after reload", async ({
  page,
  context,
  signInAdmin,
}) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  const origin = "http://127.0.0.1:18080";
  const password = "local-browser-test-password";
  const csrf_token = await signInAdmin();
  const name = `Copy keys ${Date.now()}`;
  const createdProject = await page.request.post("/v1/projects", {
    headers: { Origin: origin, "X-CSRF-Token": csrf_token },
    data: {
      name,
      description: "Password-confirmed copying",
      quota_bytes: 1 << 30,
    },
  });
  expect(createdProject.status()).toBe(201);
  const project = (await createdProject.json()) as { id: string };
  await page.goto("/api-keys/");
  await selectOption(page, "Project filter", name);
  await expect(page.locator(".project-id code").first()).toHaveText(project.id);
  await page
    .getByRole("button", { name: `Copy project ID ${project.id}`, exact: true })
    .click();
  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toBe(project.id);
  await page
    .getByRole("button", { name: "Create API key", exact: true })
    .first()
    .click();
  await page.getByLabel("Key name", { exact: true }).fill("Copyable key");
  const creating = page.waitForResponse(
    (response) =>
      response.url().endsWith("/v1/api-keys") &&
      response.request().method() === "POST",
  );
  await page.getByRole("button", { name: "Create key", exact: true }).click();
  const { secret, key } = (await (await creating).json()) as {
    secret: string;
    key: { id: string; secret_available: boolean };
  };
  expect(key.secret_available).toBe(true);
  const dialog = page.getByRole("dialog");
  await expect(
    dialog.getByRole("heading", { name: "Save your API key" }),
  ).toBeVisible();
  await expect(dialog).not.toContainText(secret);
  await expect(
    dialog.getByRole("button", { name: "Copy secret", exact: true }),
  ).toBeDisabled();
  await dialog.getByLabel("Dashboard password").fill("incorrect-password");
  await dialog
    .getByRole("button", { name: "Copy secret", exact: true })
    .click();
  await expect(
    dialog.getByText("Incorrect dashboard password. Try again."),
  ).toBeVisible();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
    project.id,
  );
  await expect(dialog.getByLabel("Dashboard password")).toHaveValue("");
  await dialog.getByLabel("Dashboard password").fill(password);
  await dialog
    .getByRole("button", { name: "Copy secret", exact: true })
    .click();
  await expect(dialog.getByRole("status")).toHaveText("API key copied.");
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
    secret,
  );
  await dialog.getByRole("button", { name: "Done", exact: true }).click();
  await page.reload();
  await selectOption(page, "Project filter", name);
  const row = page.getByRole("row").filter({ hasText: "Copyable key" });
  await expect(row.locator(".project-id code")).toHaveText(project.id);
  await row
    .getByRole("button", { name: `Copy project ID ${project.id}`, exact: true })
    .click();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
    project.id,
  );
  await row
    .getByRole("button", { name: "Copy API key Copyable key", exact: true })
    .click();
  await expect(dialog.getByLabel("Dashboard password")).toHaveValue("");
  await expect(
    dialog.getByRole("button", { name: "Confirm and copy" }),
  ).toBeDisabled();
  await dialog.getByLabel("Dashboard password").fill(password);
  await dialog.getByRole("button", { name: "Confirm and copy" }).click();
  await expect(dialog.getByRole("status")).toHaveText("API key copied.");
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
    secret,
  );
  await dialog.getByRole("button", { name: "Done", exact: true }).click();
  const metadata = await page.request.get(
    `/v1/api-keys?project_id=${project.id}`,
  );
  expect(metadata.status()).toBe(200);
  expect(await metadata.text()).not.toContain(secret);
  await page.screenshot({
    path: ".local/screenshots/api-key-copy-desktop.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await row
    .getByRole("button", { name: "Copy API key Copyable key", exact: true })
    .click();
  await expect(dialog).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: ".local/screenshots/api-key-copy-mobile.png",
    fullPage: true,
  });
  await page.keyboard.press("Escape");
  await expect(dialog).not.toBeVisible();
  await page.setViewportSize({ width: 1280, height: 720 });
  await row.getByRole("button", { name: "Revoke", exact: true }).click();
  await expect(row.getByText("Revoked", { exact: true })).toBeVisible();
  await expect(
    row.getByRole("button", { name: "Copy API key Copyable key", exact: true }),
  ).toBeDisabled();
});
