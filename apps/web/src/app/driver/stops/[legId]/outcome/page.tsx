'use client';

import { useParams, useRouter, useSearchParams } from 'next/navigation';
import { Suspense, useState } from 'react';
import {
  DELIVERY_OUTCOMES,
  type DeliveryFailureReason,
  type DeliveryOutcome,
} from '@waypoint/shared-types';
import { enqueue } from '../../../_lib/outbox';
import { useLeg } from '../../../_lib/use-outbox';
import {
  Body,
  Icon,
  OUTCOME_LABEL,
  PageTitle,
  PhotoInput,
  buttonClass,
} from '../../../_components/ui';

const OPTIONS: Record<DeliveryOutcome, { icon: string; hint: string }> = {
  DELIVERED: { icon: 'circle-check', hint: 'All items handed over — capture signature/photo next' },
  FAILED: { icon: 'circle-x', hint: 'Could not deliver — goods stay on the truck' },
  DELAYED: { icon: 'clock', hint: 'Still delivering, but later than planned' },
};

/** Figma 35:5426 reason chips. Sent as reasonCode. */
const REASONS = [
  ['OUTLET_CLOSED', 'Outlet closed'],
  ['ACCESS_BLOCKED', 'Access blocked'],
  ['REFUSED_BY_STORE', 'Refused by store'],
  ['GOODS_DAMAGED', 'Goods damaged'],
  ['OTHER', 'Other'],
] as const satisfies readonly (readonly [DeliveryFailureReason, string])[];

/** R-02 Record outcome — Figma 35:5426, validation 35:5561. */
export default function OutcomePage() {
  return (
    <Suspense>
      <RecordOutcome />
    </Suspense>
  );
}

function RecordOutcome() {
  const { legId } = useParams<{ legId: string }>();
  const initial = useSearchParams().get('o');
  const router = useRouter();
  const leg = useLeg(legId);
  const outletId = leg.status === 'ready' ? leg.leg.toOutletId : undefined;

  const [outcome, setOutcome] = useState<DeliveryOutcome>(
    DELIVERY_OUTCOMES.find((o) => o === initial) ?? 'DELIVERED',
  );
  const [reason, setReason] = useState<DeliveryFailureReason | null>(null);
  const [notes, setNotes] = useState('');
  const [photo, setPhoto] = useState<File | null>(null);
  const [tried, setTried] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const missingReason = outcome === 'FAILED' && !reason;

  async function save() {
    setTried(true);
    if (outcome === 'DELIVERED') {
      router.push(`/driver/stops/${legId}/pod`);
      return;
    }
    if (missingReason) return;
    setSaving(true);
    try {
      const e = await enqueue({
        legId,
        outletId,
        outcome,
        reasonCode: outcome === 'FAILED' ? (reason ?? undefined) : undefined,
        notes: notes.trim() || undefined,
        photo: photo ?? undefined,
      });
      router.push(`/driver/saved/${e.clientEventId}`);
    } catch {
      // IndexedDB itself failed (storage full / private mode): say so plainly.
      setError('Couldn’t save on this phone. Free some storage and try again.');
      setSaving(false);
    }
  }

  return (
    <>
      <Body>
        <PageTitle
          back={`/driver/stops/${legId}`}
          title="Record outcome"
          subtitle={outletId ? `Outlet ${outletId}` : undefined}
        />

        <fieldset className="flex flex-col gap-3">
          <legend className="mb-3 text-xs font-semibold text-ink">
            What happened at this stop?
          </legend>
          {DELIVERY_OUTCOMES.map((o) => {
            const on = o === outcome;
            return (
              <label
                key={o}
                className={`flex cursor-pointer items-center gap-3 rounded-tile px-4 py-3.5 ${
                  on
                    ? 'border-2 border-accent bg-brand/8'
                    : 'border border-line-strong bg-white'
                } focus-within:ring-2 focus-within:ring-brand`}
              >
                <span
                  className={`rounded-tile p-2 ${on ? 'bg-accent text-white' : 'bg-page text-ink'}`}
                >
                  <Icon name={OPTIONS[o].icon} />
                </span>
                <span className="flex flex-1 flex-col gap-0.5 leading-[1.3]">
                  <span className="text-base font-semibold text-ink">{OUTCOME_LABEL[o]}</span>
                  <span className="text-xs text-ink-muted">{OPTIONS[o].hint}</span>
                </span>
                <input
                  type="radio"
                  name="outcome"
                  checked={on}
                  onChange={() => setOutcome(o)}
                  className="size-5 accent-accent"
                />
              </label>
            );
          })}
        </fieldset>

        {outcome === 'FAILED' && (
          <fieldset>
            <legend
              className={`mb-3 text-xs font-semibold ${tried && missingReason ? 'text-error' : 'text-ink'}`}
            >
              Why did it fail?
            </legend>
            <div className="flex flex-wrap gap-2">
              {REASONS.map(([code, label]) => {
                const on = reason === code;
                return (
                  <button
                    key={code}
                    type="button"
                    aria-pressed={on}
                    onClick={() => setReason(code)}
                    className={`flex items-center gap-1.5 rounded-pill border px-3.5 py-2.5 text-[13px] font-semibold leading-[1.3] ${
                      on
                        ? 'border-ink bg-ink text-white'
                        : tried && missingReason
                          ? 'border-error bg-white text-ink'
                          : 'border-line-strong bg-white text-ink'
                    }`}
                  >
                    {on && <Icon name="check" size={14} />}
                    {label}
                  </button>
                );
              })}
            </div>
            {tried && missingReason && (
              <p role="alert" className="mt-3 text-xs text-error">
                Choose a reason — the store and dispatcher see it.
              </p>
            )}
          </fieldset>
        )}

        {outcome !== 'DELIVERED' && (
          <>
            <label className="flex flex-col gap-1.5">
              <span className="text-xs font-semibold text-ink">Note (optional)</span>
              <textarea
                value={notes}
                onChange={(e) => setNotes(e.target.value)}
                className="h-[88px] resize-none rounded-chip border border-line-strong bg-white px-3 py-2.5 text-sm text-ink"
              />
            </label>
            <PhotoInput photo={photo} onChange={setPhoto} label="Photo" />
          </>
        )}
      </Body>

      <div className="sticky bottom-0 flex flex-col gap-2 border-t border-line bg-white px-5 py-3">
        {error ? (
          <p role="alert" className="text-center text-xs text-error">{error}</p>
        ) : (
          <p className="text-center text-xs text-ink-muted">
            Works without signal — saved on your phone first.
          </p>
        )}
        <button type="button" onClick={save} disabled={saving} className={buttonClass('ink')}>
          <Icon name={outcome === 'DELIVERED' ? 'arrow-right' : 'check'} />
          {outcome === 'DELIVERED' ? 'Next: proof of delivery' : 'Save outcome'}
        </button>
      </div>
    </>
  );
}
