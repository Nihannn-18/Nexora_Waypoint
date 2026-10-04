'use client';

import Link from 'next/link';
import { useMemo, useState } from 'react';
import { Mono } from '@waypoint/ui';
import { api } from '../../../lib/api';
import { humanize } from '../../../lib/format';
import { useApiQuery } from '../../../lib/use-api-query';
import {
  EmptyState,
  ErrorState,
  LoadingState,
} from '../../../components/states';
import { PageHeader, BrandChip } from '../_components/ui';
import { useDispatcherScope } from '../_components/dispatcher-context';

/**
 * Dispatcher · Outlets. Master-data management: the full outlet network across
 * both depots, with a create entry point. A store manager never reaches this
 * screen; their own outlet is read through the store workspace.
 */
export function OutletsScreen() {
  const { depot } = useDispatcherScope();
  const [search, setSearch] = useState('');
  const outlets = useApiQuery('master-outlets', () => api.getOutlets());

  const all = outlets.data ?? [];
  const term = search.trim().toLowerCase();
  const shown = useMemo(
    () =>
      all.filter(
        (o) =>
          !term ||
          o.outletId.toLowerCase().includes(term) ||
          o.name.toLowerCase().includes(term) ||
          o.district.toLowerCase().includes(term),
      ),
    [all, term],
  );

  return (
    <>
      <PageHeader
        title="Outlets"
        description={
          <>
            Outlet windows, access and depot drive what can be planned.{' '}
            <Mono>{all.length}</Mono> outlets across both depots
            {depot ? <> (viewing all; current depot {depot.name})</> : null}.
          </>
        }
        actions={
          <Link
            href="/dispatcher/outlets/new"
            className="tap-target inline-flex items-center gap-2 rounded-control bg-action px-4 text-sm font-semibold text-card hover:bg-action/90"
          >
            Add outlet
          </Link>
        }
      />

      <div className="mb-3 flex justify-end">
        <label className="flex items-center gap-2 text-sm">
          <span className="sr-only">Search outlets</span>
          <input
            type="search"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search outlet, name or district…"
            className="tap-target w-64 rounded-control bg-card px-3 text-sm text-ink ring-1 ring-ink/15 focus-visible:ring-2 focus-visible:ring-brand"
          />
        </label>
      </div>

      {outlets.loading && !all.length ? (
        <LoadingState label="Loading outlets…" />
      ) : outlets.error ? (
        <ErrorState error={outlets.error} onRetry={outlets.reload} />
      ) : shown.length === 0 ? (
        <EmptyState title="No outlets match">
          Adjust the search, or add an outlet.
        </EmptyState>
      ) : (
        <div className="overflow-x-auto rounded-card bg-card ring-1 ring-ink/10">
          <table className="w-full min-w-[860px] text-left text-sm">
            <thead className="bg-brand/5 text-[0.6875rem] uppercase tracking-wide text-ink-muted">
              <tr>
                <th className="px-4 py-2.5 font-semibold">Outlet</th>
                <th className="px-4 py-2.5 font-semibold">Brand</th>
                <th className="px-4 py-2.5 font-semibold">District</th>
                <th className="px-4 py-2.5 font-semibold">Dock</th>
                <th className="px-4 py-2.5 font-semibold">Access</th>
                <th className="px-4 py-2.5 font-semibold">Window</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-ink/5">
              {shown.map((o) => (
                <tr key={o.outletId} className="hover:bg-page">
                  <td className="px-4 py-2.5">
                    <Link
                      href={`/dispatcher/outlets/${o.outletId}`}
                      className="font-medium text-link hover:underline"
                    >
                      <Mono className="font-semibold">{o.outletId}</Mono>
                    </Link>
                    <span className="ml-2 text-ink">{o.name}</span>
                  </td>
                  <td className="px-4 py-2.5">
                    <BrandChip brand={o.brand} />
                  </td>
                  <td className="px-4 py-2.5 text-ink">{o.district}</td>
                  <td className="px-4 py-2.5 text-ink-muted">
                    {humanize(o.dockType)}
                  </td>
                  <td className="px-4 py-2.5 text-ink-muted">
                    {humanize(o.parkingConstraint)}
                  </td>
                  <td className="px-4 py-2.5">
                    <Mono>
                      {o.windowOpenTime}–{o.windowCloseTime}
                    </Mono>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}
