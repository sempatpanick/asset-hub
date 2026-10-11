import { createHash } from "node:crypto";
import { test, expect } from "./fixtures";
import { selectOption } from "./select";

test("searchable dropdowns scroll through real projects and tables page filtered history", async ({
  page,
  signInAdmin,
}) => {
  test.setTimeout(120000);
  const prefix = `Controls ${Date.now()}`;
  const origin = "http://127.0.0.1:18080";
  const csrf_token = await signInAdmin();
  const headers = { Origin: origin, "X-CSRF-Token": csrf_token };
  const projects: { id: string; name: string }[] = [];
  for (let i = 0; i < 65; i++) {
    const response = await page.request.post("/v1/projects", {
      headers,
      data: {
        name: `${prefix} ${i.toString().padStart(2, "0")}`,
        description: "Persistent dropdown pagination",
        quota_bytes: 1024 ** 3,
      },
    });
    expect(response.status()).toBe(201);
    projects.push(await response.json());
  }
  await page.goto("/files/");
  await page
    .getByRole("combobox", { name: "Project filter", exact: true })
    .click();
  const list = page.getByRole("listbox", {
    name: "Project filter",
    exact: true,
  });
  await expect(list.getByRole("option")).toHaveCount(30);
  await list.evaluate((el) => el.scrollTo(0, el.scrollHeight));
  await expect(list.getByRole("option")).toHaveCount(60);
  await page
    .getByRole("combobox", { name: "Search project filter", exact: true })
    .fill(projects[0].name);
  await expect(list.getByRole("option")).toHaveCount(1);
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("Enter");
  await expect(
    page.getByRole("combobox", { name: "Project filter", exact: true }),
  ).toContainText(projects[0].name);
  // Escape closes a popup inside a modal without closing the parent dialog.
  await page.getByRole("button", { name: "Upload files", exact: true }).click();
  await page
    .getByRole("combobox", { name: "Upload project", exact: true })
    .click();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toBeVisible();
  await expect(
    page.getByRole("combobox", { name: "Upload project", exact: true }),
  ).toHaveAttribute("aria-expanded", "false");
  await page.getByRole("button", { name: "Close dialog", exact: true }).click();
  for (let i = 0; i < 13; i++) {
    const response = await page.request.post("/v1/uploads", {
      headers,
      data: {
        project_id: projects[0].id,
        name: `${prefix}-transfer-${i.toString().padStart(2, "0")}.txt`,
        size_bytes: 3,
      },
    });
    expect(response.status()).toBe(201);
  }
  await page.goto("/uploads/");
  await selectOption(page, "Upload project filter", projects[0].name);
  await selectOption(page, "Page size", "10");
  await expect(page.locator("tbody tr")).toHaveCount(10);
  await page.getByRole("button", { name: "Next page" }).click();
  await expect(page.locator("tbody tr")).toHaveCount(3);
  await expect(page.getByRole("button", { name: "Next page" })).toBeDisabled();
  // This oldest transfer is outside the initial page; the API must filter first.
  await page
    .getByLabel("Search uploads", { exact: true })
    .fill(`${prefix}-transfer-00`);
  await expect(page.locator("tbody tr")).toHaveCount(1);
  await expect(page.locator("tbody")).toContainText(
    `${prefix}-transfer-00.txt`,
  );
  await expect(
    page.getByRole("button", { name: "Previous page" }),
  ).toBeDisabled();
  await selectOption(page, "Upload status filter", "ready");
  await expect(
    page.getByText("No matching uploads", { exact: true }),
  ).toBeVisible();
  await selectOption(page, "Upload status filter", "receiving");
  await expect(page.locator("tbody tr")).toHaveCount(1);
  await page.goto("/projects/");
  await page.getByLabel("Search projects", { exact: true }).fill(prefix);
  await selectOption(page, "Page size", "25");
  await expect(page.locator(".project-card")).toHaveCount(25);
  await page.getByRole("button", { name: "Next page" }).click();
  await page.getByRole("button", { name: "Next page" }).click();
  await expect(page.locator(".project-card")).toHaveCount(15);
  await selectOption(page, "Project role filter", "viewer");
  await expect(
    page.getByText("No matching projects", { exact: true }),
  ).toBeVisible();
  await page.goto("/activity/");
  await selectOption(page, "Project filter", projects[0].name);
  await page.getByLabel("Search activity", { exact: true }).fill(prefix);
  await selectOption(page, "Activity action filter", "project");
  await expect(page.locator(".activity-row")).toHaveCount(1);
  await selectOption(page, "Page size", "10");
  await expect(page.locator(".activity-row")).toContainText(
    "project · created",
  );
  await selectOption(page, "Activity action filter", "key");
  await expect(
    page.getByText("No matching activity", { exact: true }),
  ).toBeVisible();
  // Delay the real response to exercise a popup while empty results become rows.
  let release = () => {};
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/v1/activity?**", async (route) => {
    const response = await route.fetch();
    if (
      new URL(route.request().url()).searchParams.get("action") === "project."
    )
      await gate;
    await route.fulfill({ response });
  });
  await selectOption(page, "Activity action filter", "project");
  await page.getByRole("combobox", { name: "Page size", exact: true }).click();
  await page
    .getByRole("combobox", { name: "Search page size", exact: true })
    .fill("100");
  release();
  await expect(page.locator(".activity-row")).toHaveCount(1);
  await page.getByRole("option", { name: "100", exact: true }).click();
  await expect(
    page.getByRole("combobox", { name: "Page size", exact: true }),
  ).toContainText("100");
  await page.unrouteAll({ behavior: "wait" });
  await page.screenshot({
    path: ".local/screenshots/search-controls.png",
    fullPage: true,
  });
});

