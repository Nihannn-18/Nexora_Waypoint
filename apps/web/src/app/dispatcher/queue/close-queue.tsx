'use client';

import { useState } from 'react';
import { BRANDS, type Brand } from '@waypoint/shared-types';
import { api } from '../../../lib/api';
import { plural } from '../../../lib/format';
import { describeApiError } from '../../../components/states';
import { ConfirmationDialog } from '../_components/confirmation-dialog';
import { BrandChip, buttonClass } from '../_components/ui';

/**
 * D-02 — Close the planning queue for the day. Closing is explicit and per
 * brand: the dispatcher ticks the brands whose intake is frozen, sees how many
 * confirmed orders sit in each, and confirms. Once closed the queue is
 * read-only; the API records the closure so a reload cannot reopen it.
 *
 * The button is disabled while a close is in flight, so the action cannot be
 * doubled, and a repeat close of an already-closed brand is reported as such
 * instead of failing.
 */
export function CloseQueueButton({
  date,
  depotId,
  counts,
  closedBrands,
  onClosed,
}: {
  date: string;
  depotId: string;
  counts: Record<Brand, number>;
  closedBrands: readonly string[];
  onClosed: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [selected, setSelected] = useState<Set<Brand>>(
    () => new Set(BRANDS.filter((b) => !closedBrands.includes(b))),
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [done, setDone] = useState<string | null>(null);

  const alreadyAllClosed = BRANDS.every((b) => closedBrands.includes(b));
  const toggle = (b: Brand) =>
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(b)) next.delete(b);
      else next.add(b);
      return next;
    });

  const close = async () => {
    setBusy(true);
    setError(null);
    try {
      const result = await api.closeQueue({
        date,
        depotId,
        brands: [...selected],
      });
      setDone(
        result.closed > 0
          ? `${plural(result.closed, 'brand queue')} closed.`
          : 'Those queues were already closed.',
      );
      setOpen(false);
      onClosed();
    } catch (e) {
      setError(e);
      setBusy(false);
    }
  };

  const selectedCount = [...selected].reduce((n, b) => n + counts[b], 0);

  return (
    <>
      <div className="flex flex-wrap items-center gap-2">
        <button
          type="button"
          className={buttonClass.secondary}
          onClick={() => {
            setDone(null);
            setOpen(true);
          }}
          disabled={alreadyAllClosed}
        >
          {alreadyAllClosed ? 'Queue closed' : 'Close queue'}
        </button>
        {done && (
          <span role="status" className="text-xs font-medium text-success">
            ✓ {done}
          </span>
        )}
      </div>

      <ConfirmationDialog
        open={open}
        eyebrow="Close queue"
        title={`Close the queue for ${date}?`}
        confirmLabel="Close selected queues"
        busyLabel="Closing…"
        busy={busy}
        confirmDisabled={selected.size === 0}
        onConfirm={close}
        onCancel={() => setOpen(false)}
      >
        <div className="space-y-3">
          <p>
            Closing freezes intake for the brands you tick. Confirmed orders are
            locked into this day&apos;s plan; orders that arrive later roll to
            the next operating run. A closed queue cannot be reopened from this
            screen.
          </p>
          <ul className="space-y-1.5">
            {BRANDS.map((b) => {
              const isClosed = closedBrands.includes(b);
              return (
                <li key={b}>
                  <label
                    className={`flex items-center gap-3 rounded-control px-3 py-2 ring-1 ${
                      isClosed
                        ? 'bg-success/5 ring-success/20'
                        : 'bg-page ring-ink/10'
                    }`}
                  >
                    <input
                      type="checkbox"
                      className="size-4"
                      disabled={isClosed}
                      checked={isClosed || selected.has(b)}
                      onChange={() => toggle(b)}
                    />
                    <BrandChip brand={b} />
                    <span className="text-sm text-ink">
                      {plural(counts[b], 'confirmed order')}
                    </span>
                    {isClosed && (
                      <span className="ml-auto text-xs font-medium text-success">
                        Already closed
                      </span>
                    )}
                  </label>
                </li>
              );
            })}
          </ul>
          {error ? (
            <p role="alert" className="text-sm text-error">
              {describeApiError(error).title}. Nothing was closed.
            </p>
          ) : (
            <p className="text-xs text-ink-muted">
              {selected.size === 0
                ? 'Pick at least one brand to close.'
                : `${plural(selectedCount, 'order')} across ${plural(selected.size, 'brand')} will be frozen.`}
            </p>
          )}
        </div>
      </ConfirmationDialog>
    </>
  );
}
