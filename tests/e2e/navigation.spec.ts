import { test, expect } from "./fixtures";

test("sidebar navigation preserves the shell and updates only page content", async ({
  page,
}) => {
  test.setTimeout(60000);
  let sessionLoads = 0;
  let documents = 0;
  page.on("request", (request) => {
    if (new URL(request.url()).pathname === "/v1/auth/me") sessionLoads++;
    if (request.isNavigationRequest() && request.frame() === page.mainFrame())
      documents++;
  });
  const login = await page.request.post("/v1/auth/login", {
    headers: { Origin: "http://127.0.0.1:18080" },
    data: {
      email: "browser@example.test",
      password: "local-browser-test-password",
    },
  });
  expect(login.status(), await login.text()).toBe(200);
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "Workspace overview", exact: true }),
  ).toBeVisible();
  const sidebar = await page.locator(".sidebar").elementHandle();
  const header = await page.locator(".topbar").elementHandle();
  const storage = await page.locator(".storage-card").elementHandle();
  expect(sidebar).not.toBeNull();
  expect(header).not.toBeNull();
  expect(storage).not.toBeNull();
  const draft = "Unsubmitted header search";
  await page.getByLabel("Search workspace assets", { exact: true }).fill(draft);
  const routes = [
    ["Projects", "Projects"],
    ["File Manager", "File Manager"],
    ["Uploads", "Uploads"],
    ["API Keys", "API Keys"],
    ["Activity Logs", "Activity Logs"],
    ["Settings", "Settings"],
    ["Overview", "Workspace overview"],
  ];
  for (const [link, heading] of routes) {
    await page
      .getByRole("navigation", { name: "Main navigation" })
      .getByRole("link", { name: link, exact: true })
      .click();
    await expect(
      page.getByRole("heading", { name: heading, exact: true }),
    ).toBeVisible();
    await expect(
      page
        .getByRole("navigation", { name: "Main navigation" })
        .getByRole("link", { name: link, exact: true }),
    ).toHaveAttribute("aria-current", "page");
    expect(await sidebar!.evaluate((element) => element.isConnected)).toBe(
      true,
    );
    expect(await header!.evaluate((element) => element.isConnected)).toBe(true);
    expect(await storage!.evaluate((element) => element.isConnected)).toBe(
      true,
    );
    await expect(
      page.getByLabel("Search workspace assets", { exact: true }),
    ).toHaveValue(draft);
    await expect(page.locator(".boot-screen")).toHaveCount(0);
  }
  await page.goBack();
  await expect(
    page.getByRole("heading", { name: "Settings", exact: true }),
  ).toBeVisible();
  await page.goForward();
  await expect(
    page.getByRole("heading", { name: "Workspace overview", exact: true }),
  ).toBeVisible();
  expect(await sidebar!.evaluate((element) => element.isConnected)).toBe(true);
  expect(sessionLoads).toBe(1);
  expect(documents).toBe(1);
  // Query navigation replaces only content, including a new query on File Manager itself.
  await page
    .getByLabel("Search workspace assets", { exact: true })
    .fill("navigation-nonexistent-one");
  await page
    .getByRole("button", { name: "Search assets", exact: true })
    .click();
  await expect(page.getByLabel("Search files", { exact: true })).toHaveValue(
    "navigation-nonexistent-one",
  );
  await page
    .getByLabel("Search files", { exact: true })
    .fill("local table query");
  await page
    .getByLabel("Search workspace assets", { exact: true })
    .fill("navigation-nonexistent-two");
  await page
    .getByRole("button", { name: "Search assets", exact: true })
    .click();
  await expect(page.getByLabel("Search files", { exact: true })).toHaveValue(
    "navigation-nonexistent-two",
  );
  await page
    .getByLabel("Search files", { exact: true })
    .fill("another local query");
  await page
    .getByRole("button", { name: "Search assets", exact: true })
    .click();
  await expect(page.getByLabel("Search files", { exact: true })).toHaveValue(
    "navigation-nonexistent-two",
  );
  expect(await header!.evaluate((element) => element.isConnected)).toBe(true);
  expect(sessionLoads).toBe(1);
  expect(documents).toBe(1);
  await page.getByRole("button", { name: "Sign out", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Welcome back", exact: true }),
  ).toBeVisible();
  await expect(page.locator(".sidebar")).toHaveCount(0);
  await page.goto("/files/");
  await expect(page).toHaveURL(/\/login\/$/);
  await expect(page.locator(".sidebar")).toHaveCount(0);
});
