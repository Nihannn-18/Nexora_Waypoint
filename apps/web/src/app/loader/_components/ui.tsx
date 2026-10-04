'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import type { ReactNode } from 'react';

/**
 * Loader primitives. These live in the loader folder rather than libs/ui
 * because only this role uses them; the shared pieces (StatusBadge, Mono,
 * OfflineBanner) still come from @waypoint/ui.
 *
 * The dock is a working environment: gloves, cold hands, a tablet on a trolley.
 * Targets are at least 48px, quantities are large and tabular, and nothing is
 * signalled by colour alone — every state carries a word.
 */

export function Card({
  children,
  className = '',
  as: Tag = 'section',
}: {
  children: ReactNode;
  className?: string;
  as?: 'section' | 'article' | 'div';
}) {
  return (
    <Tag
      className={`flex flex-col gap-3 rounded-card border border-line bg-card p-4 ${className}`}
    >
      {children}
    </Tag>
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
  ink: 'bg-action text-white',
  success: 'bg-success text-white',
  danger: 'bg-error-bg text-error-strong',
  outline: 'border border-line-strong bg-card text-ink',
} as const;

/** 48px minimum, full width: the dock tablet is used standing up. */
export function buttonClass(tone: keyof typeof BUTTON, extra = '') {
  return `tap-target flex w-full items-center justify-center gap-2 rounded-control px-4 text-sm font-semibold leading-[1.3] disabled:cursor-not-allowed disabled:bg-muted disabled:text-ink-faint ${BUTTON[tone]} ${extra}`;
}

/**
 * A labelled meter. `tone` is paired with the figure beside it, never used
 * alone — a loader reading it in a cold store should not have to judge a hue.
 */
export function Meter({
  label,
  value,
  detail,
  percent,
  tone = 'normal',
}: {
  label: string;
  value: string;
  detail?: string;
  percent: number;
  tone?: 'normal' | 'warning';
}) {
  return (
    <div className="flex flex-1 flex-col gap-1.5">
      <Eyebrow>{label}</Eyebrow>
      <p className="font-mono text-xl font-semibold tabular-nums text-ink">
        {value}
      </p>
      <div
        className="h-1.5 w-full overflow-hidden rounded-pill bg-muted"
        role="img"
        aria-label={`${label}: ${percent}% of capacity`}
      >
        <div
          className={`h-full rounded-pill ${
            tone === 'warning' ? 'bg-warning' : 'bg-action'
          }`}
          style={{ width: `${percent}%` }}
        />
      </div>
      {detail && <p className="text-[11px] text-ink-muted">{detail}</p>}
    </div>
  );
}

const CHIP = {
  neutral: 'bg-muted text-ink',
  info: 'bg-info-bg text-link',
  success: 'bg-success-bg text-success',
  warning: 'bg-warning-bg text-warning-ink',
  error: 'bg-error-bg text-error-strong',
} as const;

/** A status chip. The glyph plus the word carry the meaning, not the colour. */
export function Chip({
  tone = 'neutral',
  glyph,
  children,
}: {
  tone?: keyof typeof CHIP;
  glyph?: string;
  children: ReactNode;
}) {
  return (
    <span
      className={`inline-flex items-center gap-1 rounded-pill px-2 py-0.5 text-[11px] font-semibold leading-[1.4] ${CHIP[tone]}`}
    >
      {glyph && <span aria-hidden="true">{glyph}</span>}
      {children}
    </span>
  );
}

/** Every list says why it is empty and what to do next. */
export function EmptyState({
  title,
  children,
  action,
}: {
  title: string;
  children: ReactNode;
  action?: ReactNode;
}) {
  return (
    <Card className="items-start text-sm">
      <h2 className="text-base font-semibold text-ink">{title}</h2>
      <p className="text-ink-muted">{children}</p>
      {action}
    </Card>
  );
}

export function LoadingState({ label }: { label: string }) {
  return (
    <Card>
      <p className="text-sm text-ink-muted" role="status">
        {label}
      </p>
    </Card>
  );
}

/**
 * An error the loader can act on. Raw server text is never shown: the caller
 * passes a sentence, and a retry when one makes sense.
 */
export function ErrorState({
  title,
  message,
  onRetry,
}: {
  title: string;
  message: string;
  onRetry?: () => void;
}) {
  return (
    <Card className="border-error">
      <div className="flex items-start gap-2">
        <span aria-hidden="true" className="text-error">
          ⚠
        </span>
        <div className="flex flex-col gap-1">
          <h2 className="text-sm font-semibold text-error-strong">{title}</h2>
          <p className="text-sm text-ink-muted">{message}</p>
        </div>
      </div>
      {onRetry && (
        <button type="button" onClick={onRetry} className={buttonClass('outline')}>
          Try again
        </button>
      )}
    </Card>
  );
}

const TABS = [
  { href: '/loader', label: 'Trips' },
  { href: '/loader/flags', label: 'Flags' },
  { href: '/loader/account', label: 'Account' },
] as const;

/** The bottom tab bar. Only destinations that exist are linked. */
export function LoaderTabs() {
  const path = usePathname();
  return (
    <nav
      aria-label="Loader"
      className="fixed inset-x-0 bottom-0 z-20 flex border-t border-line bg-card"
    >
      {TABS.map((tab) => {
        const active =
          tab.href === '/loader' ? path === '/loader' : path.startsWith(tab.href);
        return (
          <Link
            key={tab.href}
            href={tab.href}
            aria-current={active ? 'page' : undefined}
            className={`tap-target flex flex-1 items-center justify-center text-xs transition ${
              active
                ? 'border-t-2 border-action font-semibold text-ink'
                : 'font-medium text-ink-muted hover:text-ink'
            }`}
          >
            {tab.label}
          </Link>
        );
      })}
    </nav>
  );
}
