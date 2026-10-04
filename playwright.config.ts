import { defineConfig, devices } from '@playwright/test';

/**
 * End-to-end tests for the Waypoint judge walkthrough.
 *
 * They run against a REAL running stack (Next.js on :3000, Go API on :8080,
 * PostgreSQL and RabbitMQ via Compose) — never against mocks. Start the stack
 * with `docker compose up --build` before running `npm run e2e`.
 *
 * A global setup re-seeds the demo day and confirms one plan, so the Loader and
 * Driver workspaces have real routes to work with; the tests then drive the
 * browser exactly as a judge would.
 */
export default defineConfig({
  testDir: './e2e',
  globalSetup: './e2e/global-setup.ts',
  timeout: 45_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  reporter: [['list'], ['html', { open: 'never' }]],
  use: {
    baseURL: process.env.E2E_BASE_URL ?? 'http://localhost:3000',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'off',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 900 } },
    },
  ],
});
