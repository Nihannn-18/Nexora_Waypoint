'use client';

import { useId, useState } from 'react';
import type { LoadingLine } from '@waypoint/api-client';
import { Mono } from '@waypoint/ui';
import { Chip, Eyebrow, buttonClass } from './ui';
import type { LineDraft } from '../_lib/loading';
import {
  countedQty,
  hasShortfall,
  isLineComplete,
  lineError,
  remainingQty,
} from '../_lib/loading';

/**
 * One order line on the picking list (L-02), plus the flag sheet (L-03).
 *
 * The row is built for a dock: the ordered quantity is the largest thing on it,
 * the common case ("all of it went on") is one tap, and anything short has to
 * be said explicitly rather than left as a silent difference.
 */
export function LineRow({
  line,
  draft,
  touched,
  onChange,
  onFlag,
}: {
  line: LoadingLine;
  draft: LineDraft;
  /** Whether the loader has entered anything on this line yet. */
  touched: boolean;
  onChange: (next: LineDraft) => void;
  onFlag: () => void;
}) {
  const complete = isLineComplete(line, draft);
  const short = hasShortfall(draft);
  const remaining = remainingQty(line, draft);
  const counted = countedQty(draft);
  // An untouched line is not a mistake — it is simply work still to do, and the
  // chip already says so. Flagging every unstarted row in red on open would
  // make the whole list look wrong before the loader has lifted anything.
  const error = touched || counted > line.orderedQty ? lineError(line, draft) : null;

  return (
    <article
      className={`flex flex-col gap-3 rounded-control border p-3 ${
        short
          ? 'border-error bg-error-bg/30'
          : complete
            ? 'border-success bg-success-bg/40'
            : 'border-line bg-card'
      }`}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 flex-col gap-0.5">
          <Eyebrow>
            <Mono>{line.sku}</Mono>
          </Eyebrow>
          <p className="truncate text-sm font-semibold text-ink">{line.name}</p>
        </div>
        {complete ? (
          short ? (
            <Chip tone="error" glyph="⚠">
              {line.orderedQty - draft.loadedQty} short
            </Chip>
          ) : (
            <Chip tone="success" glyph="✓">
              Loaded
            </Chip>
          )
        ) : (
          <Chip tone="neutral" glyph="○">
            {remaining > 0 ? `${remaining} to count` : 'Over'}
          </Chip>
        )}
      </div>

      <div className="flex items-end justify-between gap-3">
        <div className="flex flex-col">
          <Eyebrow>Ordered</Eyebrow>
          <p className="font-mono text-2xl font-semibold tabular-nums text-ink">
            {line.orderedQty}
          </p>
        </div>
        <div className="flex flex-col">
          <Eyebrow>Counted</Eyebrow>
          <p
            className={`font-mono text-2xl font-semibold tabular-nums ${
              error ? 'text-error-strong' : 'text-ink'
            }`}
          >
            {counted}
          </p>
        </div>
        <div className="flex flex-col text-right">
          <Eyebrow>Weight</Eyebrow>
          <p className="font-mono text-sm tabular-nums text-ink-muted">
            {line.weightKg.toFixed(1)} kg
          </p>
        </div>
      </div>

      <div className="flex gap-2">
        <button
          type="button"
          className={buttonClass(complete && !short ? 'success' : 'ink')}
          aria-pressed={complete && !short}
          onClick={() =>
            onChange({
              loadedQty: line.orderedQty,
              damagedQty: 0,
              missingQty: 0,
              photoRef: undefined,
            })
          }
        >
          {complete && !short ? '✓ All loaded' : 'Load all'}
        </button>
        <button
          type="button"
          className={buttonClass('danger')}
          onClick={onFlag}
        >
          Flag short
        </button>
      </div>

      {short && (
        <p className="text-xs text-ink-muted">
          <Mono>{draft.loadedQty}</Mono> loaded ·{' '}
          <Mono>{draft.damagedQty}</Mono> damaged ·{' '}
          <Mono>{draft.missingQty}</Mono> missing
          {draft.photoRef && ' · photo attached'}
        </p>
      )}

      {error && (
        <p className="text-xs font-medium text-error-strong" role="alert">
          {error}
        </p>
      )}
    </article>
  );
}

/**
 * L-03 · Flag an item.
 *
 * Damaged and missing are counted separately because the backend stores them
 * separately and a store manager's GRN depends on which it was. There is no
 * reason picker: the API has no shortfall-reason field, and inventing one here
 * would show the loader a choice that is then thrown away.
 */
