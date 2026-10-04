'use client';

import Link from 'next/link';
import { use, useEffect, useState } from 'react';
import { WaypointApiError } from '@waypoint/api-client';
import { Mono, OrderStatusBadge } from '@waypoint/ui';
import type {
  ExpectedReceiptLine,
  Receipt,
  ReceiptLineCondition,
  ReceiptProofOfDelivery,
  ReceiptView,
} from '@waypoint/shared-types';
import { api } from '../../../../../lib/api';
import { formatDateTime, plural } from '../../../../../lib/format';
import { fetchMediaObjectUrl } from '../../../../../lib/media';
import {
  Card,
  Chip,
  ErrorState,
  Eyebrow,
  LoadingState,
  buttonClass,
} from '../../../_components/ui';
import {
  CONDITION_LABEL,
  initialCounts,
  previewLine,
  summarise,
  toReceiptRequest,
  type LineCount,
  type LineCounts,
} from '../../../_lib/receipt';
import { readableStoreError } from '../../../_lib/store';
import { useOrderReceipt } from '../../../_lib/use-store';

/**
 * S-06 / S-06b · Goods received note.
 *
 * The store counts what arrived against what was sent, line by line. "Sent" is
 * the loader's loaded count when the loader recorded one, so a shortfall
 * flagged at the dock is shown and pre-filled rather than discovered again.
 * The driver's proof of delivery is attached. Sending the GRN records it, moves
 * the order to RECEIVED and — when it raises an issue — notifies the
 * dispatcher; the success state shows the GRN and every issue raised.
 */
export default function ReceiptPage({
  params,
}: {
  params: Promise<{ orderId: string }>;
}) {
  const { orderId } = use(params);
  const state = useOrderReceipt(orderId);
  // The GRN the server returned on send, shown at once without a refetch.
  const [recorded, setRecorded] = useState<Receipt | null>(null);

  if (state.loading && !state.data) {
    return <LoadingState label="Loading the delivery…" />;
  }
  const view = state.data;
  if (!view) {
    return (
      <ErrorState
        title="Couldn't open this delivery"
        message={
          state.error
            ? readableStoreError(state.error, 'This order')
            : 'This order was not found for your outlet.'
        }
        onRetry={state.reload}
      />
    );
  }

  const receipt = recorded ?? view.receipt;
  return (
    <div className="flex flex-col gap-4">
      <Link
        href={`/store/orders/${view.orderId}`}
        className="text-sm font-medium text-link"
      >
        ← Back to the order
      </Link>

      <header className="flex flex-wrap items-start justify-between gap-2">
        <div className="flex flex-col gap-1">
          <Eyebrow>Goods received note</Eyebrow>
          <h1 className="text-xl font-semibold text-ink">
            <Mono>{view.orderNumber}</Mono>
          </h1>
        </div>
        <OrderStatusBadge status={receipt ? 'RECEIVED' : view.orderStatus} />
      </header>

      {receipt ? (
        <GrnRecorded receipt={receipt} justRecorded={recorded !== null} />
      ) : view.orderStatus === 'DELIVERED' ? (
        <GrnForm view={view} onRecorded={setRecorded} onStale={state.reload} />
      ) : (
        <NothingToReceive status={view.orderStatus} />
      )}

      {view.proofOfDelivery ? (
        <PodCard pod={view.proofOfDelivery} />
      ) : (
        view.orderStatus === 'DELIVERED' && (
          <Card>
            <p className="text-sm text-ink-muted">
              The driver’s proof of delivery has not reached the server yet.
            </p>
          </Card>
        )
      )}
    </div>
  );
}

/* -------------------------------------------------------------------------- */
/* S-06 · the count                                                           */
/* -------------------------------------------------------------------------- */

