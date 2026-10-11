import { test as base, expect, type APIRequestContext } from "@playwright/test";

type AdminSession = {
  cookies: Awaited<ReturnType<APIRequestContext["storageState"]>>["cookies"];
  csrfToken: string;
};

export const test = base.extend<
  { signInAdmin: () => Promise<string> },
  { adminSession: AdminSession }
>({
  // Setup uses a real session shared across files in this worker. Keep cookies
  // in memory and attach them explicitly, so anonymous and sign-in workflows
  // still start with fresh browser contexts and create their own sessions.
  adminSession: [
    async ({ playwright }, provide, workerInfo) => {
      const baseURL = workerInfo.project.use.baseURL;
      if (!baseURL) throw new Error("Browser fixture baseURL is required");
      const request = await playwright.request.newContext({ baseURL });
      try {
        const response = await request.post("/v1/auth/login", {
          headers: { Origin: new URL(baseURL).origin },
          data: {
            email: "browser@example.test",
            password: "local-browser-test-password",
          },
        });
        expect(response.status(), await response.text()).toBe(200);
        const { csrf_token } = (await response.json()) as {
          csrf_token: string;
        };
        expect(csrf_token).toBeTruthy();
        await provide({
          cookies: (await request.storageState()).cookies,
          csrfToken: csrf_token,
        });
      } finally {
        await request.dispose();
      }
    },
    { scope: "worker" },
  ],
  signInAdmin: async ({ page, adminSession }, provide) => {
    await provide(async () => {
      await page.context().addCookies(adminSession.cookies);
      return adminSession.csrfToken;
    });
  },
});

export { expect };
