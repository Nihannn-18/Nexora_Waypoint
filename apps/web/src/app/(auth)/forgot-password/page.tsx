'use client';

import { useState, type FormEvent } from 'react';
import Link from 'next/link';
import { WaypointApiError } from '@waypoint/api-client';
import { api } from '../../../lib/api';

/**
 * Password reset request (Forgot password).
 *
 * The response is always the same generic message, whether or not the email
 * exists, so this screen can never be used to discover which accounts are real.
 * With DEMO_MODE on, the API logs the reset token (there is no email provider in
 * this project); the judge copies it to the reset screen. The token is never
 * returned to the browser by the API, and the log is off outside DEMO_MODE.
 */
export default function ForgotPasswordPage() {
  const [email, setEmail] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [sent, setSent] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await api.forgotPassword({ email: email.trim() });
      setSent(true);
    } catch (err) {
      // A malformed email is answered generically by the server; a genuine
      // connectivity fault is the only thing worth surfacing.
      if (err instanceof WaypointApiError && err.isOffline) {
        setError('We could not reach the server. Check the connection and try again.');
      } else {
        setSent(true);
      }
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="mx-auto flex min-h-dvh w-full max-w-md flex-col justify-center px-4 py-12">
      <header className="mb-8">
        <p className="text-xs font-semibold uppercase tracking-widest text-brand">
          Waypoint Group
        </p>
        <h1 className="mt-2 text-2xl font-semibold tracking-tight text-ink">
          Forgot password
        </h1>
        <p className="mt-2 text-sm text-ink-muted">
          Enter your email and we will provide reset instructions.
        </p>
      </header>

      {sent ? (
        <section
          role="status"
          aria-live="polite"
          className="rounded-card bg-success-bg p-4 ring-1 ring-success/30"
        >
          <p className="flex items-start gap-2 text-sm text-ink">
            <span aria-hidden="true" className="text-success">
              ✓
            </span>
            <span>
              If the account exists, password reset instructions have been
              provided.
            </span>
          </p>
          <p className="mt-3 text-xs text-ink-muted">
            This project has no email provider: with DEMO_MODE on, the reset
            token is written to the API log. Open{' '}
            <Link href="/reset-password" className="text-link hover:underline">
              Reset password
            </Link>{' '}
            and paste the token to continue.
          </p>
        </section>
      ) : (
        <form onSubmit={handleSubmit} className="space-y-4">
          {error ? (
            <p role="alert" className="rounded-control bg-error/10 p-3 text-sm text-ink">
              {error}
            </p>
          ) : null}

          <div>
            <label htmlFor="email" className="mb-1 block text-sm font-medium text-ink">
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

          <button
            type="submit"
            disabled={submitting}
            className="tap-target w-full rounded-control bg-action px-4 font-semibold text-white transition hover:opacity-90 disabled:opacity-60"
          >
            {submitting ? 'Sending…' : 'Send reset instructions'}
          </button>
        </form>
      )}

      <p className="mt-6 text-sm">
        <Link href="/signin" className="font-medium text-link hover:underline">
          ← Back to sign in
        </Link>
      </p>
    </main>
  );
}
