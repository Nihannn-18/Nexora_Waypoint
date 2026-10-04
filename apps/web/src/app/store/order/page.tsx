'use client';

import Link from 'next/link';
import { useEffect, useMemo, useState, type FormEvent } from 'react';
import { Mono } from '@waypoint/ui';
import type { CustomerOrder, Item } from '@waypoint/shared-types';
import { formatDay, formatKg, formatM3, plural } from '../../../lib/format';
import { api } from '../../../lib/api';
import { StoreDayLine, useStoreScope } from '../_components/store-shell';
import {
  ButtonLink,
  Card,
  ErrorState,
  Eyebrow,
  Fact,
  LoadingState,
  TempChip,
  buttonClass,
} from '../_components/ui';
import { useOutletItems } from '../_lib/use-store';
import {
  draftSubmission,
  nextDeliveryDay,
  readableStoreError,
  type DraftQuantities,
} from '../_lib/store';

/**
 * S-02 / S-03 · Place an order.
 *
 * The order is always for the authenticated caller's outlet: the `outletId` in
 * the request comes from the outlet the server returned for this session, never
 * from a field the store manager can change. Totals are computed from the
 * catalogue dimensions for the running meter, but the server recomputes them
 * from its own snapshots — the client figure is a courtesy, not the record.
 *
 * Chilled and dry lines cannot share one order (the backend derives a single
 * temperature per order), so the form groups items and tells the store to
 * submit them separately rather than silently merging them.
 */
