import { Pool } from 'pg';
import { auth } from './auth';

/**
 * Idempotent Better Auth demo-account seed.
 *
 * Better Auth owns credentials, so the four Hackathon accounts are created
 * through Better Auth's own sign-up API (never by writing password hashes by
 * hand). The matching authorization rows (`app_user`) are owned by the Go seed,
 * which runs when the API applies migrations; this seed therefore only creates
 * the account side and maps to `app_user` by email.
 *
 * Startup ordering: the Go API applies `00006_better_auth.sql` before it starts
 * listening, and the web container waits for the API to be healthy, so the
 * Better Auth tables exist by the time this runs. `waitForSchema` is a belt-and-
 * braces guard for local runs that start the web app first.
 *
 * Running it twice changes nothing: an existing account is left untouched.
 */

interface DemoAccount {
  readonly email: string;
  readonly name: string;
}

const DEMO_ACCOUNTS: readonly DemoAccount[] = [
  { email: 'priyantha.w@waypoint.lk', name: 'Priyantha W.' },
  { email: 'nadeesha.p@waypoint.lk', name: 'Nadeesha P.' },
  { email: 'kasun.p@waypoint.lk', name: 'Kasun P.' },
  { email: 'ishara.s@waypoint.lk', name: 'Ishara S.' },
];

let pool: Pool | null = null;

function db(): Pool {
  if (!pool) {
    pool = new Pool({ connectionString: process.env.DATABASE_URL });
  }
  return pool;
}

/** Waits until the Better Auth `user` table exists, or gives up. */
async function waitForSchema(timeoutMs: number): Promise<boolean> {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    try {
      const { rows } = await db().query(
        `SELECT to_regclass('public."user"') AS table_name`,
      );
      if (rows[0]?.table_name) return true;
    } catch {
      // Database not reachable yet; retry until the deadline.
    }
    if (Date.now() > deadline) return false;
    await new Promise((resolve) => setTimeout(resolve, 1000));
  }
}

export async function ensureDemoAccounts(): Promise<void> {
  if (!process.env.DATABASE_URL) {
    // Nothing to seed against; the API's own readiness will surface a real
    // misconfiguration to the operator.
    return;
  }

  if (!(await waitForSchema(30_000))) {
    console.error(
      '[auth-seed] Better Auth tables did not appear within 30s; skipping demo-account seeding.',
    );
    return;
  }

  const password = process.env.DEMO_SEED_PASSWORD ?? 'waypoint2026';

  for (const account of DEMO_ACCOUNTS) {
    const existing = await db().query(
      `SELECT "id" FROM "user" WHERE "email" = $1`,
      [account.email],
    );
    if ((existing.rowCount ?? 0) > 0) {
      continue;
    }
    try {
      await auth.api.signUpEmail({
        body: { email: account.email, password, name: account.name },
      });
    } catch (error) {
      // A concurrent start may have created the account between the check and
      // the insert; anything else is worth surfacing.
      const message = error instanceof Error ? error.message : String(error);
      if (!/exist/i.test(message)) {
        console.error(
          `[auth-seed] could not create ${account.email}: ${message}`,
        );
      }
    }
  }

  // Email verification is disabled for the Hackathon, but sign-ups can leave
  // the flag false depending on configuration; make it explicit for the demo.
  await db().query(
    `UPDATE "user" SET "emailVerified" = true WHERE "email" = ANY($1::text[])`,
    [DEMO_ACCOUNTS.map((a) => a.email)],
  );

  // The Go seed owns app_user. Warn if the mapping is missing so an operator
  // sees the cause before a 403 at sign-in.
  const mapped = await db().query(
    `SELECT count(*)::int AS n FROM app_user WHERE "email" = ANY($1::text[]) AND is_active`,
    [DEMO_ACCOUNTS.map((a) => a.email)],
  );
  if ((mapped.rows[0]?.n ?? 0) < DEMO_ACCOUNTS.length) {
    console.warn(
      '[auth-seed] some demo accounts have no active app_user row; start the API so its seed runs.',
    );
  }
}
