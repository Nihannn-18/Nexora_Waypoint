'use client';

import Link from 'next/link';
import { useId } from 'react';
import { Mono, StatusBadge } from '@waypoint/ui';
import {
  CONSTRAINT_CATALOG,
  type ConstraintCode,
} from '@waypoint/shared-types';
import { formatKg, formatM3, plural } from '../../../lib/format';
import { BrandChip, ConstraintChip, buttonClass } from '../_components/ui';
import {
  suggestedDecision,
  type DeferralDecision,
  type DeferredView,
} from './plan-model';

/**
 * DG-A — every order the engine could not place, grouped by the rule that
 * blocked it. Each needs a reason before the plan can be confirmed; the
 * engine's own explanation is offered, per order or for a whole group, but
 * the dispatcher has to accept or rewrite it. The decision itself cannot be
 * overridden here: a hard rule left no feasible slot.
 */
export function DeferralPanel({
  deferred,
  decisions,
  onDecide,
  readOnly,
}: {
  deferred: readonly DeferredView[];
  decisions: ReadonlyMap<string, DeferralDecision>;
  onDecide: (updates: ReadonlyMap<string, DeferralDecision>) => void;
  readOnly: boolean;
}) {
  const groups = new Map<ConstraintCode | 'OTHER', DeferredView[]>();
  for (const d of deferred) {
    const key = d.constraintCode ?? 'OTHER';
    groups.set(key, [...(groups.get(key) ?? []), d]);
  }
  const ordered = [...groups.entries()].sort(
    (a, b) => b[1].length - a[1].length,
  );

  return (
    <div className="space-y-4">
      {ordered.map(([code, rows]) => {
        const missing = rows.filter(
          (r) => !decisions.get(r.orderId)?.reasonText.trim(),
        );
        const label =
          code === 'OTHER'
            ? 'No single rule named'
            : CONSTRAINT_CATALOG[code].label;
        return (
          <section
            key={code}
            aria-label={`Deferred by ${label}`}
            className="rounded-card bg-card ring-1 ring-ink/10"
          >
            <header className="flex flex-wrap items-center gap-2 border-b border-ink/10 px-4 py-3">
              {code !== 'OTHER' ? (
                <ConstraintChip code={code} />
              ) : (
                <span className="rounded-chip bg-ink/[0.06] px-1.5 py-0.5 text-xs font-medium text-ink">
                  No single rule named by the engine
                </span>
              )}
              <span className="text-sm font-medium text-ink">
                {plural(rows.length, 'order')} blocked
              </span>
              {missing.length > 0 ? (
                <StatusBadge tone="warning">
                  {missing.length} need a reason
                </StatusBadge>
              ) : (
                <StatusBadge tone="success">All have a reason</StatusBadge>
              )}
              {!readOnly && missing.length > 0 && (
                <button
                  type="button"
                  className={`${buttonClass.secondary} ml-auto`}
                  onClick={() =>
                    onDecide(
                      new Map(
                        missing.map((r) => [r.orderId, suggestedDecision(r)]),
                      ),
                    )
                  }
                >
                  Use the engine’s reason for all {missing.length}
                </button>
              )}
            </header>
            <ul className="divide-y divide-ink/5">
              {rows.map((d) => (
                <DeferralRow
                  key={d.orderId}
                  row={d}
                  decision={decisions.get(d.orderId)}
                  onDecide={(decision) =>
                    onDecide(new Map([[d.orderId, decision]]))
                  }
                  readOnly={readOnly}
                />
              ))}
            </ul>
          </section>
        );
      })}
    </div>
  );
}

function DeferralRow({
  row,
  decision,
  onDecide,
  readOnly,
}: {
  row: DeferredView;
  decision: DeferralDecision | undefined;
  onDecide: (decision: DeferralDecision) => void;
  readOnly: boolean;
}) {
  const inputId = useId();
  const hasReason = Boolean(decision?.reasonText.trim());
  const o = row.order;
  return (
    <li className="grid gap-3 px-4 py-3 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)]">
      <div className="min-w-0 text-sm">
        <div className="flex flex-wrap items-center gap-2">
          {o ? (
            <Link
              href={`/dispatcher/queue?order=${o.orderId}`}
              className="font-mono font-semibold text-link hover:underline"
            >
              {o.orderNumber}
            </Link>
          ) : (
            <Mono className="text-ink-muted">{row.orderId.slice(0, 8)}</Mono>
          )}
          {o && <BrandChip brand={o.brand} />}
        </div>
        <p className="mt-0.5 text-ink">
          <Mono>{o?.outletId ?? '—'}</Mono>
          {row.outlet && <> · {row.outlet.name}</>}
          {row.outlet && (
            <span className="text-ink-muted"> · {row.outlet.district}</span>
          )}
        </p>
        {o && (
          <p className="text-xs text-ink-muted">
            <Mono>{formatKg(o.totalWeightKg)}</Mono> ·{' '}
            <Mono>{formatM3(o.totalVolumeM3)}</Mono>
          </p>
        )}
        <p className="mt-1.5 text-xs text-ink-muted">
          <span className="font-semibold text-ink">
            Why the engine deferred it:
          </span>{' '}
          {row.explanation || 'No explanation was returned.'}
        </p>
      </div>

      <div className="min-w-0">
        <div className="flex items-center justify-between gap-2">
          <label htmlFor={inputId} className="text-xs font-semibold text-ink">
            Reason recorded for the store
          </label>
          {hasReason ? (
            <StatusBadge tone="success">Reason set</StatusBadge>
          ) : (
            <StatusBadge tone="warning">Needs a reason</StatusBadge>
          )}
        </div>
        <textarea
          id={inputId}
          rows={2}
          readOnly={readOnly}
          aria-invalid={!hasReason}
          className="mt-1 w-full resize-y rounded-control bg-page px-2.5 py-2 text-sm text-ink ring-1 ring-ink/15 read-only:opacity-80 aria-[invalid=true]:ring-warning/50"
          placeholder="Say why this order can’t go on this day, in words the store will understand."
          value={decision?.reasonText ?? ''}
          onChange={(e) =>
            onDecide({
              reasonType: decision?.reasonType ?? 'CONSTRAINT',
              reasonText: e.target.value,
            })
          }
        />
        {!readOnly && !hasReason && (
          <button
            type="button"
            className="mt-1 text-xs font-medium text-link hover:underline"
            onClick={() => onDecide(suggestedDecision(row))}
          >
            Use the engine’s reason
          </button>
        )}
      </div>
    </li>
  );
}
