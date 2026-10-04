'use client';

import { useState, type FormEvent } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import type { Role } from '@waypoint/shared-types';
import { api, tokenStore } from '../../../lib/api';
import { ROLE_ROUTES, routeForRole } from '../../../lib/roles';
import { PasswordInput } from '../../../components/password-input';

/**
 * G-01 — Sign in.
 *
 * The Go API owns authentication. Choosing a role card only changes contextual
 * UI (which workspace the judge is heading to); it NEVER fills the email or
 * password fields and it is never an authentication mechanism. The user types
 * their own credentials and submits the form explicitly; the client then asks
 * GET /api/v1/me who the caller actually is and routes to that server-reported
 * role's workspace. The client never decides its own role.
 *
 * No account, name or email is shown or shipped in this bundle: the seeded demo
 * accounts are documented in the README for judges, and the user always types
 * their own credentials.
 */
export default function SignInPage() {
  const router = useRouter();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [selectedRole, setSelectedRole] = useState<Role | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  function selectRole(role: Role) {
    // Contextual UI only. Never touches the credential fields.
    setSelectedRole(role);
    setError(null);
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      const { token } = await api.login({ email, password });
      tokenStore.set(token);
      const me = await api.me();
      router.push(routeForRole(me.role).href);
    } catch {
      tokenStore.clear();
      setError('We could not sign you in. Check the email and password.');
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="mx-auto flex min-h-dvh w-full max-w-5xl flex-col justify-center px-4 py-12">
      <header className="mb-8">
        <p className="text-xs font-semibold uppercase tracking-widest text-brand">
          Waypoint Group
        </p>
        <h1 className="mt-2 text-3xl font-semibold tracking-tight text-ink">
          Delivery Planning System
        </h1>
        <p className="mt-2 max-w-xl text-ink-muted">
          One plan, four roles, every outlet served — or explained.
        </p>
      </header>

      <section aria-labelledby="choose-role">
        <h2
          id="choose-role"
          className="mb-1 text-sm font-medium text-ink-muted"
        >
          Choose a role to continue
        </h2>
        <p className="mb-3 text-xs text-ink-muted">
          This only highlights your workspace. Enter your own credentials below
          — nothing is filled in for you.
        </p>

        <ul className="grid grid-cols-2 gap-3">
          {ROLE_ROUTES.map((route) => {
            const selected = selectedRole === route.role;
            return (
              <li key={route.role}>
                <button
                  type="button"
                  onClick={() => selectRole(route.role)}
                  aria-pressed={selected}
                  className={`tap-target block w-full rounded-card bg-card p-4 text-left ring-1 transition focus-visible:ring-2 focus-visible:ring-brand ${
                    selected
                      ? 'ring-2 ring-brand'
                      : 'ring-ink/10 hover:ring-brand'
                  }`}
                >
                  <span className="font-semibold text-ink">{route.label}</span>
                  <p className="mt-1 hidden text-sm text-ink-muted sm:block">
                    {route.summary}
                  </p>
                </button>
              </li>
            );
          })}
        </ul>
      </section>

      <form onSubmit={handleSubmit} className="mt-8 max-w-md space-y-4">
        {error ? (
          <p
            role="alert"
            className="rounded-control bg-error/10 p-3 text-sm text-ink"
          >
            <strong className="font-semibold">Sign-in failed.</strong> {error}
          </p>
        ) : null}

        <div>
          <label
            htmlFor="email"
            className="mb-1 block text-sm font-medium text-ink"
          >
            Email
          </label>
          <input
            id="email"
            name="email"
            type="email"
            autoComplete="username"
            required
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            className="tap-target w-full rounded-control bg-card px-3 text-ink ring-1 ring-ink/15 focus-visible:ring-2 focus-visible:ring-brand"
          />
        </div>

        <div>
          <label
            htmlFor="password"
            className="mb-1 block text-sm font-medium text-ink"
          >
            Password
          </label>
          <PasswordInput
            id="password"
            name="password"
            autoComplete="current-password"
            required
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
          <div className="mt-1.5 text-right">
            <Link
              href="/forgot-password"
              className="text-sm font-medium text-link hover:underline"
            >
              Forgot password?
            </Link>
          </div>
        </div>

        <button
          type="submit"
          disabled={submitting}
          className="tap-target w-full rounded-control bg-action px-4 font-semibold text-white transition hover:opacity-90 disabled:opacity-60"
        >
          {submitting ? 'Signing in…' : 'Sign in'}
        </button>
      </form>
    </main>
  );
}
