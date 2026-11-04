import { expect, test } from "@playwright/test";

test("home page renders the hero", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Acme" })).toBeVisible();
});

test("dashboard redirects anonymous users to login", async ({ page }) => {
  await page.goto("/dashboard");
  await expect(page).toHaveURL(/\/login/);
});

test("health endpoint is ok", async ({ request }) => {
  const res = await request.get("/api/health");
  expect(res.ok()).toBeTruthy();
});
