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

test("upload timeout settings validate, persist and set new session expiry", async ({
  page,
  signInAdmin,
}) => {
  const csrf = await signInAdmin();
  const headers = { Origin: "http://127.0.0.1:18080", "X-CSRF-Token": csrf };
  const requestTimeout = () =>
    page.getByLabel("Upload request timeout (seconds)", { exact: true });
  const responseTimeout = () =>
    page.getByLabel("Upload response timeout (seconds)", { exact: true });
  const lifetime = () =>
    page.getByLabel("Resumable session lifetime (hours)", { exact: true });
  await page.goto("/settings/");
  await expect(requestTimeout()).toHaveValue("120");
  await expect(responseTimeout()).toHaveValue("150");
  await expect(lifetime()).toHaveValue("24");
  await requestTimeout().fill("300");
  await responseTimeout().fill("329");
  await page.getByRole("button", { name: "Save upload settings" }).click();
  await expect(
    page.locator("#upload_response_timeout_seconds-error"),
  ).toHaveText(
    "Response timeout must be at least 30 seconds longer than request timeout.",
  );
  await expect(responseTimeout()).toBeFocused();
  await responseTimeout().fill("360");
  await lifetime().fill("169");
  await page.getByRole("button", { name: "Save upload settings" }).click();
  await expect(page.locator("#upload_session_lifetime_hours-error")).toHaveText(
    "Enter a whole number from 1 to 168.",
  );
  await lifetime().fill("48");
  await page.getByRole("button", { name: "Save upload settings" }).click();
  await expect(page.getByRole("status")).toHaveText("Upload settings saved.");
  await page.reload();
  await expect(requestTimeout()).toHaveValue("300");
  await expect(responseTimeout()).toHaveValue("360");
  await expect(lifetime()).toHaveValue("48");
  const saved = await (await page.request.get("/v1/settings")).json();
  expect(saved).toMatchObject({
    upload_request_timeout_seconds: 300,
    upload_response_timeout_seconds: 360,
    upload_session_lifetime_hours: 48,
  });
  const projectResponse = await page.request.post("/v1/projects", {
    headers,
    data: { name: `Timeout browser ${Date.now()}`, quota_bytes: 1 << 20 },
  });
  expect(projectResponse.status()).toBe(201);
  const project = await projectResponse.json();
  const keyResponse = await page.request.post("/v1/api-keys", {
    headers,
    data: {
      project_id: project.id,
      name: "Timeout session writer",
      scopes: ["assets:read", "assets:write"],
    },
  });
  expect(keyResponse.status()).toBe(201);
  const key = await keyResponse.json();
  const createSession = async () => {
    const response = await page.request.post("/v1/uploads", {
      headers: { Authorization: `Bearer ${key.secret}` },
      data: {
        project_id: project.id,
        name: "timeout-session.txt",
        size_bytes: 1,
      },
    });
    expect(response.status()).toBe(201);
    const createdSession = (await response.json()) as { id: string };
    const persisted = await page.request.get(
      `/v1/uploads/${createdSession.id}`,
      {
        headers: { Authorization: `Bearer ${key.secret}` },
      },
    );
    expect(persisted.status()).toBe(200);
    return (await persisted.json()) as {
      id: string;
      created_at: string;
      expires_at: string;
    };
  };
  try {
    const original = await createSession();
    expect(
      (Date.parse(original.expires_at) - Date.parse(original.created_at)) /
        3600000,
    ).toBeCloseTo(48, 3);
    await lifetime().fill("1");
    await page.getByRole("button", { name: "Save upload settings" }).click();
    await expect(page.getByRole("status")).toHaveText("Upload settings saved.");
    const fresh = await createSession();
    expect(
      (Date.parse(fresh.expires_at) - Date.parse(fresh.created_at)) / 3600000,
    ).toBeCloseTo(1, 3);
    const inspected = await page.request.get(`/v1/uploads/${original.id}`, {
      headers: { Authorization: `Bearer ${key.secret}` },
    });
    expect(inspected.status()).toBe(200);
    expect((await inspected.json()).expires_at).toBe(original.expires_at);
    await page.screenshot({
      path: ".local/screenshots/upload-timeouts-desktop.png",
      fullPage: true,
    });
    await page.setViewportSize({ width: 390, height: 844 });
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await page.screenshot({
      path: ".local/screenshots/upload-timeouts-mobile.png",
      fullPage: true,
    });
  } finally {
    const restored = await page.request.patch("/v1/settings", {
      headers,
      data: {
        upload_request_timeout_seconds: 120,
        upload_response_timeout_seconds: 150,
        upload_session_lifetime_hours: 24,
      },
    });
    expect(restored.status()).toBe(200);
  }
});
