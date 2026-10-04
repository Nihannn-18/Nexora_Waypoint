'use client';

import Link from 'next/link';
import { ClockTimeText, Mono } from '@waypoint/ui';
import {
  formatKg,
  formatM3,
  formatMinutes,
  humanize,
  plural,
} from '../../../lib/format';
import { BrandChip, Meter, TempChip } from '../_components/ui';
import type { TripView } from './plan-model';

/**
 * One proposed vehicle trip. The header answers "which vehicle, which trip,
 * which brand and district"; the meters answer "how full and how long"; the
 * stops list answers "who, in what order". Driver names, plates and live
 * temperatures in the Day 5 frame are not in the data, so they are not shown.
 */
export function TripCard({ trip }: { trip: TripView }) {
  const v = trip.vehicle;
  const poolLabel =
    trip.brand === 'FRESH'
      ? 'Fresh minutes today'
      : 'Style + Tech minutes today';
  return (
    <article className="rounded-card bg-card ring-1 ring-ink/10">
      <header className="flex flex-wrap items-center gap-x-3 gap-y-1.5 border-b border-ink/10 px-4 py-3">
        <Mono className="text-base font-bold text-ink">{trip.vehicleId}</Mono>
        <span className="rounded-chip bg-ink/[0.06] px-1.5 py-0.5 text-xs font-semibold text-ink">
          Trip {trip.tripNo}
        </span>
        {v && (
          <span className="text-xs text-ink-muted">
            {v.tempClass === 'REEFER' ? 'Reefer' : 'Dry-box'}{' '}
            {humanize(v.type).toLowerCase()}
          </span>
        )}
        {trip.brand && <BrandChip brand={trip.brand} />}
        {trip.district && (
          <span className="text-sm font-medium text-ink">{trip.district}</span>
        )}
        <span className="ml-auto text-xs text-ink-muted">
          {plural(trip.stops.length, 'stop')} ·{' '}
          {formatMinutes(trip.tripMinutes)} trip
        </span>
      </header>

      <div className="grid gap-3 px-4 py-3 sm:grid-cols-3">
        <Meter
          label="Weight load"
          value={trip.weightKg}
          limit={v?.weightCapKg}
          format={formatKg}
        />
        <Meter
          label="Cube volume"
          value={trip.volumeM3}
          limit={v?.volumeCapM3}
          format={formatM3}
        />
        <Meter
          label="Time budget"
          value={trip.vehicleDayMinutes}
          limit={trip.budgetMinutes}
          format={formatMinutes}
          note={poolLabel}
        />
      </div>

      <details className="group border-t border-ink/10">
        <summary className="tap-target flex cursor-pointer list-none items-center gap-2 px-4 text-sm text-ink-muted hover:text-ink">
          <span aria-hidden="true" className="transition group-open:rotate-90">
            ▸
          </span>
          <span className="min-w-0 truncate">
            Stops in delivery order:{' '}
            <Mono className="text-ink">
              {trip.stops.map((s) => s.order?.outletId ?? '—').join(' → ')}
            </Mono>
          </span>
        </summary>
        <ol className="divide-y divide-ink/5 px-4 pb-3">
          {trip.stops.map((s, i) => (
            <li
              key={s.orderId}
              className="flex flex-wrap items-center gap-x-3 gap-y-1 py-2 text-sm"
            >
              <span
                aria-label={`Stop ${i + 1}`}
                className="grid size-6 shrink-0 place-items-center rounded-pill bg-action text-xs font-semibold text-card"
              >
                {i + 1}
              </span>
              <span className="min-w-0 flex-1">
                <span className="font-medium text-ink">
                  <Mono>{s.order?.outletId ?? '—'}</Mono>
                  {s.outlet && <> · {s.outlet.name}</>}
                </span>
                <span className="block text-xs text-ink-muted">
                  {s.order ? (
                    <Link
                      href={`/dispatcher/queue?order=${s.order.orderId}`}
                      className="text-link hover:underline"
                    >
                      <Mono>{s.order.orderNumber}</Mono>
                    </Link>
                  ) : (
                    'Order details unavailable'
                  )}
                  {s.order && (
                    <>
                      {' '}
                      · <Mono>{formatKg(s.order.totalWeightKg)}</Mono> ·{' '}
                      <Mono>{formatM3(s.order.totalVolumeM3)}</Mono>
                    </>
                  )}
                </span>
              </span>
              {s.order && <TempChip temp={s.order.temperatureRequirement} />}
              {s.outlet && (
                <span className="text-xs text-ink-muted">
                  Window{' '}
                  <ClockTimeText
                    value={s.outlet.windowOpenTime}
                    className="text-ink"
                  />
                  –
                  <ClockTimeText
                    value={s.outlet.windowCloseTime}
                    className="text-ink"
                  />
                  {s.outlet.parkingConstraint === 'VAN_ONLY' &&
                    ' · van-only access'}
                  {s.outlet.mallWindow && (
                    <>
                      {' '}
                      · mall {s.outlet.mallWindow.open}–
                      {s.outlet.mallWindow.close}
                    </>
                  )}
                </span>
              )}
            </li>
          ))}
        </ol>
      </details>
    </article>
  );
}
