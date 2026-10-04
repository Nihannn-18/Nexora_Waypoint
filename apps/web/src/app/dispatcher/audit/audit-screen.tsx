'use client';

import Link from 'next/link';
import { useState } from 'react';
import { Mono, StatusBadge } from '@waypoint/ui';
import {
  AUDIT_ACTIONS,
  AUDIT_ENTITY_TYPES,
  type AuditQuery,
  type AuditRecord,
} from '@waypoint/shared-types';
import { api } from '../../../lib/api';
import { formatInstantDay, formatTime, humanize } from '../../../lib/format';
import { useApiQuery } from '../../../lib/use-api-query';
import {
  EmptyState,
  ErrorState,
  LoadingState,
} from '../../../components/states';
import { useDispatcherScope } from '../_components/dispatcher-context';
import { PageHeader, buttonClass } from '../_components/ui';
import {
  auditActionLabel,
  auditEntityLabel,
  auditFacts,
  auditSentence,
} from './audit-model';

const PAGE_SIZE = 50;

/**
 * The audit trail: an immutable, newest-first record of what happened, who did
 * it and to what. It is history, not a to-do list — actionable alerts are in
 * Notifications. Filters run on the server; pages are fetched 50 at a time.
 */
export function AuditScreen() {
  const { depot, meta } = useDispatcherScope();
  const [action, setAction] = useState('');
  const [entityType, setEntityType] = useState('');
  const [entityId, setEntityId] = useState('');
  const [day, setDay] = useState('');
  const [allDepots, setAllDepots] = useState(false);
  const [page, setPage] = useState(0);

  // A business day in the API's timezone, as an RFC 3339 range.
  const offset = (meta && /[+-]\d{2}:\d{2}$/.exec(meta.now)?.[0]) ?? 'Z';
  const query: AuditQuery = {
    action: action || undefined,
    entityType: entityType || undefined,
    entityId: entityId.trim() || undefined,
    depotId: allDepots ? undefined : depot?.depotId,
    from: day ? `${day}T00:00:00${offset}` : undefined,
    to: day ? `${day}T23:59:59${offset}` : undefined,
    limit: PAGE_SIZE,
    offset: page * PAGE_SIZE,
  };
  const ready = depot && meta;
  const records = useApiQuery(
    ready ? `audit:${JSON.stringify(query)}` : null,
    () => api.listAudit(query),
  );

  const change = (fn: () => void) => {
    fn();
    setPage(0);
  };
  const rows = records.data ?? [];
  const filtered = Boolean(action || entityType || entityId || day);

  return (
    <>
      <PageHeader
        title="Audit trail"
        description="Immutable history of operational events — orders, plans, loading, deliveries and offline syncs — newest first."
      />

      <div className="mb-4 flex flex-wrap items-end gap-3 rounded-card bg-card p-3 ring-1 ring-ink/10">
        <label className="flex flex-col gap-1 text-xs font-medium text-ink-muted">
          Action
          <select
            value={action}
            onChange={(e) => change(() => setAction(e.target.value))}
            className="h-10 rounded-control bg-page px-2 text-sm text-ink ring-1 ring-ink/15"
          >
            <option value="">All actions</option>
            {AUDIT_ACTIONS.map((a) => (
              <option key={a} value={a}>
                {auditActionLabel(a)}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-1 text-xs font-medium text-ink-muted">
          Entity
          <select
            value={entityType}
            onChange={(e) => change(() => setEntityType(e.target.value))}
            className="h-10 rounded-control bg-page px-2 text-sm text-ink ring-1 ring-ink/15"
          >
            <option value="">All entities</option>
            {AUDIT_ENTITY_TYPES.map((t) => (
              <option key={t} value={t}>
                {auditEntityLabel(t)}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-1 text-xs font-medium text-ink-muted">
          Entity id
          <input
            value={entityId}
            onChange={(e) => change(() => setEntityId(e.target.value))}
            placeholder="Exact id"
            className="h-10 w-56 rounded-control bg-page px-3 font-mono text-sm text-ink ring-1 ring-ink/15"
          />
        </label>
        <label className="flex flex-col gap-1 text-xs font-medium text-ink-muted">
          Day
          <input
            type="date"
            value={day}
            onChange={(e) => change(() => setDay(e.target.value))}
            className="h-10 rounded-control bg-page px-2 font-mono text-sm text-ink ring-1 ring-ink/15"
          />
        </label>
        <label className="flex h-10 items-center gap-2 text-sm text-ink">
          <input
            type="checkbox"
            checked={allDepots}
            onChange={(e) => change(() => setAllDepots(e.target.checked))}
            className="size-4"
          />
          Both depots
        </label>
      </div>

      {records.loading ? (
        <LoadingState label="Loading the audit trail…" />
      ) : records.error ? (
        <ErrorState error={records.error} onRetry={records.reload} />
      ) : rows.length === 0 ? (
        <EmptyState
          title={
            filtered || page > 0 ? 'No events match' : 'Nothing audited yet'
          }
        >
          {filtered
            ? 'Widen the filters, or choose another day.'
            : 'Events are recorded as loaders, drivers and the API act — loads, shortfalls, deliveries and offline syncs appear here.'}
        </EmptyState>
      ) : (
        <AuditTimeline records={rows} />
      )}

      {(page > 0 || rows.length === PAGE_SIZE) && (
        <nav
          aria-label="Pages"
          className="mt-4 flex items-center justify-between"
        >
          <button
            type="button"
            className={buttonClass.secondary}
            disabled={page === 0}
            onClick={() => setPage((p) => p - 1)}
          >
            ← Newer
          </button>
          <span className="text-xs text-ink-muted">
            Events {page * PAGE_SIZE + 1}–{page * PAGE_SIZE + rows.length}
          </span>
          <button
            type="button"
            className={buttonClass.secondary}
            disabled={rows.length < PAGE_SIZE}
            onClick={() => setPage((p) => p + 1)}
          >
            Older →
          </button>
        </nav>
      )}
    </>
  );
}

const RESULT_TONE = {
  SUCCESS: 'success',
  FAILURE: 'error',
  DENIED: 'warning',
} as const;

export function AuditTimeline({
  records,
}: {
  records: readonly AuditRecord[];
}) {
  const days: { day: string; rows: AuditRecord[] }[] = [];
  for (const r of records) {
    const day = formatInstantDay(r.occurredAt);
    const last = days[days.length - 1];
    if (last && last.day === day) last.rows.push(r);
    else days.push({ day, rows: [r] });
  }
  return (
    <div className="space-y-5">
      {days.map(({ day, rows }) => (
        <section key={day} aria-label={day}>
          <h2 className="mb-2 font-mono text-sm text-ink/80">{day}</h2>
          <ol className="relative space-y-2 border-l border-ink/15 pl-4">
            {rows.map((r) => (
              <li
                key={r.id}
                className="relative rounded-card bg-card p-3 ring-1 ring-ink/10"
              >
                <span
                  aria-hidden="true"
                  className="absolute -left-[1.3rem] top-4 size-2 rounded-pill bg-action"
                />
                <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
                  <Mono className="text-sm text-ink">
                    {formatTime(r.occurredAt)}
                  </Mono>
                  <span className="text-sm font-semibold text-ink">
                    {auditActionLabel(r.action)}
                  </span>
                  {r.result && (
                    <StatusBadge tone={RESULT_TONE[r.result] ?? 'neutral'}>
                      {humanize(r.result)}
                    </StatusBadge>
                  )}
                  <span className="ml-auto text-xs text-ink-muted">
                    {r.actor ? (
                      <>
                        by <Mono className="text-ink">{r.actor}</Mono>
                        {r.role && <> · {humanize(r.role)}</>}
                      </>
                    ) : (
                      'by the system'
                    )}
                  </span>
                </div>
                <p className="mt-1 text-sm text-ink">{auditSentence(r)}</p>
                <dl className="mt-1.5 flex flex-wrap gap-x-4 gap-y-1 text-xs">
                  <div>
                    <dt className="inline text-ink-muted">
                      {auditEntityLabel(r.entityType)}:{' '}
                    </dt>
                    <dd className="inline">
                      <Mono className="text-ink">
                        {r.entityId ? r.entityId.slice(0, 8) : '—'}
                      </Mono>
                    </dd>
                  </div>
                  {auditFacts(r).map((f) => (
                    <div key={f.label}>
                      <dt className="inline text-ink-muted">{f.label}: </dt>
                      <dd className="inline text-ink">
                        {f.href ? (
                          <Link
                            href={f.href}
                            className="font-mono text-link hover:underline"
                          >
                            {f.value.slice(0, 8)}
                          </Link>
                        ) : (
                          f.value
                        )}
                      </dd>
                    </div>
                  ))}
                </dl>
              </li>
            ))}
          </ol>
        </section>
      ))}
    </div>
  );
}