export function FlagSheet({
  line,
  draft,
  busy,
  uploadError,
  onUploadPhoto,
  onRemovePhoto,
  onCancel,
  onApply,
}: {
  line: LoadingLine;
  draft: LineDraft;
  busy: boolean;
  uploadError: string | null;
  onUploadPhoto: (file: File) => void;
  onRemovePhoto: () => void;
  onCancel: () => void;
  onApply: (next: LineDraft) => void;
}) {
  const [damaged, setDamaged] = useState(draft.damagedQty);
  const [missing, setMissing] = useState(draft.missingQty);
  const titleId = useId();

  const loaded = line.orderedQty - damaged - missing;
  const invalid = loaded < 0;

  return (
    <div
      className="fixed inset-0 z-30 flex items-end justify-center bg-ink/40 p-0 sm:items-center sm:p-4"
      role="dialog"
      aria-modal="true"
      aria-labelledby={titleId}
    >
      <div className="flex max-h-[90dvh] w-full max-w-lg flex-col gap-4 overflow-y-auto rounded-t-card bg-card p-4 sm:rounded-card">
        <header className="flex flex-col gap-1">
          <Eyebrow>
            <Mono>{line.sku}</Mono> · Stop {line.seq} · {line.outletId}
          </Eyebrow>
          <h2 id={titleId} className="text-lg font-semibold text-ink">
            {line.name}
          </h2>
          <p className="text-sm text-ink-muted">
            Ordered <Mono>{line.orderedQty}</Mono>. Count what is damaged or
            missing — the rest is treated as loaded.
          </p>
        </header>

        <div className="flex gap-3">
          <Counter
            label="Damaged"
            value={damaged}
            max={line.orderedQty - missing}
            onChange={setDamaged}
          />
          <Counter
            label="Missing"
            value={missing}
            max={line.orderedQty - damaged}
            onChange={setMissing}
          />
        </div>

        <div className="flex items-center justify-between rounded-control bg-muted px-3 py-2">
          <span className="text-sm font-medium text-ink">Loaded</span>
          <span
            className={`font-mono text-xl font-semibold tabular-nums ${
              invalid ? 'text-error-strong' : 'text-ink'
            }`}
          >
            {loaded}
          </span>
        </div>

        {invalid && (
          <p className="text-sm font-medium text-error-strong" role="alert">
            That is more than the {line.orderedQty} ordered. Reduce the damaged
            or missing count.
          </p>
        )}

        <div className="flex flex-col gap-2">
          <Eyebrow>Photo (optional)</Eyebrow>
          {draft.photoRef ? (
            <div className="flex items-center justify-between gap-2 rounded-control border border-line p-2">
              <span className="truncate text-xs text-ink-muted">
                Photo attached
              </span>
              <button
                type="button"
                onClick={onRemovePhoto}
                className="tap-target px-3 text-sm font-semibold text-link"
              >
                Remove
              </button>
            </div>
          ) : (
            <label className={`${buttonClass('outline')} cursor-pointer`}>
              {busy ? 'Uploading…' : 'Add photo'}
              <input
                type="file"
                accept="image/jpeg,image/png,image/webp,image/heic"
                capture="environment"
                className="sr-only"
                disabled={busy}
                onChange={(e) => {
                  const file = e.target.files?.[0];
                  if (file) onUploadPhoto(file);
                }}
              />
            </label>
          )}
          {uploadError && (
            <p className="text-xs font-medium text-error-strong" role="alert">
              {uploadError}
            </p>
          )}
        </div>

        <div className="flex gap-2 pt-1">
          <button
            type="button"
            className={buttonClass('outline')}
            onClick={onCancel}
          >
            Cancel
          </button>
          <button
            type="button"
            className={buttonClass('ink')}
            disabled={invalid || busy}
            onClick={() =>
              onApply({
                loadedQty: loaded,
                damagedQty: damaged,
                missingQty: missing,
                photoRef: draft.photoRef,
              })
            }
          >
            Apply
          </button>
        </div>
      </div>
    </div>
  );
}

/** A large stepper. Typing is allowed too, for a keyboard on the dock desktop. */
function Counter({
  label,
  value,
  max,
  onChange,
}: {
  label: string;
  value: number;
  max: number;
  onChange: (next: number) => void;
}) {
  const id = useId();
  const clamp = (n: number) => Math.max(0, Math.min(max, n));

  return (
    <div className="flex flex-1 flex-col gap-1.5">
      <label htmlFor={id} className="text-xs font-semibold text-ink">
        {label}
      </label>
      <div className="flex items-center gap-2">
        <button
          type="button"
          aria-label={`One fewer ${label.toLowerCase()}`}
          className="tap-target aspect-square rounded-control border border-line-strong text-lg font-bold text-ink"
          onClick={() => onChange(clamp(value - 1))}
        >
          −
        </button>
        <input
          id={id}
          type="number"
          inputMode="numeric"
          min={0}
          max={max}
          value={value}
          onChange={(e) => onChange(clamp(Number(e.target.value) || 0))}
          className="tap-target w-full rounded-control border border-line-strong bg-card text-center font-mono text-xl font-semibold tabular-nums text-ink"
        />
        <button
          type="button"
          aria-label={`One more ${label.toLowerCase()}`}
          className="tap-target aspect-square rounded-control border border-line-strong text-lg font-bold text-ink"
          onClick={() => onChange(clamp(value + 1))}
        >
          +
        </button>
      </div>
    </div>
  );
}
