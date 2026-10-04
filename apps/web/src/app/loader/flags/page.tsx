'use client';

import Link from 'next/link';
import { Mono } from '@waypoint/ui';
import {
  Card,
  Chip,
  EmptyState,
  ErrorState,
  Eyebrow,
  LoadingState,
} from '../_components/ui';
import { useLoaderRoutes } from '../_lib/use-loading';

/**
 * L-04 · What has been flagged today.
 *
 * A read-only view of the routes carrying a shortfall, so a loader can see what
 * they reported without reopening every trip. The counts come from the same
 * server summary as the trips list — there is no separate shortfall store, and
 * this screen invents nothing.
 */
export default function LoaderFlagsPage() {
  const state = useLoaderRoutes();
  const flagged =
    state.status === 'ready'
      ? state.data.filter((r) => r.shortfallQty > 0)
      : [];

  return (
    <div className="flex flex-col gap-3">
      <header className="flex flex-col gap-1 px-1 pt-2">
        <Eyebrow>Shortfalls</Eyebrow>
        <h1 className="text-xl font-semibold text-ink">Flagged today</h1>
      </header>

      {state.status === 'loading' && <LoadingState label="Loading flags…" />}

      {state.status === 'error' && (
        <ErrorState
          title="Couldn't load flags"
          message={state.message}
          onRetry={state.reload}
        />
      )}

      {state.status === 'ready' && flagged.length === 0 && (
        <EmptyState title="Nothing flagged">
          Every line counted so far matched its order. Anything you mark damaged
          or missing on a trip appears here, and the dispatcher is told at the
          same time.
        </EmptyState>
      )}

      {flagged.length > 0 && (
        <ul className="flex flex-col gap-3">
          {flagged.map((route) => (
            <li key={route.routeId}>
              <Link
                href={`/loader/trips/${route.routeId}`}
                className="block rounded-card focus:outline-2 focus:outline-offset-2 focus:outline-action"
              >
                <Card as="article">
                  <div className="flex items-start justify-between gap-2">
                    <div className="flex flex-col gap-0.5">
                      <Eyebrow>
                        Trip {route.tripNo} · {route.brand}
                      </Eyebrow>
                      <p className="text-base font-semibold text-ink">
                        <Mono>{route.vehicleId}</Mono> · {route.district}
                      </p>
                    </div>
                    <Chip tone="error" glyph="⚠">
                      {route.shortfallQty} short
                    </Chip>
                  </div>
                  <p className="text-sm text-ink-muted">
                    The dispatcher has been notified. Open the trip to review or
                    correct the counts.
                  </p>
                </Card>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
