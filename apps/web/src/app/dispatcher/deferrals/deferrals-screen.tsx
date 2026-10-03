'use client';

import Link from 'next/link';
import { useMemo, useState } from 'react';
import { Mono, StatusBadge } from '@waypoint/ui';
import {
  CONSTRAINT_CATALOG,
  type ConstraintCode,
  type DeferralLogEntry,
  type Outlet,
} from '@waypoint/shared-types';
import { api } from '../../../lib/api';
import {
  addDays,
  businessDate,
  formatDateTime,
  formatDay,
  humanize,
  plural,
} from '../../../lib/format';
import { useApiQuery } from '../../../lib/use-api-query';
import {
  EmptyState,
  ErrorState,
  LoadingState,
} from '../../../components/states';
import { useDispatcherScope } from '../_components/dispatcher-context';
import { fetchAllOrders, indexBy, loadOutlets } from '../_components/data';
import { ButtonLink, ConstraintChip, PageHeader } from '../_components/ui';

/** D-07 highlights an outlet skipped this many times inside the window. */
const REPEAT_SKIPS = 2;
const REPEAT_WINDOW_DAYS = 14;

/**
 * D-07 — the deferral log. Every deferral says which rule blocked the order,
 * who decided, when, and the reason the store was given. Outlets deferred
 * twice inside 14 days are highlighted: they are the ones a fair plan must not
 * keep skipping.
 *
 * GET /deferrals returns the complete history (it is not paged), so searching
 * it here searches everything.
 */
export function DeferralsScreen() {
  const { depot, today } = useDispatcherScope();
  const log = useApiQuery(
    depot ? `deferrals:${depot.depotId}` : null,
    async () => {
      const [entries, outlets, stillDeferred] = await Promise.all([
        api.getDeferrals(),
        loadOutlets(),
        fetchAllOrders({ status: 'DEFERRED', depotId: depot?.depotId }),
      ]);
      return {
        entries,
        outlets: indexBy(outlets, 'outletId') as Map<string, Outlet>,
        stillDeferred: new Set(stillDeferred.map((o) => o.orderId)),
      };
    },
  );

  const [search, setSearch] = useState('');
  const [rule, setRule] = useState<ConstraintCode | ''>('');

  const depotEntries = useMemo(() => {
    if (!log.data || !depot) return [];
    return log.data.entries.filter(
      (e) => log.data?.outlets.get(e.outletId)?.depotId === depot.depotId,
    );
  }, [log.data, depot]);

  // Outlets deferred REPEAT_SKIPS+ times in the last REPEAT_WINDOW_DAYS days.
  const repeatOutlets = useMemo(() => {
    if (!today) return new Set<string>();
    const since = addDays(today, -REPEAT_WINDOW_DAYS);
    const counts = new Map<string, number>();
    for (const e of depotEntries) {
      if (businessDate(new Date(e.decidedAt)) >= since) {
        counts.set(e.outletId, (counts.get(e.outletId) ?? 0) + 1);
      }
    }
    return new Set(
      [...counts].filter(([, n]) => n >= REPEAT_SKIPS).map(([id]) => id),
    );
  }, [depotEntries, today]);

  const rulesPresent = useMemo(
    () =>
      [
        ...new Set(depotEntries.map((e) => e.constraintCode).filter(Boolean)),
      ] as ConstraintCode[],
    [depotEntries],
  );

  const needle = search.trim().toLowerCase();
  const shown = depotEntries.filter((e) => {
    if (rule && e.constraintCode !== rule) return false;
    if (!needle) return true;
    const outlet = log.data?.outlets.get(e.outletId);
    return [e.outletId, e.orderNumber, outlet?.name ?? ''].some((v) =>
      v.toLowerCase().includes(needle),
    );
  });

  return (
    <>
      <PageHeader
        screenId="D-07"
        title="Deferral log"
        description={`Every order left unserved at ${depot?.name ?? '…'}, with the rule that blocked it and the reason the store was given.`}
      />

      {log.loading ? (
        <LoadingState label="Loading the deferral log…" />
      ) : log.error ? (
        <ErrorState error={log.error} onRetry={log.reload} />
      ) : depotEntries.length === 0 ? (
        <EmptyState
          title="No deferrals logged yet"
          action={
            <ButtonLink href="/dispatcher/plan">Plan a delivery day</ButtonLink>
          }
        >
          A deferral is recorded when a plan is confirmed with orders the fleet
          cannot serve, each with the rule that blocked it and your reason.
          Nothing has been deferred at this depot.
        </EmptyState>
      ) : (
        <>
          <div className="mb-4 flex flex-wrap items-end gap-3 rounded-card bg-card p-3 ring-1 ring-ink/10">
            <label className="flex min-w-56 flex-1 flex-col gap-1 text-xs font-medium text-ink-muted">
              Search by outlet or order
              <input
                type="search"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder="OUT014, outlet name or order number"
                className="h-10 rounded-control bg-page px-3 text-sm text-ink ring-1 ring-ink/15"
              />
            </label>
            <label className="flex flex-col gap-1 text-xs font-medium text-ink-muted">
              Blocking rule
              <select
                value={rule}
                onChange={(e) => setRule(e.target.value as ConstraintCode | '')}
                className="h-10 rounded-control bg-page px-2 text-sm text-ink ring-1 ring-ink/15"
              >
                <option value="">All rules</option>
                {rulesPresent.map((c) => (
                  <option key={c} value={c}>
                    {CONSTRAINT_CATALOG[c].designRuleId}{' '}
                    {CONSTRAINT_CATALOG[c].label}
                  </option>
                ))}
              </select>
            </label>
            <p className="pb-2 text-sm text-ink-muted" aria-live="polite">
              Showing <Mono>{shown.length}</Mono> of{' '}
              <Mono>{depotEntries.length}</Mono>
            </p>
          </div>

          {repeatOutlets.size > 0 && (
            <p className="mb-3 rounded-control bg-warning/10 p-3 text-sm text-ink ring-1 ring-warning/30">
              <span className="font-semibold">
                ! {plural(repeatOutlets.size, 'outlet')} skipped {REPEAT_SKIPS}+
                times in {REPEAT_WINDOW_DAYS} days:
              </span>{' '}
              <Mono>{[...repeatOutlets].join(', ')}</Mono>. The planner protects
              an outlet deferred on the previous run, so check these on the next
              plan.
            </p>
          )}

          {shown.length === 0 ? (
            <EmptyState title="No deferrals match">
              Clear the search or pick another rule.
            </EmptyState>
          ) : (
            <ul className="space-y-2">
              {shown.map((e) => (
                <DeferralRow
                  key={e.deferralId}
                  entry={e}
                  outlet={log.data?.outlets.get(e.outletId)}
                  repeat={repeatOutlets.has(e.outletId)}
                  stillDeferred={
                    log.data?.stillDeferred.has(e.orderId) ?? false
                  }
                />
              ))}
            </ul>
          )}
        </>
      )}
    </>
  );
}

