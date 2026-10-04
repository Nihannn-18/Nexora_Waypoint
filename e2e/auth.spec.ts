import { test, expect } from '@playwright/test';
import { EMAIL, PASSWORD } from './helpers';

const DEST = {
  dispatcher: '/dispatcher',
  loader: '/loader',
  driver: '/driver',
  store: '/store',
} as const;

test.describe('G-01 sign in', () => {
  for (const [role, email] of Object.entries(EMAIL)) {
    test(`${role} signs in and lands in its workspace`, async ({ page }) => {
      await page.goto('/signin');
      await page.getByLabel('Email').fill(email);
      await page.locator('#password').fill(PASSWORD);
      await page.getByRole('button', { name: 'Sign in' }).click();
      await page.waitForURL(new RegExp(DEST[role as keyof typeof DEST] + '(\\/|$)'), {
        timeout: 20_000,
      });
      expect(page.url()).toContain(DEST[role as keyof typeof DEST]);
    });
  }

  test('choosing a role card never pre-fills the credentials', async ({ page }) => {
    await page.goto('/signin');
    await page.getByRole('button', { name: /Dispatcher/ }).click();
    await expect(page.getByLabel('Email')).toHaveValue('');
    await expect(page.locator('#password')).toHaveValue('');
  });

  test('wrong credentials show an accessible error and stay on sign-in', async ({ page }) => {
    await page.goto('/signin');
    await page.getByLabel('Email').fill(EMAIL.dispatcher);
    await page.locator('#password').fill('definitely-wrong');
    await page.getByRole('button', { name: 'Sign in' }).click();
    await expect(page.getByText(/could not sign you in/i)).toBeVisible();
    expect(page.url()).toContain('/signin');
  });

  test('an unauthenticated protected workspace redirects to sign-in', async ({ page }) => {
    await page.goto('/dispatcher');
    await page.waitForURL(/\/signin/, { timeout: 20_000 });
    expect(page.url()).toContain('/signin');
  });
});
