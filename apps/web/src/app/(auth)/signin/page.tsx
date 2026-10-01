import Link from 'next/link';
import type { Metadata } from 'next';
import { Mono } from '@waypoint/ui';
import { ROLE_ROUTES } from '../../../lib/roles';

export const metadata: Metadata = { title: 'Sign in' };

/**
 * G-01 — Sign in.
 *
 * SCAFFOLD. The four role cards and the seeded-account hints are here because a
 * judge must be able to reach every workspace without hunting for credentials.
 * Replace the links with a real form posting to POST /auth/login; keep the cards.
 */
export default function SignInPage() {
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
              <Link
                href={route.href}
                className="tap-target block rounded-card bg-card p-4 ring-1 ring-ink/10 transition hover:ring-brand focus-visible:ring-2 focus-visible:ring-brand"
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
              </Link>
            </li>
          ))}
        </ul>
      </section>

      <p className="mt-8 rounded-control bg-warning/10 p-3 text-sm text-ink">
        <strong className="font-semibold">Scaffold:</strong> these cards link
        straight through without authenticating. Wire{' '}
        <Mono>POST /auth/login</Mono> and store the JWT before the demo.
      </p>
    </main>
  );
}
