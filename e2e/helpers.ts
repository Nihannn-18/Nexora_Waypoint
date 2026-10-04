import { expect, request, type APIRequestContext, type Page } from '@playwright/test';

export const API_URL = process.env.E2E_API_URL ?? 'http://localhost:8080';
export const API = (path: string) => `/api/v1${path}`;
export const DATE = '2026-09-26';
export const PASSWORD = 'waypoint2026';

export const EMAIL = {
  dispatcher: 'priyantha.w@waypoint.lk',
  loader: 'nadeesha.p@waypoint.lk',
  driver: 'kasun.p@waypoint.lk',
  store: 'ishara.s@waypoint.lk',
} as const;

export type Role = keyof typeof EMAIL;

const DEST: Record<Role, RegExp> = {
  dispatcher: /\/dispatcher(\/|$)/,
  loader: /\/loader(\/|$)/,
  driver: /\/driver(\/|$)/,
  store: /\/store(\/|$)/,
};

/** Sign in through the real sign-in screen and wait for the role workspace. */
export async function loginAs(page: Page, role: Role): Promise<void> {
  await page.goto('/signin');
  await page.getByLabel('Email').fill(EMAIL[role]);
  await page.locator('#password').fill(PASSWORD);
  await page.getByRole('button', { name: 'Sign in' }).click();
  await page.waitForURL(DEST[role], { timeout: 20_000 });
}

/** A raw API context authenticated as a seeded account, for test setup/assertions. */
export async function apiAs(role: Role): Promise<{ ctx: APIRequestContext; token: string }> {
  const ctx = await request.newContext({ baseURL: API_URL });
  const res = await ctx.post(API('/auth/login'), { data: { email: EMAIL[role], password: PASSWORD } });
  expect(res.ok(), `login ${role} failed: ${res.status()}`).toBeTruthy();
  const { token } = await res.json();
  return { ctx, token };
}

/** The first stop leg id on the driver's run, read through the real API. */
export async function driverFirstLeg(ctx: APIRequestContext, token: string): Promise<string> {
  const res = await ctx.get(API(`/driver/routes?date=${DATE}`), {
    headers: { Authorization: `Bearer ${token}` },
  });
  expect(res.ok(), `driver routes failed: ${res.status()}`).toBeTruthy();
  const routes = await res.json();
  const withStop = routes.find((r: { stops?: unknown[] }) => (r.stops?.length ?? 0) > 0);
  expect(withStop, 'the driver has no route with stops').toBeTruthy();
  return withStop.stops[0].legId as string;
}
