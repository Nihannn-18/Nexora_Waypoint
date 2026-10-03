import { betterAuth } from 'better-auth';
import { nextCookies } from 'better-auth/next-js';
import { bearer } from 'better-auth/plugins/bearer';
import { dash } from '@better-auth/infra';
import { Pool } from 'pg';

/**
 * Better Auth owns authentication, sessions and identity for Waypoint.
 *
 * The session, user, account and verification tables are created by the Go API's
 * Goose migration (`00006_better_auth.sql`) so a single migration path applies
 * the schema before either process serves traffic. Better Auth writes those
 * tables; the Go API only reads the session table to verify API requests.
 *
 * The Bearer plugin lets the Go API authenticate the same session with an
 * `Authorization: Bearer <session-token>` header (see the Driver PWA, which
 * cannot rely on cross-origin cookies). The browser still gets the normal
 * Better Auth session cookie for the web app itself.
 *
 * `DATABASE_URL`, `BETTER_AUTH_SECRET` and `BETTER_AUTH_URL` are required at
 * runtime. `BETTER_AUTH_SECRET` is never given a default: a placeholder secret
 * in production is a security bug, so the process should fail loudly instead.
 *
 * `dash()` connects this auth server to the hosted Better Auth dashboard using
 * `BETTER_AUTH_API_KEY` (it defaults `apiUrl`/`kvUrl` to Better Auth's hosted
 * endpoints). It is an observability/connection plugin and does not change the
 * sign-in or session behaviour the Go API verifies.
 */
const databaseUrl = process.env.DATABASE_URL;

/**
 * A dedicated pool for Better Auth. It is created lazily so importing this
 * module during `next build` (where DATABASE_URL may be absent) does not throw;
 * Better Auth only connects when an auth endpoint is actually called.
 */
function createPool(): Pool {
  return new Pool({ connectionString: databaseUrl });
}

function trustedOrigins(): string[] {
  const raw = process.env.BETTER_AUTH_TRUSTED_ORIGINS ?? '';
  return raw
    .split(',')
    .map((origin) => origin.trim())
    .filter((origin) => origin.length > 0);
}

export const auth = betterAuth({
  database: createPool(),
  secret: process.env.BETTER_AUTH_SECRET,
  baseURL: process.env.BETTER_AUTH_URL,
  trustedOrigins: trustedOrigins(),
  emailAndPassword: {
    enabled: true,
    // The Hackathon runs offline-friendly and without a mail provider; email
    // verification would lock every judge out of the seeded accounts.
    requireEmailVerification: false,
  },
  session: {
    // A full shift: a driver is not signed out mid-route.
    expiresIn: 60 * 60 * 12,
    updateAge: 60 * 60,
  },
  plugins: [bearer(), dash(), nextCookies()],
});
