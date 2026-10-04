import { test, expect } from '@playwright/test';
import { loginAs } from './helpers';

test.describe('Store manager workspace', () => {
  test('places an order and sees the server-issued order number', async ({ page }) => {
    await loginAs(page, 'store');
    await expect(page.getByText(/store home/i).first()).toBeVisible();

    await page.goto('/store/order');
    await expect(page.getByRole('heading', { name: /order for/i })).toBeVisible();

    // An untouched order is not an error: the submit button is simply disabled.
    const submit = page.getByRole('button', { name: /submit order/i });
    await expect(submit).toBeDisabled();

    // Enter a quantity on the first line; the running meter enables submit.
    await page.getByRole('spinbutton').first().fill('3');
    await expect(submit).toBeEnabled();

    await submit.click();
    await expect(page.getByText(/order submitted/i)).toBeVisible();
    await expect(page.getByText(/order number is/i)).toBeVisible();
  });

  test('an order for the authenticated outlet only', async ({ page }) => {
    await loginAs(page, 'store');
    // The store sees only its own outlet on the order form.
    await page.goto('/store/order');
    await expect(page.getByText(/OUT014/).first()).toBeVisible();
  });

  test('my orders lists the outlet orders', async ({ page }) => {
    await loginAs(page, 'store');
    await page.goto('/store/orders');
    await expect(page.getByRole('heading', { name: /orders/i }).first()).toBeVisible();
  });
});
