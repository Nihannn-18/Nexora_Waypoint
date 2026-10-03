'use client';

import { useParams, useRouter } from 'next/navigation';
import { useRef, useState, type PointerEvent } from 'react';
import { enqueue } from '../../../_lib/outbox';
import { useLeg } from '../../../_lib/use-outbox';
import {
  Body,
  Icon,
  PageTitle,
  PhotoInput,
  buttonClass,
} from '../../../_components/ui';

/**
 * Proof of delivery — Figma cockpit "Proof of Delivery" card (1:4485).
 * The API requires a receiver name plus a signature or photo for DELIVERED
 * (delivery.ValidateEvent); Figma shows the name as fixed text, so it is an
 * input here.
 */
export default function PodPage() {
  const { legId } = useParams<{ legId: string }>();
  const router = useRouter();
  const leg = useLeg(legId);
  const outletId = leg.status === 'ready' ? leg.leg.toOutletId : undefined;

  const canvas = useRef<HTMLCanvasElement>(null);
  const drawing = useRef(false);
  const [signed, setSigned] = useState(false);
  const [receiver, setReceiver] = useState('');
  const [photo, setPhoto] = useState<File | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  function point(e: PointerEvent<HTMLCanvasElement>) {
    const c = e.currentTarget;
    const r = c.getBoundingClientRect();
    return [((e.clientX - r.left) * c.width) / r.width, ((e.clientY - r.top) * c.height) / r.height] as const;
  }
  function down(e: PointerEvent<HTMLCanvasElement>) {
    const ctx = e.currentTarget.getContext('2d');
    if (!ctx) return;
    e.currentTarget.setPointerCapture(e.pointerId);
    drawing.current = true;
    ctx.lineWidth = 2.5;
    ctx.lineCap = 'round';
    ctx.strokeStyle = '#191c1e';
    ctx.beginPath();
    ctx.moveTo(...point(e));
  }
  function move(e: PointerEvent<HTMLCanvasElement>) {
    if (!drawing.current) return;
    const ctx = e.currentTarget.getContext('2d');
    ctx?.lineTo(...point(e));
    ctx?.stroke();
    setSigned(true);
  }
  function clear() {
    const c = canvas.current;
    c?.getContext('2d')?.clearRect(0, 0, c.width, c.height);
    setSigned(false);
  }

  async function finish() {
    if (!receiver.trim()) return setError('Enter the name of the person receiving the goods.');
    if (!signed && !photo) return setError('Add a signature or a photo as proof of delivery.');
    setError(null);
    setSaving(true);
    try {
      const signature =
        signed && canvas.current
          ? await new Promise<Blob | null>((ok) => canvas.current?.toBlob(ok, 'image/png'))
          : null;
      const e = await enqueue({
        legId,
        outletId,
        outcome: 'DELIVERED',
        receiverName: receiver.trim(),
        signature: signature ?? undefined,
        photo: photo ?? undefined,
      });
      router.push(`/driver/saved/${e.clientEventId}`);
    } catch {
      setError('Couldn’t save on this phone. Free some storage and try again.');
      setSaving(false);
    }
  }

  const proofs = Number(signed) + Number(Boolean(photo));

  return (
    <Body>
      <PageTitle
        back={`/driver/stops/${legId}/outcome?o=DELIVERED`}
        title="Proof of Delivery"
        subtitle={outletId ? `Outlet ${outletId}` : undefined}
      />

      <section className="flex flex-col gap-3.5 rounded-tile border border-line bg-white p-4">
        <label className="flex flex-col gap-1 rounded-chip border border-line bg-page p-3">
          <span className="text-[11px] font-semibold uppercase text-ink-faint">Deliver to</span>
          <input
            value={receiver}
            onChange={(e) => setReceiver(e.target.value)}
            autoComplete="name"
            placeholder="Receiver’s name"
            className="bg-transparent text-sm font-bold text-ink outline-none placeholder:font-normal placeholder:text-ink-faint"
          />
        </label>

        <div className="flex flex-col gap-1.5">
          <div className="flex items-center justify-between">
            <p id="sig-label" className="text-[11px] font-semibold tracking-[0.22px] text-ink">
              Recipient Digital Signature
            </p>
            <button type="button" onClick={clear} className="text-xs font-semibold text-error">
              Clear Pad
            </button>
          </div>
          <div className="relative h-36 rounded-chip border-2 border-dashed border-line-strong bg-page">
            {!signed && (
              <p className="pointer-events-none absolute inset-0 flex items-center justify-center text-xs font-medium text-ink-faint">
                Sign with stylus or finger within boundary
              </p>
            )}
            <canvas
              ref={canvas}
              width={640}
              height={280}
              aria-labelledby="sig-label"
              role="img"
              className="absolute inset-0 size-full touch-none"
              onPointerDown={down}
              onPointerMove={move}
              onPointerUp={() => (drawing.current = false)}
              onPointerCancel={() => (drawing.current = false)}
            />
          </div>
        </div>

        <div className="flex gap-2">
          <div className="flex-1">
            <PhotoInput photo={photo} onChange={setPhoto} label="Dock photo" />
          </div>
          <p className="flex items-center gap-1 rounded-chip border border-line-strong bg-page px-2.5 text-xs text-ink-faint">
            {proofs} Proof Attached
          </p>
        </div>

        {error && (
          <p role="alert" className="text-xs text-error">
            {error}
          </p>
        )}

        <button type="button" onClick={finish} disabled={saving} className={buttonClass('ink')}>
          <Icon name="check" />
          Finish Delivery
        </button>
        <p className="text-center text-xs text-ink-muted">
          Works without signal — saved on your phone first.
        </p>
      </section>
    </Body>
  );
}
