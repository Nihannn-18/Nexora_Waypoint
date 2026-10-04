'use client';

import { useState } from 'react';
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

/** D-09 — the depot's vehicles and whether each can run on the delivery day. */
export function FleetScreen() {
  const { depot, deliveryDate } = useDispatcherScope();
  const [filter, setFilter] = useState<FleetFilter>('ALL');
  const key = depot && deliveryDate ? `${depot.depotId}:${deliveryDate}` : null;
  const vehicles = useApiQuery(key && `fleet:${key}`, () =>
    api.getVehicles({ depotId: depot?.depotId, date: deliveryDate }),
  );

  const all = vehicles.data ?? [];
  const match = FILTERS.find((f) => f.key === filter)?.match ?? (() => true);
  const shown = all.filter(match);
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
            available. Only refrigerated vehicles may carry chilled orders.
          </>
        }
      />
      <div
        role="group"
        aria-label="Vehicle type"
        className="mb-3 flex flex-wrap gap-1.5"
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

      {vehicles.loading ? (
        <LoadingState label="Loading vehicles…" />
      ) : vehicles.error ? (
        <ErrorState error={vehicles.error} onRetry={vehicles.reload} />
      ) : shown.length === 0 ? (
        <EmptyState title="No vehicles of this type at this depot" />
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
                <tr key={v.vehicleId}>
                  <td className="px-4 py-2.5">
                    <Mono className="font-semibold text-ink">
                      {v.vehicleId}
                    </Mono>
                  </td>
                  <td className="px-4 py-2.5 text-ink">
                    {v.tempClass === 'REEFER' ? 'Refrigerated' : 'Dry-box'}{' '}
                    {humanize(v.type).toLowerCase()}
                  </td>
                  <td className="px-4 py-2.5">
                    <StatusBadge
                      tone={
                        v.status === 'AVAILABLE'
                          ? 'success'
                          : v.status === 'IN_WORKSHOP'
                            ? 'warning'
                            : 'error'
                      }
                    >
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
