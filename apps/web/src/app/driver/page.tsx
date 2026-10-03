'use client';

import Link from 'next/link';
import { useEffect } from 'react';
import type { DriverRoute, DriverRouteStop } from '@waypoint/api-client';
import type { DockType, TempRequirement } from '@waypoint/shared-types';
import { useLeg, useOutbox, useRoutes, prefetchLegs } from './_lib/use-outbox';
import type { OutboxEvent } from './_lib/outbox';
import {
  Alert,
  Body,
  Card,
  Eyebrow,
  Icon,
  OUTCOME_LABEL,
  buttonClass,
  time,
} from './_components/ui';

const DOCK_LABEL: Record<DockType, string> = {
  REAR_DOCK: 'Rear loading bay',
  STREET: 'Street unloading',
  MALL_BAY: 'Mall bay',
};
const TEMP_LABEL: Record<TempRequirement, string> = {
  CHILLED: 'Chilled cargo',
  FROZEN: 'Chilled cargo',
  AMBIENT: 'Dry cargo',
};

/** A stop is done once the server has an outcome or this phone recorded one. */
const isDone = (s: DriverRouteStop, local: ReadonlyMap<string, OutboxEvent>) =>
  local.has(s.legId) || !['PENDING', 'IN_TRANSIT'].includes(s.status);

/**
 * R-01 Cockpit (Figma 1:4306 online / 37:5584 offline): active run, current
 * stop with window and unloading bay, planned route with completed stops.
 *
 * GET /driver/routes is depot-scoped — the schema links no driver to a
 * vehicle — so the active run is the first route that still has stops to do.
 */
export default function DriverCockpitPage() {
  const { online, events } = useOutbox();
  const state = useRoutes();
  const local = new Map(events.map((e) => [e.legId, e]));
  const latest = events.at(-1);

  const routes = state.status === 'ready' ? state.routes : [];
  const active = routes.find((r) => r.stops.some((s) => !isDone(s, local)));

  // Save the whole run on the phone while there is signal — once per route,
  // so `active` is deliberately keyed by its id.
  const activeId = active?.routeId;
  useEffect(() => {
    if (active && online) prefetchLegs(active);
  }, [activeId, online]);

  return (
    <Body>
      {!online && (
        <Alert tone="info" eyebrow="Everything still works" title="Keep delivering — nothing is lost">
          Outcomes, signatures and photos are kept on this phone and upload in
          order when signal returns.
        </Alert>
      )}

      {latest && <LastAction event={latest} />}

      {state.status === 'loading' ? (
        <p className="py-10 text-center text-sm text-ink-muted" role="status">
          Loading today’s route…
        </p>
      ) : state.status === 'error' ? (
        <Card>
          <p role="alert" className="text-sm text-ink">
            {state.message}
          </p>
        </Card>
      ) : !active ? (
        <Card>
          <h2 className="text-base font-bold text-ink">
            {routes.length ? 'All stops done' : 'No route yet'}
          </h2>
          <p className="text-sm text-ink-muted">
            {routes.length
              ? 'Every stop on today’s runs has an outcome. Check the log to see what has uploaded.'
              : `Nothing is dispatched from your depot for ${state.date}. The run appears here once dispatch publishes the plan.`}
          </p>
        </Card>
      ) : (
        <ActiveRun
          route={active}
          trips={routes.filter((r) => r.vehicleId === active.vehicleId).length}
          local={local}
          cached={state.cached}
        />
      )}
    </Body>
  );
}