function GrnForm({
  view,
  onRecorded,
  onStale,
}: {
  view: ReceiptView;
  onRecorded: (receipt: Receipt) => void;
  onStale: () => void;
}) {
  const [counts, setCounts] = useState<LineCounts>(() => initialCounts(view));
  const [notes, setNotes] = useState('');
  const [sending, setSending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const summary = summarise(view.lines, counts);
  const flagged = view.lines.filter((line) => line.loaderFlag).length;

  function setCount(orderItemId: string, next: Partial<LineCount>) {
    setCounts((current) => {
      const before = current[orderItemId] ?? { received: '', damaged: '' };
      return { ...current, [orderItemId]: { ...before, ...next } };
    });
  }

  async function send() {
    if (sending || summary.invalidLines > 0) return;
    setSending(true);
    setError(null);
    try {
      const receipt = await api.createReceipt(
        view.orderId,
        toReceiptRequest(view.lines, counts, notes),
      );
      onRecorded(receipt);
    } catch (err) {
      setSending(false);
      if (err instanceof WaypointApiError && err.status === 409) {
        setError(
          'This delivery can no longer be received from here — it may already have a GRN. The page has been refreshed to show its current state.',
        );
        onStale();
        return;
      }
      setError(readableStoreError(err, 'This order'));
    }
  }

  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={(event) => {
        event.preventDefault();
        void send();
      }}
      noValidate
    >
      <Card className="gap-1 border-brand bg-info-bg">
        <h2 className="text-sm font-semibold text-ink">
          Check what arrived against what was sent
        </h2>
        <p className="text-sm text-ink-muted">
          Each line starts at the quantity that left the depot. Change it only
          where the delivery differs: count damaged items separately, and
          anything not counted is recorded as short.
          {flagged > 0 &&
            ` The loader already flagged ${plural(flagged, 'line')} before departure; those are pre-filled and are not reported again.`}
        </p>
      </Card>

      {summary.invalidLines > 0 && (
        <p
          role="alert"
          className="rounded-card border border-error bg-error-bg p-3 text-sm text-error-strong"
        >
          {plural(summary.invalidLines, 'line')} need a valid count before the
          GRN can be sent. They are marked below.
        </p>
      )}

      <Card className="gap-0 p-0">
        <ul className="flex flex-col divide-y divide-line">
          {view.lines.map((line) => (
            <CountRow
              key={line.orderItemId}
              line={line}
              count={counts[line.orderItemId]}
              onChange={(next) => setCount(line.orderItemId, next)}
            />
          ))}
        </ul>
      </Card>

      <Card>
        <label className="flex flex-col gap-1 text-sm">
          <span className="font-medium text-ink">Note (optional)</span>
          <textarea
            value={notes}
            maxLength={500}
            rows={2}
            onChange={(event) => setNotes(event.target.value)}
            placeholder="For example: outer carton crushed on one pallet"
            className="rounded-control bg-page p-2 text-ink ring-1 ring-ink/15 focus-visible:ring-2 focus-visible:ring-brand"
          />
        </label>
        <p className="text-sm text-ink">
          {summary.invalidLines > 0
            ? 'Fix the marked lines to see what this GRN reports.'
            : summary.damaged + summary.short === 0
              ? 'Nothing to report — every line arrived as sent.'
              : `This GRN reports ${summary.damaged} damaged and ${summary.short} short. The dispatcher is notified when you send it.`}
        </p>
        {error && (
          <p role="alert" className="text-sm text-error-strong">
            {error}
          </p>
        )}
        <button
          type="submit"
          disabled={sending || summary.invalidLines > 0}
          className={buttonClass('ink', 'w-full sm:w-auto')}
        >
          {sending ? 'Sending GRN…' : 'Send GRN'}
        </button>
      </Card>
    </form>
  );
}

function CountRow({
  line,
  count,
  onChange,
}: {
  line: ExpectedReceiptLine;
  count: LineCount | undefined;
  onChange: (next: Partial<LineCount>) => void;
}) {
  const preview = previewLine(line, count);
  const errorId = `count-error-${line.orderItemId}`;
  return (
    <li className="flex flex-col gap-3 p-4">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <p className="text-sm font-medium text-ink">{line.name}</p>
          <p className="text-xs text-ink-muted">
            <Mono>{line.sku}</Mono> · ordered <Mono>{line.orderedQty}</Mono> ·
            sent <Mono>{line.expectedQty}</Mono>
          </p>
        </div>
        {preview.condition && <ConditionChip condition={preview.condition} />}
      </div>

      {line.loaderFlag && (
        <div className="flex flex-wrap items-start gap-3 rounded-control border border-warning bg-warning-bg p-3">
          <p className="min-w-0 flex-1 text-sm text-warning-ink">
            <span aria-hidden="true">⚑ </span>
            Loader flagged{' '}
            {[
              line.loaderFlag.missingQty > 0 &&
                `${line.loaderFlag.missingQty} missing`,
              line.loaderFlag.damagedQty > 0 &&
                `${line.loaderFlag.damagedQty} damaged`,
            ]
              .filter(Boolean)
              .join(' and ')}{' '}
            before departure. Those did not leave the depot, so this line
            expects <Mono>{line.expectedQty}</Mono>.
          </p>
          {line.loaderFlag.photoRef && (
            <MediaThumb
              mediaKey={line.loaderFlag.photoRef}
              alt={`Loader's photo for ${line.name}`}
            />
          )}
        </div>
      )}

      <div className="flex flex-wrap items-end gap-3">
        <CountInput
          label="Received in good condition"
          value={count?.received ?? ''}
          invalid={preview.error !== null}
          describedBy={preview.error ? errorId : undefined}
          onChange={(received) => onChange({ received })}
        />
        <CountInput
          label="Damaged"
          value={count?.damaged ?? ''}
          invalid={preview.error !== null}
          describedBy={preview.error ? errorId : undefined}
          onChange={(damaged) => onChange({ damaged })}
        />
        <div className="flex flex-col gap-1 text-sm">
          <span className="text-ink-muted">Short</span>
          <span className="flex h-12 items-center font-mono tabular-nums text-ink">
            {preview.short ?? '—'}
          </span>
        </div>
      </div>
      {preview.error && (
        <p id={errorId} className="text-sm text-error-strong">
          <span aria-hidden="true">⚠ </span>
          {preview.error}
        </p>
      )}
    </li>
  );
}