export default function PlaceOrderPage() {
  const { outlet, outletLoading, meta } = useStoreScope();
  const brand = outlet?.brand ?? null;
  const items = useOutletItems(brand);

  const [quantities, setQuantities] = useState<DraftQuantities>({});
  const [deliveryDate, setDeliveryDate] = useState('');
  const [notes, setNotes] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [created, setCreated] = useState<CustomerOrder | null>(null);

  const today = meta?.now.slice(0, 10);

  // Default the delivery day to the next day on the API clock. The dispatcher's
  // planning run decides the actual operating day.
  useEffect(() => {
    if (!deliveryDate && today) setDeliveryDate(nextDeliveryDay(today));
  }, [deliveryDate, today]);

  const catalogue = items.data ?? [];
  const draft = useMemo(
    () => draftSubmission(catalogue, quantities),
    [catalogue, quantities],
  );

  const chilled = catalogue.filter(
    (item) =>
      item.temperatureRequirement === 'CHILLED' ||
      item.temperatureRequirement === 'FROZEN',
  );
  const ambient = catalogue.filter(
    (item) => item.temperatureRequirement === 'AMBIENT',
  );

  const setQuantity = (itemId: string, raw: string) => {
    const value = Number.parseInt(raw, 10);
    setQuantitySafe(itemId, Number.isFinite(value) && value > 0 ? value : 0);
  };
  const setQuantitySafe = (itemId: string, value: number) => {
    setSubmitError(null);
    setQuantities((prev) => ({ ...prev, [itemId]: value }));
  };

  const canSubmit =
    Boolean(outlet) &&
    Boolean(deliveryDate) &&
    !submitting &&
    draft.error === null;

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!outlet || draft.error) return;
    setSubmitting(true);
    setSubmitError(null);
    try {
      const order = await api.createOrder({
        outletId: outlet.outletId,
        requestedDeliveryDate: deliveryDate,
        items: [...draft.items],
        ...(notes.trim() ? { notes: notes.trim() } : {}),
      });
      setCreated(order);
    } catch (error) {
      setSubmitError(readableStoreError(error, 'This order'));
    } finally {
      setSubmitting(false);
    }
  }

  if (created) {
    return (
      <OrderPlaced
        order={created}
        onReset={() => {
          setCreated(null);
          setQuantities({});
          setNotes('');
          setDeliveryDate(today ? nextDeliveryDay(today) : '');
        }}
      />
    );
  }

  if (outletLoading && !outlet) {
    return <LoadingState label="Loading your outlet…" />;
  }
  if (!outlet) {
    return (
      <ErrorState
        title="Couldn't load your outlet"
        message="Your outlet could not be resolved from your account. Sign in again."
      />
    );
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-4 pb-32">
      <header className="flex flex-col gap-1 px-1 pt-2">
        <Eyebrow>Place an order</Eyebrow>
        <h1 className="text-xl font-semibold text-ink">
          Order for <Mono>{outlet.outletId}</Mono>
        </h1>
        <StoreDayLine />
      </header>

      <Card as="section" className="gap-3">
        <div className="grid gap-4 sm:grid-cols-2">
          <label className="flex flex-col gap-1 text-sm">
            <span className="font-medium text-ink">Delivery day</span>
            <input
              type="date"
              value={deliveryDate}
              min={today}
              onChange={(event) => setDeliveryDate(event.target.value)}
              className="h-11 rounded-control bg-page px-3 font-mono text-sm text-ink ring-1 ring-ink/15 focus-visible:ring-2 focus-visible:ring-brand"
            />
            {deliveryDate && (
              <span className="text-xs text-ink-muted">
                {formatDay(deliveryDate)}
              </span>
            )}
          </label>
          <label className="flex flex-col gap-1 text-sm">
            <span className="font-medium text-ink">Note (optional)</span>
            <input
              type="text"
              value={notes}
              maxLength={280}
              onChange={(event) => setNotes(event.target.value)}
              placeholder="Anything the dispatcher should know"
              className="h-11 rounded-control bg-page px-3 text-sm text-ink ring-1 ring-ink/15 focus-visible:ring-2 focus-visible:ring-brand"
            />
          </label>
        </div>
      </Card>

      {items.loading && !items.data ? (
        <LoadingState label="Loading the catalogue…" />
      ) : items.error && !items.data ? (
        <ErrorState
          title="Couldn't load the catalogue"
          message={readableStoreError(items.error, 'The catalogue')}
          onRetry={items.reload}
        />
      ) : catalogue.length === 0 ? (
        <Card>
          <p className="text-sm text-ink-muted">
            No SKUs are available for your outlet’s brand yet.
          </p>
        </Card>
      ) : (
        <>
          {chilled.length > 0 && (
            <ItemGroup
              title="Chilled"
              hint="Travels on a refrigerated vehicle."
              items={chilled}
              quantities={quantities}
              onChange={setQuantity}
            />
          )}
          {ambient.length > 0 && (
            <ItemGroup
              title="Dry"
              hint="Ambient goods, no refrigeration."
              items={ambient}
              quantities={quantities}
              onChange={setQuantity}
            />
          )}
        </>
      )}

      {draft.error &&
        (draft.items.length > 0 || (items.data?.length ?? 0) > 0) && (
          <p
            role="alert"
            className="rounded-card border border-error bg-error-bg p-3 text-sm text-error-strong"
          >
            {draft.error}
          </p>
        )}

      {/* Sticky summary + submit, so the running totals and the action stay in
          view while the store scrolls the catalogue. */}
      <div className="fixed inset-x-0 bottom-0 z-10 border-t border-line bg-card/95 backdrop-blur">
        <div className="mx-auto flex w-full max-w-4xl flex-wrap items-center gap-x-4 gap-y-2 p-3">
          <dl className="flex flex-1 flex-wrap gap-x-5 gap-y-1">
            <Fact label="Units">
              <Mono>{draft.units}</Mono>
            </Fact>
            <Fact label="Weight">
              <Mono>{formatKg(draft.weightKg)}</Mono>
            </Fact>
            <Fact label="Volume">
              <Mono>{formatM3(draft.volumeM3)}</Mono>
            </Fact>
            <Fact label="Temperature">
              <TempChip temp={draft.temperature} />
            </Fact>
          </dl>
          <button
            type="submit"
            disabled={!canSubmit}
            className={buttonClass('ink')}
          >
            {submitting ? 'Submitting…' : 'Submit order'}
          </button>
        </div>
      </div>

      {submitError && (
        <p
          role="alert"
          className="fixed inset-x-0 bottom-24 z-20 mx-auto max-w-4xl rounded-card border border-error bg-error-bg p-3 text-sm text-error-strong"
        >
          {submitError}
        </p>
      )}
    </form>
  );
}