test("header search and live pages reflect persistent changes from another client", async ({
  page,
  signInAdmin,
}) => {
  test.setTimeout(90000);
  const origin = "http://127.0.0.1:18080";
  const prefix = `Live ${Date.now()}`;
  const csrf_token = await signInAdmin();
  const headers = { Origin: origin, "X-CSRF-Token": csrf_token };
  await page.goto("/projects/");
  await page.getByLabel("Search projects", { exact: true }).fill(prefix);
  await expect(
    page.getByText("No matching projects", { exact: true }),
  ).toBeVisible();
  const response = await page.request.post("/v1/projects", {
    headers,
    data: {
      name: prefix,
      description: "Created in another client",
      quota_bytes: 1024 ** 3,
    },
  });
  expect(response.status()).toBe(201);
  const project = await response.json();
  await expect(
    page.getByRole("heading", { name: prefix, exact: true }),
  ).toBeVisible({ timeout: 12000 });
  await page.goto("/");
  await expect(page.getByText("Total files", { exact: true })).toBeVisible();
  const beforeFiles = Number(
    (
      await page
        .locator(".stat-card")
        .filter({ hasText: "Total files" })
        .locator(".stat-value")
        .innerText()
    ).replaceAll(",", ""),
  );
  const beforeStorage = await page.locator(".storage-card p").innerText();
  const name = `${prefix}.txt`;
  const content = Buffer.alloc(1024 * 1024, 65);
  const uploadResponse = await page.request.post("/v1/uploads", {
    headers,
    data: { project_id: project.id, name, size_bytes: content.length },
  });
  expect(uploadResponse.status()).toBe(201);
  const upload = await uploadResponse.json();
  const part = await page.request.put(`/v1/uploads/${upload.id}/parts/0`, {
    headers: {
      ...headers,
      "Content-Type": "application/octet-stream",
      "X-Content-SHA256": createHash("sha256").update(content).digest("hex"),
    },
    data: content,
  });
  expect(part.status()).toBe(200);
  const completed = await page.request.post(
    `/v1/uploads/${upload.id}/complete`,
    { headers },
  );
  expect(completed.status()).toBe(202);
  const asset = await completed.json();
  await expect(
    page
      .locator(".stat-card")
      .filter({ hasText: "Total files" })
      .locator(".stat-value"),
  ).toHaveText(String(beforeFiles + 1), { timeout: 12000 });
  await expect(page.locator(".storage-card p")).not.toHaveText(beforeStorage, {
    timeout: 12000,
  });
  await page.keyboard.press("/");
  await expect(
    page.getByLabel("Search workspace assets", { exact: true }),
  ).toBeFocused();
  await page.getByLabel("Search workspace assets", { exact: true }).fill(name);
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(
    new RegExp(`/files/\\?q=${encodeURIComponent(name)}`),
  );
  await expect(page.getByLabel("Search files", { exact: true })).toHaveValue(
    name,
  );
  await expect(page.getByRole("button", { name, exact: true })).toBeVisible();
  // Search on File Manager itself updates the current list.
  await page
    .getByLabel("Search workspace assets", { exact: true })
    .fill(prefix);
  await page.keyboard.press("Enter");
  await expect(page.getByLabel("Search files", { exact: true })).toHaveValue(
    prefix,
  );
  const renamed = `${prefix} renamed.txt`;
  expect(
    (
      await page.request.patch(`/v1/assets/${asset.id}`, {
        headers,
        data: { name: renamed },
      })
    ).ok(),
  ).toBe(true);
  await expect(
    page.getByRole("button", { name: renamed, exact: true }),
  ).toBeVisible({ timeout: 12000 });
  await expect(
    page.getByRole("button", { name, exact: true }),
  ).not.toBeVisible();
  await page.getByLabel("Search files", { exact: true }).fill("");
  const filteredLoaded = page.waitForResponse((response) => {
    const url = new URL(response.url());
    return (
      url.pathname === "/v1/assets" &&
      url.searchParams.get("project_id") === project.id &&
      url.searchParams.get("q") === "" &&
      response.ok()
    );
  });
  await selectOption(page, "Project filter", prefix);
  await filteredLoaded;
  await expect(
    page.getByRole("button", { name: renamed, exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("Loading your workspace…", { exact: true }),
  ).not.toBeVisible();
  await page.route("**/v1/assets?**", (route) => route.abort());
  await expect(page.locator("main").getByRole("alert")).toBeVisible({
    timeout: 12000,
  });
  await expect(
    page.getByRole("button", { name: renamed, exact: true }),
  ).toBeVisible();
  await page.unroute("**/v1/assets?**");
  await page.evaluate(() => window.dispatchEvent(new Event("online")));
  await expect(page.locator("main").getByRole("alert")).not.toBeVisible({
    timeout: 12000,
  });
  await page.screenshot({
    path: ".local/screenshots/live-search.png",
    fullPage: true,
  });
});
