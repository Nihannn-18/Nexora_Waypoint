'use client';

import Link from 'next/link';
import type { ReactNode } from 'react';
import { type Brand, type TempRequirement } from '@waypoint/shared-types';

/**
 * Store-manager presentation pieces. Only this role uses them, so they live
 * here rather than in @waypoint/ui; the genuinely cross-role primitives (Mono,
 * OrderStatusBadge, StatusBadge, OfflineBanner) still come from there.
 *
 * A store manager works at a desk or on a phone in the shop. Card, list and
 * form controls follow the shared design tokens: white on grey, no hard-coded
 * colour, and every state carries a word as well as a tone.
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
      className={`text-[11px] font-semibold uppercase leading-[1.3] tracking-[0.4px] ${className}`}
    >
      {children}
    </p>
  );
}

export function SectionHeading({
  children,
  aside,
}: {
  children: ReactNode;
  aside?: ReactNode;
}) {
  return (
    <div className="flex items-end justify-between gap-3 border-b border-line pb-1.5">
      <h2 className="font-mono text-sm tracking-wide text-ink/80">
        {children}
      </h2>
      {aside}
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

const BRAND_CLASS: Record<Brand, string> = {
  FRESH: 'bg-fresh/10 text-fresh ring-fresh/30',
  STYLE: 'bg-style/10 text-style ring-style/30',
  TECH: 'bg-tech/10 text-tech ring-tech/30',
};

export function BrandChip({ brand }: { brand: Brand }) {
  return (
    <span
      className={`inline-flex items-center rounded-chip px-1.5 py-0.5 text-[11px] font-semibold uppercase tracking-wide ring-1 ring-inset ${BRAND_CLASS[brand]}`}
    >
      {brand}
    </span>
  );
}

/** Chilled and frozen both need a reefer; the dataset only uses chilled. */
export function TempChip({ temp }: { temp: TempRequirement }) {
  if (temp === 'AMBIENT') {
    return (
      <span className="inline-flex items-center gap-1 rounded-chip bg-page px-1.5 py-0.5 text-[11px] font-semibold uppercase tracking-wide text-ink-muted ring-1 ring-inset ring-ink/15">
        <span aria-hidden="true">▢</span> Dry
      </span>
    );
  }
  return (
    <span className="inline-flex items-center gap-1 rounded-chip bg-brand/10 px-1.5 py-0.5 text-[11px] font-semibold uppercase tracking-wide text-link ring-1 ring-inset ring-link/25">
      <span aria-hidden="true">❄</span> Chilled
    </span>
  );
}

const BUTTON = {
  ink: 'bg-action text-white',
  outline: 'border border-line-strong bg-card text-ink',
} as const;

export function buttonClass(tone: keyof typeof BUTTON, extra = ''): string {
  return `tap-target inline-flex items-center justify-center gap-2 rounded-control px-4 text-sm font-semibold leading-[1.3] disabled:cursor-not-allowed disabled:bg-muted disabled:text-ink-faint ${BUTTON[tone]} ${extra}`;
}

export function ButtonLink({
  href,
  children,
  variant = 'outline',
}: {
  href: string;
  children: ReactNode;
  variant?: 'ink' | 'outline';
}) {
  return (
    <Link href={href} className={buttonClass(variant)}>
      {children}
    </Link>
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
      <p className="text-sm text-ink-muted" role="status" aria-live="polite">
        {label}
      </p>
    </Card>
  );
}

/** An error the store manager can act on; raw server text is never shown. */
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
        <button
          type="button"
          onClick={onRetry}
          className={buttonClass('outline')}
        >
          Try again
        </button>
      )}
    </Card>
  );
}

/** Key/value pair for a detail panel. */
export function Fact({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  return (
    <div className="min-w-0">
      <dt className="text-[11px] font-semibold uppercase tracking-wide text-ink-muted">
        {label}
      </dt>
      <dd className="mt-0.5 text-sm text-ink">{children}</dd>
    </div>
  );
}
