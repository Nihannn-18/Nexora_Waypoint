'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import type { ReactNode } from 'react';
import type { DeliveryOutcome } from '@waypoint/shared-types';
import { useOutbox } from '../_lib/use-outbox';
import type { OutboxEvent } from '../_lib/outbox';

/**
 * Figma icons (Lucide-style, file public/icons/driver) drawn through a CSS
 * mask, so one file takes any colour via `currentColor`.
 */
export function Icon({
  name,
  size = 20,
  className = '',
}: {
  name: string;
  size?: number;
  className?: string;
}) {
  const mask = `url(/icons/driver/${name}.svg) center / contain no-repeat`;
  return (
    <span
      aria-hidden="true"
      className={`inline-block shrink-0 bg-current ${className}`}
      style={{ width: size, height: size, mask, WebkitMask: mask }}
    />
  );
}

export const time = (iso?: string) =>
  iso
    ? new Date(iso).toLocaleTimeString('en-GB', {
        hour: '2-digit',
        minute: '2-digit',
      })
    : '';

export const OUTCOME_LABEL: Record<DeliveryOutcome, string> = {
  DELIVERED: 'Delivered',
  FAILED: 'Failed',
  DELAYED: 'Delayed',
};

/** "OUT019 · Delivered + signature + photo" */
export function eventTitle(e: OutboxEvent): string {
  const extras = [e.signature && 'signature', e.photo && 'photo'].filter(
    Boolean,
  );
  return `${e.outletId ?? 'Stop'} · ${OUTCOME_LABEL[e.outcome]}${
    extras.length ? ` + ${extras.join(' + ')}` : ''
  }`;
}

/* --- Shell (Mobile App Bar · Sync Status Bar · Mobile Tab Bar) ------------ */

export function DriverShell({ children }: { children: ReactNode }) {
  const { online, pending, syncing, events } = useOutbox();
  const path = usePathname();
  const rejected = events.filter((e) => e.status === 'REJECTED').length;

  // Offline is a neutral operating state, never an error (DG-B): use the
  // neutral `bg-offline` token, not `bg-error`.
  const sync = !online
    ? { dot: 'bg-offline', text: 'Offline · saving on this phone' }
    : syncing || pending > 0
      ? { dot: 'bg-warning', text: 'Syncing…' }
      : { dot: 'bg-live', text: 'Live Sync ON' };

  return (
    <div className="flex h-dvh flex-col bg-mobile">
      <header className="flex h-16 shrink-0 items-center justify-between px-4 py-3">
        <div className="flex flex-col gap-1 text-[11px] leading-[1.3]">
          <p className="font-semibold text-black">DRIVER TASKS</p>
          <p
            className="flex items-center gap-1.5 font-medium text-ink-muted"
            role="status"
          >
            <span className={`size-2 rounded-pill ${sync.dot}`} />
            {sync.text}
          </p>
        </div>
        <Link
          href="/signin"
          aria-label="Account"
          className="flex size-11 items-center justify-center"
        >
          <span className="flex rounded-control bg-black p-2 text-white">
            <Icon name="user" size={16} />
          </span>
        </Link>
      </header>

      {!online ? (
        <Bar tone="dark" icon="wifi-off">
          No signal · {pending} {pending === 1 ? 'action' : 'actions'} saved on
          this phone
        </Bar>
      ) : rejected > 0 ? (
        <Bar tone="error" icon="ban">
          {rejected} {rejected === 1 ? 'action needs' : 'actions need'}{' '}
          attention
        </Bar>
      ) : path === '/driver/log' && pending === 0 && events.length > 0 ? (
        <Bar tone="success" icon="circle-check">
          All actions uploaded · {time(lastSyncedAt(events))}
        </Bar>
      ) : null}

      <main className="flex min-h-0 flex-1 flex-col overflow-y-auto">
        {children}
      </main>

      <nav
        aria-label="Driver"
        className="relative flex h-16 shrink-0 border-t border-line bg-page"
      >
        <Tab label="Deliveries" icon="tab-deliveries" />
        <Tab
          label="Cockpit"
          icon="tab-cockpit"
          href="/driver"
          active={path !== '/driver/log'}
        />
        <Tab label="Vehicle" icon="tab-vehicle" />
        <Tab
          label="Log"
          icon="tab-log"
          href="/driver/log"
          active={path === '/driver/log'}
        />
        {pending > 0 && (
          <span className="absolute top-1.5 left-[calc(87.5%+8px)] rounded-pill bg-error px-1.5 py-px text-[10px] font-semibold leading-[1.3] text-white">
            {pending}
          </span>
        )}
      </nav>
    </div>
  );
}

function lastSyncedAt(events: readonly OutboxEvent[]) {
  return events
    .map((e) => e.syncedAt ?? '')
    .sort()
    .at(-1);
}

function Bar({
  tone,
  icon,
  children,
}: {
  tone: 'dark' | 'success' | 'error';
  icon: string;
  children: ReactNode;
}) {
  const style = {
    dark: 'bg-ink text-white',
    success: 'bg-success-bg text-success',
    error: 'bg-error-bg text-error-strong',
  }[tone];
  return (
    <div
      role="status"
      className={`flex shrink-0 items-center gap-2.5 px-4 py-2.5 text-xs font-semibold leading-[1.3] ${style}`}
    >
      <Icon name={icon} size={18} />
      <p className="flex-1">{children}</p>
      {tone !== 'success' && (
        <Link href="/driver/log" className="text-brand underline">
          View log
        </Link>
      )}
    </div>
  );
}