function ActiveRun({
  route,
  trips,
  local,
  cached,
}: {
  route: DriverRoute;
  trips: number;
  local: ReadonlyMap<string, OutboxEvent>;
  cached: boolean;
}) {
  const index = route.stops.findIndex((s) => !isDone(s, local));
  const current = route.stops[index];
  const left = route.stops.length - index;
  if (!current) return null;

  return (
    <>
      <Card>
        <div className="flex items-center justify-between gap-2 border-b border-line pb-3">
          <div className="flex flex-col leading-[1.3]">
            <Eyebrow>Active run</Eyebrow>
            <p className="text-base font-semibold text-ink">
              {route.brand.charAt(0) + route.brand.slice(1).toLowerCase()} ·{' '}
              {route.district}
            </p>
            <p className="text-xs text-ink-muted">
              Trip {route.tripNo} of {trips} ·{' '}
              <span className="font-mono">{route.vehicleId}</span>
            </p>
          </div>
          <span className="shrink-0 rounded-chip border border-brand/30 bg-info-bg px-2 py-0.5 text-xs font-medium text-link">
            Stop {index + 1} of {route.stops.length}
          </span>
        </div>
        <CurrentStop stop={current} />
        {cached && (
          <p className="text-xs text-ink-muted">
            Showing the run sheet saved on this phone.
          </p>
        )}
      </Card>

      <Card>
        <div className="flex items-center justify-between border-b border-line pb-2">
          <h2 className="text-base font-bold text-ink">Your Planned Route</h2>
          <p className="text-[11px] font-semibold text-ink-muted">
            {left} {left === 1 ? 'stop' : 'stops'} to drop off
          </p>
        </div>
        <ol className="relative flex flex-col gap-5 pl-6">
          <span aria-hidden="true" className="absolute top-3 bottom-3 left-3 w-0.5 bg-line-strong" />
          {route.stops.map((s, i) => (
            <StopRow
              key={s.legId}
              stop={s}
              n={i + 1}
              state={i < index || isDone(s, local) ? 'done' : i === index ? 'current' : 'next'}
              event={local.get(s.legId)}
            />
          ))}
        </ol>
      </Card>

      <div className="flex flex-col gap-2.5">
        <Link
          href={`/driver/stops/${current.legId}/outcome?o=DELIVERED`}
          className={buttonClass('success')}
        >
          <Icon name="thumbs-up" />
          Confirm delivery
        </Link>
        <Link
          href={`/driver/stops/${current.legId}/outcome?o=FAILED`}
          className={buttonClass('danger')}
        >
          <Icon name="ban" />
          Report failure / delay
        </Link>
      </div>
    </>
  );
}

/** Figma "Target waypoint": window and unloading bay come from GET /legs/{id}. */
function CurrentStop({ stop }: { stop: DriverRouteStop }) {
  const leg = useLeg(stop.legId);
  const detail = leg.status === 'ready' ? leg.leg : undefined;
  const temps = [...new Set(detail?.orders.map((o) => TEMP_LABEL[o.tempRequirement]))];
  const units = detail?.orders.reduce((n, o) => n + o.totalUnits, 0);
  const kg = detail?.orders.reduce((n, o) => n + o.totalWeightKg, 0);

  return (
    <Link
      href={`/driver/stops/${stop.legId}`}
      className="flex flex-col gap-1.5 rounded-chip border border-line bg-page p-3"
    >
      <div className="flex items-center justify-between">
        <Eyebrow className="font-bold text-accent">Target waypoint</Eyebrow>
        {stop.plannedArrival && (
          <span className="rounded-chip border border-success/40 bg-success-bg px-2 py-0.5 text-[11px] font-medium text-success">
            Planned <span className="font-mono">{time(stop.plannedArrival)}</span>
          </span>
        )}
      </div>
      <p className="text-lg font-bold leading-[1.3] text-ink">
        {stop.outletName}{' '}
        <span className="font-mono text-sm font-normal text-ink-muted">
          ({stop.outletId})
        </span>
      </p>
      <div className="flex items-center justify-between gap-2 border-t border-line pt-1.5 text-xs">
        <p className="text-ink-muted">
          Window{' '}
          <span className="font-mono">
            {stop.windowOpen}–{stop.windowClose}
          </span>
        </p>
        {detail && (
          <span className="rounded-chip border border-line-strong bg-white px-2 py-0.5 text-right font-semibold text-ink">
            {DOCK_LABEL[detail.outlet.dockType]}
            {temps.length > 0 && ` · ${temps.join(' + ')}`}
          </span>
        )}
      </div>
      {detail && (
        <p className="text-xs text-ink-muted">
          {units} units · {kg?.toLocaleString('en-GB', { maximumFractionDigits: 1 })} kg ·{' '}
          {detail.orders.length} {detail.orders.length === 1 ? 'order' : 'orders'}
        </p>
      )}
    </Link>
  );
}

