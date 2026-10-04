import { test, expect, request as pwRequest } from '@playwright/test';
import globalSetup from './global-setup';
import { API, API_URL, DATE, PASSWORD, loginAs } from './helpers';

test.describe('Dispatcher workspace', () => {
  test.beforeEach(async ({ page }) => {
    await loginAs(page, 'dispatcher');
  });

  test('dashboard shows the operational KPIs and nav', async ({ page }) => {
    await expect(page.getByRole('navigation', { name: 'Dispatcher' })).toBeVisible();
    await expect(page.getByText(/dispatch control/i).first()).toBeVisible();
    // The queue, planning, deferrals, users and audit screens are all reachable.
    for (const label of ['Orders', 'Plan & allocate', 'Deferrals', 'Users', 'Audit trail']) {
      await expect(page.getByRole('link', { name: new RegExp(label, 'i') }).first()).toBeVisible();
    }
  });

  test('order queue lists confirmed orders and can be read', async ({ page }) => {
    await page.goto('/dispatcher/queue');
    await expect(page.getByRole('heading', { name: 'Orders' })).toBeVisible();
    // The seeded queue has orders; a queue row exposes the order number.
    await expect(page.getByText(/S1-/i).first()).toBeVisible({ timeout: 20_000 });
  });

  test('plan board exposes the per-rule panel after a run', async ({ page }) => {
    // Leave orders waiting so the board has something to plan, then restore the
    // confirmed plan afterwards for the Loader and Driver specs.
    const ctx = await pwRequest.newContext({ baseURL: API_URL });
    const login = await ctx.post(API('/auth/login'), {
      data: { email: 'priyantha.w@waypoint.lk', password: PASSWORD },
    });
    const { token } = await login.json();
    const auth = { Authorization: `Bearer ${token}` };
    await ctx.post(API('/demo/reset'), { headers: auth });
    const depots = await (await ctx.get(API('/depots'), { headers: auth })).json();
    const depotId = depots.depots.find((d: { code: string }) => d.code === 'PELIYAGODA').depotId;
    await ctx.post(API('/orders/close'), { headers: auth, data: { date: DATE, depotId } });
    await ctx.post(API('/allocations/suggest'), { headers: auth, data: { planningDate: DATE, depotId } });
    await ctx.dispose();

    await page.goto('/dispatcher/plan');
    await expect(page.getByRole('heading', { name: /plan & allocate/i })).toBeVisible();
    await page.getByRole('button', { name: /run planning/i }).click();
    await expect(page.getByText('Hard rules')).toBeVisible({ timeout: 30_000 });
    await expect(page.getByText(/E-0[1-7]/).first()).toBeVisible();

    await globalSetup();
  });

  test('users list shows the operational accounts without credentials', async ({ page }) => {
    await page.goto('/dispatcher/users');
    await expect(page.getByRole('heading', { name: /user/i }).first()).toBeVisible();
    await expect(page.getByText(/waypoint2026/)).toHaveCount(0);
  });

  test('deferral log is reachable and explains itself', async ({ page }) => {
    await page.goto('/dispatcher/deferrals');
    await expect(page.getByRole('heading', { name: /deferral/i }).first()).toBeVisible();
  });
});
