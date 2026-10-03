'use client';

import Link from 'next/link';
import { useParams } from 'next/navigation';
import { useOutbox } from '../../_lib/use-outbox';
import { Body, Icon, OUTCOME_LABEL, buttonClass, time } from '../../_components/ui';

/**
 * Delivery saved — Figma 37:5727. The copy is honest about where the data is:
 * "saved on this phone" until the server has accepted it, "uploaded" after.
 */
export default function SavedPage() {
  const { id } = useParams<{ id: string }>();
  const { events, pending } = useOutbox();
  const e = events.find((x) => x.clientEventId === id);

  if (!e) {
    return (
      <Body>
        <p role="status" className="py-10 text-center text-sm text-ink-muted">
          Loading…
        </p>
      </Body>
    );
  }

  const status = {
    PENDING: {
      icon: 'cloud-upload',
      ink: 'text-warning-ink',
      title: 'Saved on this phone',
      body: `Uploads automatically when signal returns · ${pending} waiting`,
    },
    SYNCED: {
      icon: 'circle-check',
      ink: 'text-success',
      title: 'Uploaded',
      body: 'Dispatch and the store can see it.',
    },
    REJECTED: {
      icon: 'ban',
      ink: 'text-error-strong',
      title: 'Not accepted by the server',
      body: e.reason ?? 'Open the log for details.',
    },
  }[e.status];

  return (
    <div className="flex flex-1 flex-col items-center gap-3 px-5 pt-10 pb-4">
      <span className="rounded-pill bg-success-bg p-5 text-success">
        <Icon name="circle-check" size={44} />
      </span>
      <h1 className="text-center text-lg font-bold leading-[1.3] text-ink">
        {OUTCOME_LABEL[e.outcome]} · {time(e.occurredAt)}
      </h1>
      <p className="text-center text-sm leading-[1.3] text-ink-muted">
        {[
          e.outletId && `Outlet ${e.outletId}`,
          e.receiverName && `signed by ${e.receiverName}`,
        ]
          .filter(Boolean)
          .join(' · ')}
      </p>
      <div role="status" className="flex w-full items-center gap-2.5 rounded-tile border border-line bg-white p-3.5">
        <Icon name={status.icon} size={22} className={status.ink} />
        <div className="flex flex-col gap-0.5 text-xs leading-[1.3]">
          <p className="font-semibold text-ink">{status.title}</p>
          <p className="text-ink-muted">{status.body}</p>
        </div>
      </div>
      <div className="flex-1" />
      <Link href="/driver" className={buttonClass('ink')}>
        <Icon name="arrow-right" />
        Back to cockpit
      </Link>
      <Link href="/driver/log" className={buttonClass('outline')}>
        View upload queue
      </Link>
    </div>
  );
}
