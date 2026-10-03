/**
 * Next.js instrumentation hook: runs once when the server process starts.
 *
 * Better Auth owns authentication, so the four Hackathon demo accounts are
 * provisioned through Better Auth's own API. The Go API applies the Better Auth
 * schema migration before it starts listening and the web container waits for
 * the API to be healthy, so the tables exist first. `ensureDemoAccounts` also
 * waits for the schema, so a local `nx dev web` started before the API does not
 * race the migration.
 *
 * Only the Node.js runtime has `pg`; the Edge runtime is skipped.
 */
export async function register(): Promise<void> {
  if (process.env.NEXT_RUNTIME !== 'nodejs') {
    return;
  }
  const { ensureDemoAccounts } = await import('./lib/seed-auth');
  try {
    await ensureDemoAccounts();
  } catch (error) {
    console.error('[auth-seed] demo-account seeding failed', error);
  }
}
