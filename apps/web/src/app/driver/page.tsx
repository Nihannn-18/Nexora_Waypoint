'use client';

import Link from 'next/link';
import { useEffect, useState } from 'react';
import type { LegContext } from '@waypoint/api-client';
import { listLegs } from './_lib/outbox';
import { useOutbox } from './_lib/use-outbox';
import {
  Alert,
  Body,
  Card,
  Eyebrow,
  Icon,
  OUTCOME_LABEL,
  time,
} from './_components/ui';

/**
 * R-01 Cockpit (Figma 1:4306 online / 37:5584 offline).
 *
 * The API has no driver-facing route endpoint yet (GET /routes* is
 * dispatcher-only), so the planned-route list cannot be loaded. The cockpit
 * shows that honestly and lists the stops already cached on this phone.
 */
export default function DriverCockpitPage() {
  const { online, events } = useOutbox();
  const [legs, setLegs] = useState<LegContext[] | null>(null);

  useEffect(() => {
    listLegs().then(setLegs, () => setLegs([]));
  }, []);

  const latest = events.at(-1);
  const done = new Set(events.map((e) => e.legId));

  return (
    <Body>
      {latest && (
        <Card>
          <Eyebrow>Last action</Eyebrow>
          <div className="flex items-center gap-2.5 rounded-tile bg-success-bg p-3 text-xs leading-[1.3]">
            <Icon name="circle-check" className="text-success" />
            <div className="flex flex-col gap-0.5">
              <p className="font-semibold text-ink">
                {latest.outletId ?? 'Stop'} ·{' '}
                {OUTCOME_LABEL[latest.outcome].toLowerCase()}{' '}
                {time(latest.occurredAt)}
              </p>
              <p className="text-ink-muted">
                {latest.status === 'SYNCED'
                  ? 'Uploaded — dispatch and the store can see it'
                  : latest.status === 'REJECTED'
                    ? 'Not accepted by the server — see the log'
                    : 'Saved on phone · waiting to upload'}
              </p>
            </div>
          </div>
        </Card>
      )}

      {!online && (
        <Alert tone="info" eyebrow="Everything still works" title="Keep delivering — nothing is lost">
          Outcomes, signatures and photos are kept on this phone and upload in
          order when signal returns.
        </Alert>
      )}

      <Card>
        <div className="flex items-center justify-between border-b border-line pb-2">
          <h2 className="text-base font-bold text-ink">Your Planned Route</h2>
        </div>
        <p className="text-sm text-ink-muted">
          Today’s route isn’t available on this phone yet. Stops opened from
          dispatch stay here and keep working without signal.
        </p>
      </Card>

      {legs === null ? (
        <p className="text-center text-xs text-ink-muted" role="status">
          Loading saved stops…
        </p>
      ) : (
        legs.map((leg) => (
          <Link
            key={leg.legId}
            href={`/driver/stops/${leg.legId}`}
            className="flex items-center justify-between rounded-tile border border-line bg-white p-3"
          >
            <div className="flex flex-col gap-0.5 leading-[1.3]">
              <p className="font-mono text-xs font-bold text-ink">{leg.toOutletId}</p>
              <p className="text-xs text-ink-muted">
                {leg.orderIds.length} {leg.orderIds.length === 1 ? 'order' : 'orders'} ·{' '}
                {leg.routeDate}
              </p>
            </div>
            {done.has(leg.legId) ? (
              <span className="rounded-chip border border-success/40 bg-success-bg px-2 py-0.5 text-[11px] font-bold uppercase text-success">
                Done
              </span>
            ) : (
              <Icon name="arrow-right" className="text-ink-muted" />
            )}
          </Link>
        ))
      )}
    </Body>
  );
}
