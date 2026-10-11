import { test, expect } from "./fixtures";
test("invitation token is removed from the URL and creates a persistent account", async ({
  page,
  signInAdmin,
}) => {
  const email = `invited-${Date.now()}@example.test`;
  const password = "local-invitation-test-password";
  const csrf_token = await signInAdmin();
  const invitation = await page.request.post("/v1/invitations", {
    headers: { "X-CSRF-Token": csrf_token, Origin: "http://127.0.0.1:18080" },
    data: { email, role: "member" },
  });
  expect(invitation.status()).toBe(201);
  const { url } = await invitation.json();
  await page.context().clearCookies();
  await page.goto(url);
  await expect(
    page.getByRole("button", { name: "Create account", exact: true }),
  ).toBeEnabled();
  await expect(page).toHaveURL(/\/invite\/$/);
  await page.getByLabel("Display name").fill("Invited browser member");
  await page.getByLabel("Password", { exact: true }).fill(password);
  await page
    .getByRole("button", { name: "Create account", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "You’re all set" }),
  ).toBeVisible();
  await page.getByRole("link", { name: "Go to sign in" }).click();
  await page.getByLabel("Email address").fill(email);
  await page.getByLabel("Password", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Workspace overview" }),
  ).toBeVisible();
  await page.reload();
  await expect(
    page.getByRole("heading", { name: "Workspace overview" }),
  ).toBeVisible();
});
test("persistent dashboard, projects, keys and audit work", async ({
  page,
}) => {
  const errors: string[] = [];
  const projectName = `Browser integration ${Date.now()}`;
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto("/login/");
  await page.getByLabel("Email address").fill("browser@example.test");
  await page
    .getByLabel("Password", { exact: true })
    .fill("local-browser-test-password");
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Workspace overview" }),
  ).toBeVisible();
  await expect(page.getByText("Total files", { exact: true })).toBeVisible();
  await page.screenshot({
    path: ".local/screenshots/overview.png",
    fullPage: true,
  });
  await page.getByRole("button", { name: "New project", exact: true }).click();
  await page.getByLabel("Project name").fill(projectName);
  await page
    .getByLabel("Description", { exact: true })
    .fill("Created through the real persistent API");
  await page
    .getByRole("button", { name: "Create project", exact: true })
    .click();
  await expect(page.getByRole("dialog")).not.toBeVisible();
  await page.getByRole("link", { name: "Projects", exact: true }).click();
  await expect(page.getByRole("heading", { name: projectName })).toBeVisible();
  await page.reload();
  await expect(page.getByRole("heading", { name: projectName })).toBeVisible();
  await page.getByRole("link", { name: "API Keys", exact: true }).click();
  await page
    .getByRole("button", { name: "Create API key", exact: true })
    .first()
    .click();
  await page.getByLabel("Key name").fill("Browser test key");
  const creatingKey = page.waitForResponse(
    (response) =>
      response.url().endsWith("/v1/api-keys") &&
      response.request().method() === "POST",
  );
  await page.getByRole("button", { name: "Create key", exact: true }).click();
  const keyResponse = await creatingKey;
  expect(keyResponse.status()).toBe(201);
  const { key } = (await keyResponse.json()) as {
    key: { expires_at: string };
  };
  const expiry = Date.parse(key.expires_at);
  expect(expiry).toBeGreaterThan(Date.now() + 89 * 86400000);
  expect(expiry).toBeLessThanOrEqual(Date.now() + 90 * 86400000);
  await expect(
    page.getByRole("heading", { name: "Save your API key" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "I’ve saved it" }).click();
  await expect(page.getByText("Browser test key")).toBeVisible();
  await page.getByRole("button", { name: "Revoke", exact: true }).click();
  await expect(page.getByText("Revoked", { exact: true })).toBeVisible();
  await page.getByRole("link", { name: "Activity Logs", exact: true }).click();
  await expect(
    page.getByText("project · created", { exact: true }).first(),
  ).toBeVisible();
  await expect(
    page.getByText("key · revoked", { exact: true }).first(),
  ).toBeVisible();
  await page.getByRole("link", { name: "File Manager", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Upload files" }),
  ).toBeEnabled();
  await page.getByLabel("Search files").fill("nonexistent");
  await expect(page.getByText("No matching assets")).toBeVisible();
  await page.getByRole("link", { name: "Settings", exact: true }).click();
  await expect(
    page.getByText(
      "Uploads are enabled. New assets require verified encrypted primary storage and backup.",
    ),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Invite member" }),
  ).toBeVisible();
  expect(errors).toEqual([]);
  await page.getByRole("button", { name: "Sign out" }).click();
  await expect(
    page.getByRole("heading", { name: "Welcome back" }),
  ).toBeVisible();
});
test("mobile navigation and dialog remain within the viewport", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/login/");
  await page.getByLabel("Email address").fill("browser@example.test");
  await page
    .getByLabel("Password", { exact: true })
    .fill("local-browser-test-password");
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Workspace overview" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Open navigation" }).click();
  await page.getByRole("link", { name: "Projects", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Projects", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "New project", exact: true }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  const dialog = await page.getByRole("dialog").boundingBox();
  expect(dialog?.x).toBeGreaterThanOrEqual(15);
  expect(dialog?.y).toBeGreaterThan(0);
  await page.screenshot({
    path: ".local/screenshots/mobile.png",
    fullPage: true,
  });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).not.toBeVisible();
  await page.getByRole("button", { name: "Open navigation" }).click();
  await page.getByRole("link", { name: "File Manager", exact: true }).click();
  await expect(
    page.getByRole("combobox", { name: "File type filter", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("Loading your workspace…", { exact: true }),
  ).not.toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: ".local/screenshots/mobile-files.png",
    fullPage: true,
  });
  await page
    .getByRole("combobox", { name: "File type filter", exact: true })
    .click();
  await page
    .getByRole("combobox", { name: "Search file type filter", exact: true })
    .fill("Documents");
  const popup = await page.locator(".select-popup:popover-open").boundingBox();
  expect(popup?.x).toBeGreaterThanOrEqual(0);
  expect((popup?.x ?? 0) + (popup?.width ?? 0)).toBeLessThanOrEqual(390);
  await page
    .getByRole("option", { name: "Documents & archives", exact: true })
    .click();
  await page
    .getByLabel("Search workspace assets", { exact: true })
    .fill("mobile-nonexistent");
  await page
    .getByRole("button", { name: "Search assets", exact: true })
    .click();
  await expect(page.getByLabel("Search files", { exact: true })).toHaveValue(
    "mobile-nonexistent",
  );
  await expect(
    page.getByText("No matching assets", { exact: true }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
});