function Tab({
  label,
  icon,
  href,
  active = false,
}: {
  label: string;
  icon: string;
  href?: string;
  active?: boolean;
}) {
  const cls = `flex flex-1 flex-col items-center justify-center gap-1 text-[11px] leading-[1.3] ${
    active
      ? 'border-t-2 border-black font-bold text-black'
      : 'font-semibold text-ink-muted'
  }`;
  const body = (
    <>
      <Icon name={icon} />
      {label}
    </>
  );
  // ponytail: Deliveries and Vehicle have no Figma screen or API yet — shown, not linked.
  return href ? (
    <Link
      href={href}
      className={cls}
      aria-current={active ? 'page' : undefined}
    >
      {body}
    </Link>
  ) : (
    <span className={`${cls} opacity-60`} aria-disabled="true">
      {body}
    </span>
  );
}

/* --- Building blocks ----------------------------------------------------- */

export function Card({
  children,
  className = '',
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <section
      className={`flex flex-col gap-2.5 rounded-tile border border-line bg-white p-4 ${className}`}
    >
      {children}
    </section>
  );
}

export function Eyebrow({
  children,
  className = 'text-ink-muted',
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <p
      className={`text-[10px] font-semibold uppercase leading-[1.3] tracking-[0.4px] ${className}`}
    >
      {children}
    </p>
  );
}

const BUTTON = {
  ink: 'bg-black text-white',
  success: 'bg-success text-white',
  danger: 'bg-error-bg text-error-strong',
  outline: 'border border-line-strong bg-white text-ink',
  sky: 'border border-brand bg-brand/8 text-link',
  muted: 'bg-muted text-ink-faint',
} as const;

export function buttonClass(tone: keyof typeof BUTTON) {
  return `flex h-[52px] w-full items-center justify-center gap-2 rounded-tile px-5 text-base font-bold leading-[1.3] disabled:cursor-not-allowed disabled:bg-muted disabled:text-ink-faint ${BUTTON[tone]}`;
}

export function Alert({
  tone,
  eyebrow,
  title,
  children,
}: {
  tone: 'info' | 'success';
  eyebrow: string;
  title: string;
  children: ReactNode;
}) {
  const t =
    tone === 'info'
      ? {
          box: 'border-link bg-info-bg',
          ink: 'text-link',
          chip: 'bg-brand/8',
          icon: 'info',
        }
      : {
          box: 'border-success bg-success-bg',
          ink: 'text-success',
          chip: 'bg-success-bg',
          icon: 'circle-check',
        };
  return (
    <div className={`flex gap-3 rounded-tile border-l-4 p-4 ${t.box}`}>
      <span className={`self-start rounded-chip p-1.5 ${t.chip} ${t.ink}`}>
        <Icon name={t.icon} size={16} />
      </span>
      <div className="flex flex-1 flex-col gap-1 leading-[1.3]">
        <p
          className={`text-[11px] font-semibold uppercase tracking-[0.44px] ${t.ink}`}
        >
          {eyebrow}
        </p>
        <p className="text-base font-semibold text-ink">{title}</p>
        <p className="font-mono text-xs text-ink">{children}</p>
      </div>
    </div>
  );
}

export function SyncBadge({ status }: { status: OutboxEvent['status'] }) {
  const s = {
    PENDING: [
      'bg-warning-bg text-warning-ink',
      'bg-warning-ink',
      'Pending sync',
    ],
    SYNCED: ['bg-success-bg text-success', 'bg-success', 'Synced'],
    REJECTED: [
      'bg-error-bg text-error-strong',
      'bg-error-strong',
      'Not accepted',
    ],
  }[status];
  return (
    <span
      className={`flex shrink-0 items-center gap-1.5 rounded-pill px-2.5 py-1 text-[11px] font-semibold leading-[1.3] ${s[0]}`}
    >
      <span className={`size-1.5 rounded-pill ${s[1]}`} />
      {s[2]}
    </span>
  );
}

/** Page body: 20px gutters, 12px rhythm, as every Figma Driver frame. */
export function Body({ children }: { children: ReactNode }) {
  return (
    <div className="flex flex-1 flex-col gap-3 px-5 pt-3 pb-4">{children}</div>
  );
}

export function PageTitle({
  title,
  subtitle,
  back,
}: {
  title: string;
  subtitle?: string;
  back?: string;
}) {
  return (
    <div className="flex items-center gap-1">
      {back && (
        <Link
          href={back}
          aria-label="Back"
          className="flex size-11 shrink-0 items-center justify-center rounded-control text-ink"
        >
          <Icon name="back" size={22} />
        </Link>
      )}
      <div className="flex min-w-0 flex-col gap-0.5 leading-[1.3]">
        <h1 className="text-lg font-bold text-ink">{title}</h1>
        {subtitle && (
          <p className="truncate text-xs text-ink-muted">{subtitle}</p>
        )}
      </div>
    </div>
  );
}

/** Image types the media API accepts (media.isImageContentType). */
const POD_TYPES = 'image/jpeg,image/png,image/webp,image/heic';

/** Native camera capture — no camera library; the OS camera is better. */
export function PhotoInput({
  photo,
  onChange,
  label,
}: {
  photo: File | null;
  onChange: (f: File | null) => void;
  label: string;
}) {
  return (
    <label className="flex cursor-pointer items-center gap-2.5 rounded-tile border border-line-strong bg-white px-3.5 py-3 text-xs font-semibold leading-[1.3] text-ink focus-within:ring-2 focus-within:ring-brand">
      <Icon name="camera" />
      <span className="flex-1">
        {photo ? `${label} attached` : `Add ${label.toLowerCase()}`}
      </span>
      {photo && <Icon name="circle-check" size={18} className="text-success" />}
      <input
        type="file"
        accept={POD_TYPES}
        capture="environment"
        className="sr-only"
        onChange={(e) => onChange(e.target.files?.[0] ?? null)}
      />
    </label>
  );
}
