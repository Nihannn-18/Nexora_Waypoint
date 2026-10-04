import { test, expect } from '@playwright/test';
import { apiAs, driverFirstLeg, loginAs } from './helpers';

// Judges test the Driver at phone width.
test.use({ viewport: { width: 402, height: 850 } });

test.describe('Driver workspace', () => {
  test('cockpit loads the driver run sheet', async ({ page }) => {
    await loginAs(page, 'driver');
    await expect(page).toHaveURL(/\/driver/);
    await expect(page.getByText(/DRIVER TASKS/i)).toBeVisible();
    await expect(page.getByText(/live sync on|syncing|offline/i).first()).toBeVisible();
  });

  test('a failed delivery requires a reason before it is saved', async ({ page, request }) => {
    const { ctx, token } = await apiAs('driver');
    const legId = await driverFirstLeg(ctx, token);
    await ctx.dispose();

    await loginAs(page, 'driver');
    await page.goto(`/driver/stops/${legId}/outcome?o=FAILED`);
    await expect(page.getByRole('heading', { name: /record outcome/i })).toBeVisible();

    // Attempt to save without a reason: the screen refuses and says why.
    await page.getByRole('button', { name: /save outcome/i }).click();
    await expect(page.getByText(/choose a reason/i)).toBeVisible();

    // Choose a reason chip, then the outcome saves to the phone outbox.
    await page.getByRole('button', { name: 'Outlet closed' }).click();
    await page.getByRole('button', { name: /save outcome/i }).click();
    await page.waitForURL(/\/driver\/saved\//, { timeout: 20_000 });
  });

  test('offline is a neutral state and actions are saved on the phone', async ({ page, context }) => {
    await loginAs(page, 'driver');
    await expect(page.getByText(/DRIVER TASKS/i)).toBeVisible();
    // Losing signal flips the shell to its neutral offline state; the app
    // listens for the browser's offline event.
    await context.setOffline(true);
    await page.evaluate(() => window.dispatchEvent(new Event('offline')));
    await expect(page.getByText(/no signal/i)).toBeVisible({ timeout: 20_000 });
    await expect(page.getByText(/saved on this phone/i)).toBeVisible();
    await expect(page.getByText(/offline · saving on this phone/i)).toBeVisible();
    await context.setOffline(false);
    await page.evaluate(() => window.dispatchEvent(new Event('online')));
  });
});
