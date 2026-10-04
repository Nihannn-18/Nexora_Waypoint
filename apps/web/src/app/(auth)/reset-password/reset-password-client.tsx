'use client';

import { useEffect, useState, type FormEvent } from 'react';
import Link from 'next/link';
import { useRouter, useSearchParams } from 'next/navigation';
import { WaypointApiError } from '@waypoint/api-client';
import { api } from '../../../lib/api';

/**
 * Password reset (consume token).
 *
 * The token arrives from the reset link (here, the DEMO_MODE API log) as a query
 * parameter. Every failure case — unknown, expired, already-used or an inactive
 * account — is surfaced as the same generic message, so the screen cannot be
 * used to probe token or account state.
 */
export default function ResetPasswordClient() {
  const router = useRouter();
  const search = useSearchParams();
  const [token, setToken] = useState('');
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [done, setDone] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const fromLink = search.get('token');
    if (fromLink) setToken(fromLink);
  }, [search]);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    if (password.length < 8) {
      setError('Password must be at least 8 characters.');
      return;
    }
    if (password !== confirm) {
      setError('The passwords do not match.');
      return;
    }
    setSubmitting(true);
    try {
      await api.resetPassword({
        token: token.trim(),
        newPassword: password,
        confirmPassword: confirm,
      });
      setDone(true);
      // The old session (if any) has been revoked server-side; send the user
      // back to sign in with the new password.
      window.setTimeout(() => router.push('/signin'), 1500);
    } catch (err) {
      if (err instanceof WaypointApiError && err.isOffline) {
        setError('We could not reach the server. Check the connection and try again.');
      } else {
        setError(
          'This reset link is invalid or has expired. Request a new one and try again.',
        );
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
          Reset password
        </h1>
        <p className="mt-2 text-sm text-ink-muted">
          Choose a new password for your account.
        </p>
      </header>

      {done ? (
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
              Your password has been reset. Taking you to sign in…
            </span>
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
            <label htmlFor="token" className="mb-1 block text-sm font-medium text-ink">
              Reset token
            </label>
            <input
              id="token"
              name="token"
              type="text"
              autoComplete="one-time-code"
              required
              value={token}
              onChange={(event) => setToken(event.target.value)}
              className="tap-target w-full rounded-control bg-card px-3 font-mono text-sm text-ink ring-1 ring-ink/15 focus-visible:ring-2 focus-visible:ring-brand"
            />
            <p className="mt-1 text-xs text-ink-muted">
              With DEMO_MODE on, this is the token written to the API log after a
              reset request.
            </p>
          </div>

          <div>
            <label htmlFor="password" className="mb-1 block text-sm font-medium text-ink">
              New password
            </label>
            <input
              id="password"
              name="password"
              type="password"
              autoComplete="new-password"
              required
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              className="tap-target w-full rounded-control bg-card px-3 text-ink ring-1 ring-ink/15 focus-visible:ring-2 focus-visible:ring-brand"
            />
          </div>

          <div>
            <label htmlFor="confirm" className="mb-1 block text-sm font-medium text-ink">
              Confirm password
            </label>
            <input
              id="confirm"
              name="confirm"
              type="password"
              autoComplete="new-password"
              required
              value={confirm}
              onChange={(event) => setConfirm(event.target.value)}
              className="tap-target w-full rounded-control bg-card px-3 text-ink ring-1 ring-ink/15 focus-visible:ring-2 focus-visible:ring-brand"
            />
          </div>

          <button
            type="submit"
            disabled={submitting}
            className="tap-target w-full rounded-control bg-action px-4 font-semibold text-white transition hover:opacity-90 disabled:opacity-60"
          >
            {submitting ? 'Resetting…' : 'Reset password'}
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
