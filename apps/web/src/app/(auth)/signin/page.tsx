'use client';

import { useState, type FormEvent } from 'react';
import { useRouter } from 'next/navigation';
import { Mono } from '@waypoint/ui';
import { authClient } from '../../../lib/auth-client';
import { api } from '../../../lib/api';
import { ROLE_ROUTES, routeForRole } from '../../../lib/roles';

/**
 * G-01 — Sign in.
 *
 * Real Better Auth email/password sign-in. Picking a role card pre-fills the
 * demo account (a judge never has to hunt for credentials); the submit calls
 * Better Auth, stores the bearer session token through the existing API-client
 * token store, then asks GET /api/v1/me who the caller actually is and routes
 * to that role's workspace. The client never decides its own role: it is read
 * from the server-verified identity.
 */
const DEMO_PASSWORD = 'waypoint2026';

export default function SignInPage() {
  const router = useRouter();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  function selectRole(roleEmail: string) {
    setEmail(roleEmail);
    setPassword(DEMO_PASSWORD);
    setError(null);
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      const { error: signInError } = await authClient.signIn.email({
        email,
        password,
      });
      if (signInError) {
        setError('We could not sign you in. Check the email and password.');
        return;
      }
      const me = await api.me();
      router.push(routeForRole(me.role).href);
    } catch {
      setError('Sign-in failed. Please try again.');
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
          className="mb-3 text-sm font-medium text-ink-muted"
        >
          Choose a role to continue
        </h2>

        <ul className="grid gap-3 sm:grid-cols-2">
          {ROLE_ROUTES.map((route) => (
            <li key={route.role}>
              <button
                type="button"
                onClick={() => selectRole(route.email)}
                className="tap-target block w-full rounded-card bg-card p-4 text-left ring-1 ring-ink/10 transition hover:ring-brand focus-visible:ring-2 focus-visible:ring-brand"
              >
                <div className="flex items-baseline justify-between gap-2">
                  <span className="font-semibold text-ink">{route.label}</span>
                  <span className="text-xs text-ink-muted">
                    {route.persona}
                  </span>
                </div>
                <p className="mt-1 text-sm text-ink-muted">{route.summary}</p>
                <Mono className="mt-2 block text-xs text-ink-muted">
                  {route.email}
                </Mono>
              </button>
            </li>
          ))}
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
          <input
            id="password"
            name="password"
            type="password"
            autoComplete="current-password"
            required
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            className="tap-target w-full rounded-control bg-card px-3 text-ink ring-1 ring-ink/15 focus-visible:ring-2 focus-visible:ring-brand"
          />
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