function ItemGroup({
  title,
  hint,
  items,
  quantities,
  onChange,
}: {
  title: string;
  hint: string;
  items: readonly Item[];
  quantities: DraftQuantities;
  onChange: (itemId: string, raw: string) => void;
}) {
  return (
    <section className="flex flex-col gap-3">
      <div className="flex flex-col gap-0.5 px-1">
        <h2 className="text-sm font-semibold text-ink">{title}</h2>
        <p className="text-xs text-ink-muted">{hint}</p>
      </div>
      <Card className="gap-0 p-0">
        <ul className="flex flex-col divide-y divide-line">
          {items.map((item) => (
            <li
              key={item.itemId}
              className="flex flex-wrap items-center justify-between gap-3 p-4"
            >
              <div className="min-w-0">
                <p className="text-sm font-medium text-ink">{item.name}</p>
                <p className="text-xs text-ink-muted">
                  <Mono>{item.sku}</Mono> · unit{' '}
                  <Mono>{formatKg(item.unitWeightKg)}</Mono>
                </p>
              </div>
              <label className="flex items-center gap-2 text-sm">
                <span className="text-ink-muted">Qty</span>
                <input
                  type="number"
                  inputMode="numeric"
                  min={0}
                  step={1}
                  aria-label={`Quantity for ${item.name}`}
                  value={quantities[item.itemId] || ''}
                  onChange={(event) =>
                    onChange(item.itemId, event.target.value)
                  }
                  className="h-11 w-20 rounded-control bg-page px-2 text-right font-mono tabular-nums text-ink ring-1 ring-ink/15 focus-visible:ring-2 focus-visible:ring-brand"
                />
              </label>
            </li>
          ))}
        </ul>
      </Card>
    </section>
  );
}

/** S-03 · Submitted. The generated order number is the server's, not a guess. */
function OrderPlaced({
  order,
  onReset,
}: {
  order: CustomerOrder;
  onReset: () => void;
}) {
  return (
    <div className="flex flex-col gap-4">
      <Card as="section" className="gap-3 border-success bg-success-bg">
        <div className="flex items-center gap-2">
          <span aria-hidden="true" className="text-success">
            ✓
          </span>
          <h1 className="text-base font-semibold text-ink">Order submitted</h1>
        </div>
        <p className="text-sm text-ink">
          Your order number is{' '}
          <Mono className="font-semibold">{order.orderNumber}</Mono>. It is{' '}
          {order.status.toLowerCase()} for{' '}
          <Mono>{formatDay(order.requestedDeliveryDate)}</Mono>. Confirm it so
          the dispatcher can include it in planning — planning is the
          dispatcher’s job, not automatic.
        </p>
        {order.afterCutoff && (
          <p className="rounded-control bg-card px-3 py-2 text-xs text-warning-ink">
            It arrived after the 16:00 cutoff, so it is held for the next
            operating run.
          </p>
        )}
        <p className="text-xs text-ink-muted">
          {plural(order.totalUnits, 'unit')} · {formatKg(order.totalWeightKg)} ·{' '}
          {formatM3(order.totalVolumeM3)}
        </p>
      </Card>

      <div className="flex flex-wrap gap-2">
        <ButtonLink href={`/store/orders/${order.orderId}`} variant="ink">
          Review &amp; confirm
        </ButtonLink>
        <button
          type="button"
          onClick={onReset}
          className={buttonClass('outline')}
        >
          Place another order
        </button>
        <Link href="/store/orders" className={buttonClass('outline', 'w-auto')}>
          All my orders
        </Link>
      </div>
    </div>
  );
}
