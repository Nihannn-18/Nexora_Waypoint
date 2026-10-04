'use client';

import Link from 'next/link';
import { useMemo, useState } from 'react';
import { Mono, StatusBadge } from '@waypoint/ui';
import type { Vehicle } from '@waypoint/shared-types';
import { api } from '../../../lib/api';
import {
  formatDay,
  formatKg,
  formatLitres,
  formatM3,
  humanize,
} from '../../../lib/format';
import { useApiQuery } from '../../../lib/use-api-query';
import {
  EmptyState,
  ErrorState,
  LoadingState,
} from '../../../components/states';
import { useDispatcherScope } from '../_components/dispatcher-context';
import { PageHeader } from '../_components/ui';
import { vehicleStatusTone, vehicleSummary } from '../_components/master-data';

type FleetFilter = 'ALL' | 'REEFER' | 'DRY' | 'VAN';

const FILTERS: {
  key: FleetFilter;
  label: string;
  match: (v: Vehicle) => boolean;
}[] = [
  { key: 'ALL', label: 'All', match: () => true },
  {
    key: 'REEFER',
    label: 'Refrigerated',
    match: (v) => v.tempClass === 'REEFER',
  },
  {
    key: 'DRY',
    label: 'Dry-box',
    match: (v) => v.tempClass === 'AMBIENT' && v.type === 'TRUCK',
  },
  { key: 'VAN', label: 'Vans', match: (v) => v.type === 'VAN' },
];

/**
 * D-09 — the depot's vehicles and whether each can run on the delivery day.
 *
 * This is also the one place vehicles are managed: each vehicle id opens its
 * detail (edit, driver assignment) and "Add vehicle" creates one. A separate
 * Vehicles list used to repeat this table without the delivery-day context.
 */
export function FleetScreen() {
  const { depot, deliveryDate } = useDispatcherScope();
  const [filter, setFilter] = useState<FleetFilter>('ALL');
  const [search, setSearch] = useState('');
  const key = depot && deliveryDate ? `${depot.depotId}:${deliveryDate}` : null;
  const vehicles = useApiQuery(key && `fleet:${key}`, () =>
    api.getVehicles({ depotId: depot?.depotId, date: deliveryDate }),
  );

  const all = vehicles.data ?? [];
  const match = FILTERS.find((f) => f.key === filter)?.match ?? (() => true);
  const term = search.trim().toLowerCase();
  const shown = useMemo(
    () =>
      all
        .filter(match)
        .filter(
          (v) =>
            !term ||
            v.vehicleId.toLowerCase().includes(term) ||
            v.fuelType.toLowerCase().includes(term),
        ),
    [all, match, term],
  );
  const available = all.filter((v) => v.status === 'AVAILABLE').length;

  return (
    <>
      <PageHeader
        screenId="D-09"
        title="Fleet"
        description={
          <>
            {depot?.name ?? '…'} on{' '}
            {deliveryDate ? formatDay(deliveryDate) : '…'}:{' '}
            <Mono>{available}</Mono> of <Mono>{all.length}</Mono> vehicles
            available. Only refrigerated vehicles may carry chilled orders. Open
            a vehicle to edit it or assign its driver.
          </>
        }
        actions={
          <Link
            href="/dispatcher/vehicles/new"
            className="tap-target inline-flex items-center gap-2 rounded-control bg-action px-4 text-sm font-semibold text-card hover:bg-action/90"
          >
            Add vehicle
          </Link>
        }
      />
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <div
          role="group"
          aria-label="Vehicle type"
          className="flex flex-wrap gap-1.5"
        >
          {FILTERS.map((f) => (
            <button
              key={f.key}
              type="button"
              aria-pressed={filter === f.key}
              onClick={() => setFilter(f.key)}
              className={`h-9 rounded-control px-3 text-xs font-medium ring-1 ${
                filter === f.key
                  ? 'bg-action text-card ring-action'
                  : 'bg-card text-ink ring-ink/15 hover:bg-page'
              }`}
            >
              {f.label} ({all.filter(f.match).length})
            </button>
          ))}
        </div>
        <label className="flex w-full items-center gap-2 text-sm sm:ml-auto sm:w-auto">
          <span className="sr-only">Search vehicles</span>
          <input
            type="search"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search vehicle id or fuel…"
            className="tap-target w-full rounded-control bg-card px-3 text-sm text-ink ring-1 ring-ink/15 focus-visible:ring-2 focus-visible:ring-brand sm:w-56"
          />
        </label>
      </div>

      {vehicles.loading ? (
        <LoadingState label="Loading vehicles…" />
      ) : vehicles.error ? (
        <ErrorState error={vehicles.error} onRetry={vehicles.reload} />
      ) : shown.length === 0 ? (
        <EmptyState title="No vehicles match">
          Adjust the filter or search, switch depot, or add a vehicle.
        </EmptyState>
      ) : (
        <div className="overflow-x-auto rounded-card bg-card ring-1 ring-ink/10">
          <table className="w-full min-w-[720px] text-left text-sm">
            <thead className="bg-brand/5 text-[0.6875rem] uppercase tracking-wide text-ink-muted">
              <tr>
                <th className="px-4 py-2.5 font-semibold">Vehicle</th>
                <th className="px-4 py-2.5 font-semibold">Type</th>
                <th className="px-4 py-2.5 font-semibold">Availability</th>
                <th className="px-4 py-2.5 text-right font-semibold">
                  Weight cap
                </th>
                <th className="px-4 py-2.5 text-right font-semibold">
                  Volume cap
                </th>
                <th className="px-4 py-2.5 text-right font-semibold">Fuel</th>
                <th className="px-4 py-2.5 text-right font-semibold">
                  Weekly quota
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-ink/5">
              {shown.map((v) => (
                <tr key={v.vehicleId} className="hover:bg-page">
                  <td className="px-4 py-2.5">
                    <Link
                      href={`/dispatcher/vehicles/${v.vehicleId}`}
                      className="font-medium text-link hover:underline"
                    >
                      <Mono className="font-semibold">{v.vehicleId}</Mono>
                    </Link>
                  </td>
                  <td className="px-4 py-2.5 text-ink">{vehicleSummary(v)}</td>
                  <td className="px-4 py-2.5">
                    <StatusBadge tone={vehicleStatusTone(v.status)}>
                      {humanize(v.status)}
                    </StatusBadge>
                  </td>
                  <td className="px-4 py-2.5 text-right">
                    <Mono>{formatKg(v.weightCapKg)}</Mono>
                  </td>
                  <td className="px-4 py-2.5 text-right">
                    <Mono>{formatM3(v.volumeCapM3)}</Mono>
                  </td>
                  <td className="px-4 py-2.5 text-right text-ink-muted">
                    {v.fuelType} · <Mono>{v.kmPerL}</Mono> km/L
                  </td>
                  <td className="px-4 py-2.5 text-right">
                    <Mono>{formatLitres(v.weeklyFuelQuotaL)}</Mono>
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