function CountInput({
  label,
  value,
  invalid,
  describedBy,
  onChange,
}: {
  label: string;
  value: string;
  invalid: boolean;
  describedBy?: string;
  onChange: (value: string) => void;
}) {
  return (
    <label className="flex flex-col gap-1 text-sm">
      <span className="text-ink-muted">{label}</span>
      <input
        type="number"
        inputMode="numeric"
        min={0}
        step={1}
        value={value}
        aria-invalid={invalid}
        aria-describedby={describedBy}
        onChange={(event) => onChange(event.target.value)}
        className={`tap-target h-12 w-24 rounded-control bg-page px-2 text-right font-mono tabular-nums text-ink ring-1 focus-visible:ring-2 focus-visible:ring-brand ${invalid ? 'ring-error' : 'ring-ink/15'}`}
      />
    </label>
  );
}

/* -------------------------------------------------------------------------- */
/* S-06b · the recorded GRN                                                   */
/* -------------------------------------------------------------------------- */

function GrnRecorded({
  receipt,
  justRecorded,
}: {
  receipt: Receipt;
  justRecorded: boolean;
}) {
  const hasIssues = receipt.issues.length > 0;
  return (
    <div className="flex flex-col gap-4">
      <Card
        className={`gap-1 ${hasIssues ? 'border-warning bg-warning-bg' : 'border-success bg-success-bg'}`}
      >
        <h2
          className={`text-sm font-semibold ${hasIssues ? 'text-warning-ink' : 'text-success'}`}
        >
          <span aria-hidden="true">{hasIssues ? '⚑ ' : '✓ '}</span>
          {justRecorded ? 'GRN sent' : 'GRN recorded'} ·{' '}
          {hasIssues
            ? `${plural(receipt.issues.length, 'issue')} raised`
            : 'received in full'}
        </h2>
        <p className="text-sm text-ink">
          Received
          {receipt.receivedByName ? ` by ${receipt.receivedByName}` : ''} on{' '}
          <Mono>{formatDateTime(receipt.receivedAt)}</Mono>. The order is now
          marked received.
          {hasIssues &&
            ' The dispatcher was notified of every issue below when the GRN was sent.'}
        </p>
      </Card>

      {hasIssues && (
        <Card as="section">
          <h2 className="text-sm font-semibold text-ink">Issues raised</h2>
          <ul className="flex flex-col gap-1 text-sm text-ink">
            {receipt.issues.map((issue) => (
              <li
                key={`${issue.orderItemId}-${issue.type}`}
                className="flex flex-wrap items-center gap-2"
              >
                <ConditionChip condition={issue.type} />
                <Mono>{issue.quantity}</Mono> × {issue.name}{' '}
                <span className="text-ink-muted">
                  (<Mono>{issue.sku}</Mono>)
                </span>
              </li>
            ))}
          </ul>
        </Card>
      )}

      <Card className="gap-0 p-0">
        <ul className="flex flex-col divide-y divide-line">
          {receipt.lines.map((line) => (
            <li
              key={line.orderItemId}
              className="flex flex-wrap items-center justify-between gap-2 p-4"
            >
              <div className="min-w-0">
                <p className="text-sm font-medium text-ink">{line.name}</p>
                <p className="text-xs text-ink-muted">
                  <Mono>{line.sku}</Mono> · ordered{' '}
                  <Mono>{line.orderedQty}</Mono> · sent{' '}
                  <Mono>{line.expectedQty}</Mono>
                </p>
              </div>
              <div className="flex flex-wrap items-center gap-3 text-sm">
                <span>
                  received <Mono>{line.receivedQty}</Mono>
                </span>
                <span>
                  damaged <Mono>{line.damagedQty}</Mono>
                </span>
                <span>
                  short <Mono>{line.shortQty}</Mono>
                </span>
                <ConditionChip condition={line.condition} />
              </div>
            </li>
          ))}
        </ul>
      </Card>

      {receipt.notes && (
        <Card>
          <p className="text-sm text-ink">
            <span className="font-medium">Note:</span> {receipt.notes}
          </p>
        </Card>
      )}
    </div>
  );
}

