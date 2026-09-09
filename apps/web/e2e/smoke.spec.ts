import { expect, test } from "@playwright/test";

/**
 * Critical-path smoke: auth (bootstrap or login) → overview dashboard loads.
 * Against a fresh Compose stack this uses first-run bootstrap.
 */
test("login/bootstrap then overview loads", async ({ page }) => {
  const email = process.env.E2E_EMAIL || "e2e@fleetdeck.local";
  const password = process.env.E2E_PASSWORD || "e2e-test-password-12";
  const displayName = process.env.E2E_DISPLAY_NAME || "E2E Admin";

  await page.goto("/");
  await expect(page.getByTestId("auth-form")).toBeVisible();

  const heading = page.locator("h1");
  const isBootstrap = (await heading.textContent())?.includes("Create admin") ?? false;

  if (isBootstrap) {
    await page.getByLabel("Display name").fill(displayName);
  }
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Password").fill(password);
  await page.getByTestId("auth-submit").click();

  await expect(page.getByTestId("overview-dashboard")).toBeVisible({ timeout: 45_000 });
  await expect(page.getByRole("heading", { name: "Dashboard" })).toBeVisible();
});
