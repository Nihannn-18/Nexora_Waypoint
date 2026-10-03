'use client';

import { useEffect, useState } from 'react';
import { api } from '../../../lib/api';
import type { OutboxEvent } from '../_lib/outbox';
import { useOutbox } from '../_lib/use-outbox';
import {
  Alert,
  Body,
  Eyebrow,
  Icon,
  SyncBadge,
  buttonClass,
  eventTitle,
  time,
} from '../_components/ui';

/** R-04 Delivery log — Figma 37:5812 (waiting) and 37:5965 (all synced). */
export default function LogPage() {
  const { events, pending, online, syncing, sync } = useOutbox();
  const [server, setServer] = useState<{ synced: number; conflicts: number } | null>(null);

  // GET /sync/status: the server's own count, shown next to ours.
  useEffect(() => {
    if (online && !syncing) api.getSyncStatus().then(setServer, () => setServer(null));
  }, [online, syncing, pending]);

  const open = events.filter((e) => e.status !== 'SYNCED');
  const done = events.filter((e) => e.status === 'SYNCED').reverse();
  const rejected = open.filter((e) => e.status === 'REJECTED').length;

  return (
    <Body>
      <div className="flex flex-col gap-1 leading-[1.3]">
        <h1 className="text-lg font-bold text-ink">Delivery log</h1>
        <p className="text-xs text-ink-muted">
          {events.length === 0
            ? 'Nothing recorded on this phone yet.'
            : pending > 0
              ? `${pending} ${pending === 1 ? 'action' : 'actions'} waiting — they upload in order automatically. You don’t need to do anything.`
              : rejected > 0
                ? 'Some actions were not accepted. Tell dispatch — they can correct it.'
                : 'Everything is uploaded. The dispatcher and stores can see it.'}
        </p>
      </div>

      {open.length > 0 && <Rows events={open} />}

      {done.length > 0 && (
        <>
          {open.length > 0 && <Eyebrow>Earlier</Eyebrow>}
          <Rows events={done} />
        </>
      )}

      {pending > 0 ? (
        <>
          <button
            type="button"
            onClick={sync}
            disabled={!online || syncing}
            className={buttonClass('outline')}
          >
            <Icon name="refresh" />
            {syncing ? 'Uploading…' : 'Upload now'}
          </button>
          <p className="text-center text-xs text-ink-muted">
            Upload starts automatically when there’s signal.
          </p>
        </>
      ) : (
        done.length > 0 &&
        rejected === 0 && (
          <Alert
            tone="success"
            eyebrow={`Reconciled ${time(done[0]?.syncedAt)}`}
            title={`No conflicts — ${done.length} ${done.length === 1 ? 'event' : 'events'} accepted`}
          >
            {server
              ? `Server confirms ${server.synced} synced · ${server.conflicts} conflicts.`
              : 'The server holds every action recorded on this phone.'}
          </Alert>
        )
      )}
    </Body>
  );
}

function Rows({ events }: { events: readonly OutboxEvent[] }) {
  return (
    <ul className="overflow-hidden rounded-tile border border-line">
      {events.map((e) => (
        <li
          key={e.clientEventId}
          className="flex items-center gap-3 border-b border-line bg-white px-4 py-3 last:border-b-0"
        >
          <Icon
            name={e.status === 'SYNCED' ? 'circle-check' : e.status === 'PENDING' ? 'clock' : 'ban'}
            className={
              e.status === 'SYNCED'
                ? 'text-success'
                : e.status === 'PENDING'
                  ? 'text-warning-ink'
                  : 'text-error-strong'
            }
          />
          <div className="flex min-w-0 flex-1 flex-col gap-0.5 leading-[1.3]">
            <p className="text-[13px] font-semibold text-ink">{eventTitle(e)}</p>
            <p className="font-mono text-[11px] text-ink-muted">
              Captured {time(e.occurredAt)}
              {e.createdOffline ? ' offline' : ''}
              {e.syncedAt ? ` · uploaded ${time(e.syncedAt)}` : ''}
              {e.status === 'PENDING' && e.retryCount > 0 ? ` · ${e.retryCount} retries` : ''}
            </p>
            {e.reason && <p className="text-[11px] text-error-strong">{e.reason}</p>}
          </div>
          <SyncBadge status={e.status} />
        </li>
      ))}
    </ul>
  );
}