/** The order has not reached the store yet, or never will on this run. */
function NothingToReceive({ status }: { status: string }) {
  const copy =
    status === 'FAILED'
      ? 'The driver recorded this delivery as failed, so there is nothing to receive. The dispatcher can re-plan or defer it.'
      : status === 'DEFERRED'
        ? 'This order was deferred to a later run. Its GRN opens once it is delivered.'
        : 'This order has not been recorded as delivered yet. If the driver is out of signal, the delivery appears here as soon as their phone syncs.';
  return (
    <Card>
      <h2 className="text-sm font-semibold text-ink">Nothing to receive yet</h2>
      <p className="text-sm text-ink-muted">{copy}</p>
    </Card>
  );
}

/* -------------------------------------------------------------------------- */
/* Shared pieces                                                              */
/* -------------------------------------------------------------------------- */

const CONDITION_TONE: Record<
  ReceiptLineCondition,
  { tone: 'success' | 'warning'; glyph: string }
> = {
  GOOD: { tone: 'success', glyph: '✓' },
  DAMAGED: { tone: 'warning', glyph: '✕' },
  SHORT: { tone: 'warning', glyph: '−' },
  DAMAGED_AND_SHORT: { tone: 'warning', glyph: '✕' },
};

function ConditionChip({ condition }: { condition: ReceiptLineCondition }) {
  const { tone, glyph } = CONDITION_TONE[condition];
  return (
    <Chip tone={tone} glyph={glyph}>
      {CONDITION_LABEL[condition]}
    </Chip>
  );
}

function PodCard({ pod }: { pod: ReceiptProofOfDelivery }) {
  return (
    <Card as="section">
      <h2 className="text-sm font-semibold text-ink">
        Driver’s proof of delivery
      </h2>
      <p className="text-sm text-ink">
        {pod.outcome === 'DELAYED' ? 'Delivered late' : 'Delivered'} on{' '}
        <Mono>{formatDateTime(pod.occurredAt)}</Mono>, received by{' '}
        <span className="font-medium">{pod.receiverName}</span>.
      </p>
      {(pod.fileRef || pod.signature) && (
        <div className="flex flex-wrap gap-3">
          {pod.fileRef && (
            <MediaThumb mediaKey={pod.fileRef} alt="Proof of delivery photo" />
          )}
          {pod.signature && (
            <MediaThumb
              mediaKey={pod.signature}
              alt={`Signature of ${pod.receiverName}`}
            />
          )}
        </div>
      )}
    </Card>
  );
}

/**
 * A stored photo, fetched with the session (the media route is guarded) and
 * shown from an object URL that is released when the image unmounts.
 */
function MediaThumb({ mediaKey, alt }: { mediaKey: string; alt: string }) {
  const [src, setSrc] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    let cancelled = false;
    let url: string | null = null;
    fetchMediaObjectUrl(mediaKey).then(
      (objectUrl) => {
        url = objectUrl;
        if (cancelled) URL.revokeObjectURL(objectUrl);
        else setSrc(objectUrl);
      },
      () => {
        if (!cancelled) setFailed(true);
      },
    );
    return () => {
      cancelled = true;
      if (url) URL.revokeObjectURL(url);
    };
  }, [mediaKey]);

  if (failed) {
    return <p className="text-xs text-ink-muted">{alt} could not be loaded.</p>;
  }
  if (!src) {
    return (
      <div
        role="status"
        aria-label={`Loading ${alt}`}
        className="h-24 w-24 rounded-control bg-muted"
      />
    );
  }
  return (
    // An object URL from an authenticated fetch, so a plain img, not next/image.
    <img
      src={src}
      alt={alt}
      className="h-24 w-24 rounded-control object-cover ring-1 ring-ink/15"
    />
  );
}
