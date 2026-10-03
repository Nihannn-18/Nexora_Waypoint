'use client';

import Link from 'next/link';
import { useParams } from 'next/navigation';
import { useLeg, useOutbox } from '../../_lib/use-outbox';
import {
  Body,
  Eyebrow,
  Icon,
  OUTCOME_LABEL,
  SyncBadge,
  buttonClass,
  time,
} from '../../_components/ui';

/**
 * Stop details — Figma "Next stop" card (37:5662) and the cockpit actions
 * (37:5708). Shows only what GET /legs/{id} returns to a driver.
 */
export default function StopPage() {
  const { legId } = useParams<{ legId: string }>();
  const state = useLeg(legId);
  const { events } = useOutbox();
  const recorded = events.filter((e) => e.legId === legId).at(-1);

  if (state.status === 'loading') {
    return (
      <Body>
        <p role="status" className="py-10 text-center text-sm text-ink-muted">
          Loading stop…
        </p>
      </Body>
    );
  }
  if (state.status === 'error') {
    return (
      <Body>
        <p role="alert" className="rounded-tile bg-white p-4 text-sm text-ink">
          {state.message}
        </p>
        <Link href="/driver" className={buttonClass('outline')}>
          Back to cockpit
        </Link>
      </Body>
    );
  }

  const { leg, cached } = state;
  return (
    <Body>
      <section className="flex flex-col gap-2.5 rounded-tile border-2 border-accent bg-white p-4">
        <Eyebrow className="text-link">
          Stop · {leg.status.replaceAll('_', ' ')}
        </Eyebrow>
        <h1 className="text-[17px] font-medium leading-[1.3] text-ink">
          Outlet <span className="font-mono">{leg.toOutletId}</span>
        </h1>
        <div className="flex gap-2">
          <Stat label="Orders" value={String(leg.orderIds.length)} />
          <Stat label="Route date" value={leg.routeDate} />
        </div>
        {cached && (
          <p className="text-xs text-ink-muted">
            Showing the copy saved on this phone.
          </p>
        )}
      </section>

      {recorded ? (
        <div className="flex items-center gap-3 rounded-tile border border-line bg-white p-3.5">
          <div className="flex flex-1 flex-col gap-0.5 text-xs leading-[1.3]">
            <p className="font-semibold text-ink">
              {OUTCOME_LABEL[recorded.outcome]} · {time(recorded.occurredAt)}
            </p>
            {recorded.reason && <p className="text-error-strong">{recorded.reason}</p>}
          </div>
          <SyncBadge status={recorded.status} />
        </div>
      ) : (
        <div className="flex flex-col gap-2.5">
          <Link
            href={`/driver/stops/${legId}/outcome?o=DELIVERED`}
            className={buttonClass('success')}
          >
            <Icon name="thumbs-up" />
            Confirm delivery
          </Link>
          <Link
            href={`/driver/stops/${legId}/outcome?o=FAILED`}
            className={buttonClass('danger')}
          >
            <Icon name="ban" />
            Report failure / delay
          </Link>
        </div>
      )}
    </Body>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-1 flex-col gap-0.5 rounded-tile bg-page p-2.5 leading-[1.3]">
      <Eyebrow>{label}</Eyebrow>
      <p className="font-mono text-[13px] font-bold text-ink">{value}</p>
    </div>
  );
}