function StopRow({
  stop,
  n,
  state,
  event,
}: {
  stop: DriverRouteStop;
  n: number;
  state: 'done' | 'current' | 'next';
  event?: OutboxEvent;
}) {
  const box = {
    done: 'border border-success/40 bg-success-bg',
    current: 'border-2 border-accent bg-white',
    next: 'border border-line-strong bg-muted',
  }[state];
  const dot = {
    done: 'bg-success',
    current: 'bg-accent ring-4 ring-info-bg',
    next: 'bg-ink-faint',
  }[state];
  const status = event
    ? `${OUTCOME_LABEL[event.outcome]} ${time(event.occurredAt)}`
    : state === 'done'
      ? stop.status.charAt(0) + stop.status.slice(1).toLowerCase()
      : `Window closes ${stop.windowClose}`;

  return (
    <li className="relative">
      <span
        aria-hidden="true"
        className={`absolute top-0 -left-6 flex size-6 items-center justify-center rounded-pill border-2 border-white text-xs font-bold text-white ${dot}`}
      >
        {state === 'done' ? <Icon name="check" size={12} /> : n}
      </span>
      <Link
        href={`/driver/stops/${stop.legId}`}
        aria-current={state === 'current' ? 'step' : undefined}
        className={`flex items-center justify-between gap-2 rounded-chip p-3 ${box}`}
      >
        <div className="flex min-w-0 flex-col gap-0.5 text-xs leading-[1.3]">
          {state === 'current' && (
            <span className="flex w-fit items-center gap-1 rounded-chip bg-info-bg px-2 py-0.5 text-[11px] font-bold uppercase text-link">
              <span className="size-1.5 rounded-pill bg-accent" />
              Current stop
            </span>
          )}
          <p className="truncate">
            <span className={`font-bold ${state === 'done' ? 'text-success' : 'text-ink-muted'}`}>
              STOP {n}
            </span>{' '}
            <span className="font-mono font-semibold text-ink">{stop.outletId}</span>{' '}
            <span className="font-semibold text-ink">{stop.outletName}</span>
          </p>
          <p className={state === 'done' ? 'text-success' : 'text-ink-muted'}>{status}</p>
        </div>
        {state === 'done' && (
          <span className="shrink-0 rounded-chip border border-success/40 bg-white px-2 py-0.5 text-[11px] font-bold uppercase text-success">
            Done
          </span>
        )}
      </Link>
    </li>
  );
}

function LastAction({ event }: { event: OutboxEvent }) {
  return (
    <Card>
      <Eyebrow>Last action</Eyebrow>
      <div className="flex items-center gap-2.5 rounded-tile bg-success-bg p-3 text-xs leading-[1.3]">
        <Icon name="circle-check" className="text-success" />
        <div className="flex flex-col gap-0.5">
          <p className="font-semibold text-ink">
            {event.outletId ?? 'Stop'} · {OUTCOME_LABEL[event.outcome].toLowerCase()}{' '}
            {time(event.occurredAt)}
          </p>
          <p className="text-ink-muted">
            {event.status === 'SYNCED'
              ? 'Uploaded — dispatch and the store can see it'
              : event.status === 'REJECTED'
                ? 'Not accepted by the server — see the log'
                : 'Saved on phone · waiting to upload'}
          </p>
        </div>
      </div>
    </Card>
  );
}