function DeferralRow({
  entry: e,
  outlet,
  repeat,
  stillDeferred,
}: {
  entry: DeferralLogEntry;
  outlet: Outlet | undefined;
  repeat: boolean;
  stillDeferred: boolean;
}) {
  return (
    <li
      className={`grid gap-3 rounded-card bg-card p-3 ring-1 md:grid-cols-[180px_minmax(0,1fr)_minmax(0,1.3fr)_160px] ${
        repeat ? 'ring-warning/50' : 'ring-ink/10'
      }`}
    >
      <div className="text-sm">
        <p className="font-mono text-ink">{formatDateTime(e.decidedAt)}</p>
        <p className="text-xs text-ink-muted">
          by <Mono>{e.decidedBy || 'system'}</Mono> · {humanize(e.reasonType)}
        </p>
      </div>
      <div className="min-w-0 text-sm">
        <Link
          href={`/dispatcher/queue?order=${e.orderId}&days=all`}
          className="font-mono font-semibold text-link hover:underline"
        >
          {e.orderNumber}
        </Link>
        <p className="text-ink">
          <Mono>{e.outletId}</Mono>
          {outlet && <> · {outlet.name}</>}
        </p>
        {repeat && (
          <span className="mt-1 inline-block rounded-chip bg-warning/10 px-1.5 py-0.5 text-xs font-medium text-ink ring-1 ring-warning/30">
            ! Skipped {REPEAT_SKIPS}+ times in {REPEAT_WINDOW_DAYS} days
          </span>
        )}
      </div>
      <div className="min-w-0 text-sm">
        {e.constraintCode && <ConstraintChip code={e.constraintCode} />}
        <p className="mt-1 text-ink">{e.reasonText}</p>
      </div>
      <div className="text-sm">
        <p className="text-xs font-semibold uppercase tracking-wide text-ink-muted">
          New date
        </p>
        <p className="text-ink">
          {e.deferredToDate ? formatDay(e.deferredToDate) : 'Next planning run'}
        </p>
        <div className="mt-1">
          {stillDeferred ? (
            <StatusBadge tone="warning">Still deferred</StatusBadge>
          ) : (
            <StatusBadge tone="info">Re-entered</StatusBadge>
          )}
        </div>
      </div>
    </li>
  );
}
