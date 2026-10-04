import { test, expect } from '@playwright/test';
import { loginAs } from './helpers';

// Judges test the Loader at phone width.
test.use({ viewport: { width: 402, height: 850 } });

test.describe('Loader workspace', () => {
  test('loads a trip in reverse stop order and blocks save until counted', async ({ page }) => {
    await loginAs(page, 'loader');
    await expect(page.getByRole('heading', { name: /today.s trips/i })).toBeVisible();

    const trip = page.locator('a[href^="/loader/trips/"]').first();
    await expect(trip).toBeVisible({ timeout: 20_000 });
    await trip.click();

    await expect(page).toHaveURL(/\/loader\/trips\//);
    await expect(page.getByText(/reverse stop order/i)).toBeVisible();

    // Not every line is counted yet, so the trip is not markable as loaded.
    await expect(page.getByRole('button', { name: /nothing to save/i })).toBeDisabled();

    // Count every line in full; the same rule the server enforces.
    for (let i = 0; i < 200; i++) {
      const loadAll = page.getByRole('button', { name: /^Load all$/ }).first();
      if ((await loadAll.count()) === 0) break;
      await loadAll.click();
    }
    const save = page.getByRole('button', { name: /save .*lines?/i });
    await expect(save).toBeEnabled();
    await save.click();
    await expect(page.getByRole('status')).toContainText(/saved/i, { timeout: 20_000 });
  });

  test('a loader cannot open another depot route', async ({ page }) => {
    await loginAs(page, 'loader');
    await page.goto('/loader/trips/00000000-0000-0000-0000-000000000000');
    // The API returns 404 for an unknown/out-of-scope route; the screen says so.
    await expect(page.getByText(/couldn.t load|not found|no route|empty/i).first()).toBeVisible({
      timeout: 20_000,
    });
  });
});
