'use client';

import Link from 'next/link';
import type { LoaderRouteSummary } from '@waypoint/api-client';
import { Mono } from '@waypoint/ui';
import {
  Card,
  Chip,
  EmptyState,
  ErrorState,
  Eyebrow,
  LoadingState,
} from './_components/ui';
import { useLoaderRoutes } from './_lib/use-loading';

/**
 * L-01 · Today's trips.
 *
 * The dock's confirmed routes for the business day, ordered by trip so the
 * earliest departure is first. Every figure here is a real count from the
 * server — stops, lines counted, shortfall units. There are no invented
 * metrics: if the API cannot say it, the screen does not claim it.
 */
export default function LoaderTripsPage() {
  const state = useLoaderRoutes();

  return (
    <div className="flex flex-col gap-3">
      <header className="flex flex-col gap-1 px-1 pt-2">
        <Eyebrow>Loading manifest</Eyebrow>
        <h1 className="text-xl font-semibold text-ink">Today&rsquo;s trips</h1>
        {state.date && (
          <p className="text-sm text-ink-muted">
            Delivery day <Mono>{formatDate(state.date)}</Mono>
          </p>
        )}
      </header>

      {state.status === 'loading' && (
        <LoadingState label="Loading today's trips…" />
      )}

      {state.status === 'error' && (
        <ErrorState
          title="Couldn't load your trips"
          message={state.message}
          onRetry={state.reload}
        />
      )}

      {state.status === 'ready' && state.data.length === 0 && (
        <EmptyState title="Nothing to load yet">
          No routes have been confirmed for your depot on this day. The
          dispatcher publishes the plan after the queue closes — this list fills
          in as soon as that happens.
        </EmptyState>
      )}

      {state.status === 'ready' && state.data.length > 0 && (
        <>
          <Summary routes={state.data} />
          <ul className="flex flex-col gap-3">
            {state.data.map((route) => (
              <li key={route.routeId}>
                <RouteCard route={route} />
              </li>
            ))}
          </ul>
        </>
      )}
    </div>
  );
}

/** A plain count of the work, so the first glance answers "how much is left". */
function Summary({ routes }: { routes: readonly LoaderRouteSummary[] }) {
  const ready = routes.filter((r) => r.routeReady).length;
  const shortfalls = routes.filter((r) => r.shortfallQty > 0).length;

  return (
    <Card className="flex-row gap-0 divide-x divide-line">
      <Figure label="Trips" value={`${routes.length}`} />
      <Figure label="Ready" value={`${ready}`} />
      <Figure label="With shortfall" value={`${shortfalls}`} />
    </Card>
  );
}

function Figure({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-1 flex-col gap-1 px-3 first:pl-0 last:pr-0">
      <Eyebrow>{label}</Eyebrow>
      <p className="font-mono text-2xl font-semibold tabular-nums text-ink">
        {value}
      </p>
    </div>
  );
}

function RouteCard({ route }: { route: LoaderRouteSummary }) {
  const status = loadingStatus(route);

  return (
    <Link
      href={`/loader/trips/${route.routeId}`}
      className="block rounded-card focus:outline-2 focus:outline-offset-2 focus:outline-action"
    >
      <Card as="article" className="gap-3">
        <div className="flex items-start justify-between gap-2">
          <div className="flex flex-col gap-1">
            <Eyebrow>
              Trip {route.tripNo} · {route.brand}
            </Eyebrow>
            <p className="text-base font-semibold text-ink">
              <Mono>{route.vehicleId}</Mono>
            </p>
            <p className="text-sm text-ink-muted">
              {route.district} · {route.stops}{' '}
              {route.stops === 1 ? 'stop' : 'stops'} · {route.lines}{' '}
              {route.lines === 1 ? 'line' : 'lines'}
            </p>
          </div>
          <Chip tone={status.tone} glyph={status.glyph}>
            {status.label}
          </Chip>
        </div>

        <div className="flex flex-col gap-1.5">
          <div
            className="h-1.5 w-full overflow-hidden rounded-pill bg-muted"
            role="img"
            aria-label={`${route.linesComplete} of ${route.lines} lines counted`}
          >
            <div
              className="h-full rounded-pill bg-action"
              style={{
                width: `${
                  route.lines > 0
                    ? Math.round((route.linesComplete / route.lines) * 100)
                    : 0
                }%`,
              }}
            />
          </div>
          <p className="text-xs text-ink-muted">
            <Mono>
              {route.linesComplete}/{route.lines}
            </Mono>{' '}
            lines counted
            {route.shortfallQty > 0 && (
              <>
                {' · '}
                <span className="font-semibold text-error-strong">
                  {route.shortfallQty} short
                </span>
              </>
            )}
          </p>
        </div>
      </Card>
    </Link>
  );
}

/**
 * The chip a loader reads first. States are derived from the server's counts,
 * never from a client-only idea of "done": "Ready" appears only when the server
 * says every line reconciles.
 */
function loadingStatus(route: LoaderRouteSummary): {
  label: string;
  tone: 'neutral' | 'info' | 'success' | 'error';
  glyph: string;
} {
  if (route.shortfallQty > 0) {
    return { label: 'Flagged', tone: 'error', glyph: '⚠' };
  }
  if (route.routeReady) {
    return { label: 'Ready', tone: 'success', glyph: '✓' };
  }
  if (route.linesComplete > 0) {
    return { label: 'Loading', tone: 'info', glyph: '◐' };
  }
  return { label: 'Not started', tone: 'neutral', glyph: '○' };
}

const DAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'] as const;
const MONTHS = [
  'Jan',
  'Feb',
  'Mar',
  'Apr',
  'May',
  'Jun',
  'Jul',
  'Aug',
  'Sep',
  'Oct',
  'Nov',
  'Dec',
] as const;

/**
 * "Sat 26 Sep", the date format the design uses. The names are spelled out
 * rather than taken from toLocaleDateString, which renders September as "Sept"
 * and would drift from the rest of the product.
 */
function formatDate(iso: string): string {
  const d = new Date(`${iso}T00:00:00`);
  return `${DAYS[d.getDay()] ?? ''} ${d.getDate()} ${MONTHS[d.getMonth()] ?? ''}`;
}
