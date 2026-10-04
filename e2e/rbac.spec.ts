import { test, expect, request } from '@playwright/test';
import { API, API_URL, apiAs } from './helpers';

test.describe('Backend RBAC (the security boundary is the server)', () => {
  test('a store manager is forbidden from dispatcher-only endpoints', async () => {
    const { ctx, token } = await apiAs('store');
    const auth = { Authorization: `Bearer ${token}` };
    for (const path of ['/users', '/audit', '/orders/queue?date=2026-09-26', '/vehicles']) {
      const res = await ctx.get(API(path), { headers: auth });
      expect(res.status(), `${path} should be 403`).toBe(403);
    }
    await ctx.dispose();
  });

  test('a driver is forbidden from dispatcher-only endpoints', async () => {
    const { ctx, token } = await apiAs('driver');
    const res = await ctx.get(API('/audit'), { headers: { Authorization: `Bearer ${token}` } });
    expect(res.status()).toBe(403);
    await ctx.dispose();
  });

  test('unauthenticated requests are rejected with 401', async () => {
    const anon = await request.newContext({ baseURL: API_URL });
    expect((await anon.get(API('/me'))).status()).toBe(401);
    expect((await anon.get(API('/users'))).status()).toBe(401);
    await anon.dispose();
  });
});
