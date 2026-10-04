import Link from 'next/link';
import type { ComponentType, ReactNode, SVGProps } from 'react';
import { Mono, StatusBadge, type StatusTone } from '@waypoint/ui';
import {
  CONSTRAINT_CATALOG,
  type Brand,
  type ConstraintCode,
  type TempRequirement,
} from '@waypoint/shared-types';
import { ArrowRightIcon } from '../../../components/icons';
import { humanize, percentOf } from '../../../lib/format';

/** Dispatcher-only presentation pieces. Cross-role primitives live in @waypoint/ui. */

export function PageHeader({
  screenId,
  title,
  description,
  actions,
}: {
  screenId?: string;
  title: string;
  description?: ReactNode;
  actions?: ReactNode;
}) {
  return (
    <header className="mb-5 flex flex-wrap items-end justify-between gap-3">
      <div className="min-w-0">
        <div className="flex items-baseline gap-2">
          {screenId && (
            <Mono className="rounded-chip bg-brand/10 px-1.5 py-0.5 text-xs text-link">
              {screenId}
            </Mono>
          )}
          <h1 className="text-xl font-semibold tracking-tight text-ink">
            {title}
          </h1>
        </div>
        {description && (
          <p className="mt-1 max-w-3xl text-sm text-ink-muted">{description}</p>
        )}
      </div>
      {actions && (
        <div className="flex flex-wrap items-center gap-2">{actions}</div>
      )}
    </header>
  );
}

/** Mono section heading with a rule, as in the Day 5 dispatcher frames. */
export function SectionHeading({
  children,
  aside,
}: {
  children: ReactNode;
  aside?: ReactNode;
}) {
  return (
    <div className="mb-3 flex items-end justify-between gap-3 border-b border-ink/15 pb-1.5">
      <h2 className="font-mono text-base tracking-wide text-ink/80">
        {children}
      </h2>
      {aside}
    </div>
  );
}

export function Card({
  children,
  className = '',
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <section
      className={`rounded-card bg-card p-4 ring-1 ring-ink/10 ${className}`}
    >
      {children}
    </section>
  );
}

/**
 * A KPI card that links to the screen that resolves it (D-01). The value is
 * always a real number from the API, or a dash while loading.
 */
export function MetricCard({
  label,
  value,
  unit,
  hint,
  hintTone = 'muted',
  href,
  linkLabel,
  icon: Icon,
}: {
  label: string;
  value: ReactNode;
  unit?: string;
  hint?: ReactNode;
  hintTone?: 'muted' | 'warning' | 'error' | 'success';
  href: string;
  linkLabel: string;
  icon: ComponentType<SVGProps<SVGSVGElement>>;
}) {
  const hintClass = {
    muted: 'text-ink-muted',
    warning: 'text-warning',
    error: 'text-error',
    success: 'text-success',
  }[hintTone];
  return (
    <Link
      href={href}
      className="group flex flex-col rounded-card bg-brand/10 p-4 ring-1 ring-brand/15 transition hover:ring-brand/50"
    >
      <span className="flex items-center gap-2 font-mono text-sm text-ink/80">
        <Icon width={20} height={20} className="text-ink-muted" />
        {label}
      </span>
      <span className="mt-3 flex items-baseline gap-2">
        <span className="text-3xl font-medium tracking-tight text-ink tabular-nums">
          {value}
        </span>
        {unit && (
          <span className="text-sm font-light text-ink-muted">{unit}</span>
        )}
      </span>
      {hint && <span className={`mt-1 text-xs ${hintClass}`}>{hint}</span>}
      <span className="mt-auto flex items-center justify-end gap-1 pt-3 font-mono text-[0.6875rem] text-link group-hover:underline">
        {linkLabel} <ArrowRightIcon width={12} height={12} />
      </span>
    </Link>
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
      className={`inline-flex items-center rounded-chip px-1.5 py-0.5 text-[0.6875rem] font-semibold uppercase tracking-wide ring-1 ring-inset ${BRAND_CLASS[brand]}`}
    >
      {brand}
    </span>
  );
}

/** Chilled and frozen both need a reefer; the dataset only uses chilled. */
export function TempChip({ temp }: { temp: TempRequirement }) {
  if (temp === 'AMBIENT') {
    return (
      <span className="inline-flex items-center rounded-chip bg-page px-1.5 py-0.5 text-[0.6875rem] font-semibold uppercase tracking-wide text-ink-muted ring-1 ring-inset ring-ink/15">
        Ambient
      </span>
    );
  }
  return (
    <span className="inline-flex items-center gap-1 rounded-chip bg-brand/10 px-1.5 py-0.5 text-[0.6875rem] font-semibold uppercase tracking-wide text-link ring-1 ring-inset ring-link/25">
      <span aria-hidden="true">❄</span> Chilled · reefer
    </span>
  );
}

/** The binding rule with its Day 5 rule-panel id, e.g. "E-02 · Refrigeration". */
export function ConstraintChip({ code }: { code: ConstraintCode | string }) {
  const def = CONSTRAINT_CATALOG[code as ConstraintCode];
  return (
    <span className="inline-flex items-center gap-1.5 rounded-chip bg-warning/10 px-1.5 py-0.5 text-xs font-medium text-ink ring-1 ring-inset ring-warning/30">
      <Mono className="font-semibold text-warning">
        {def?.designRuleId ?? 'E-?'}
      </Mono>
      {def?.label ?? humanize(code)}
    </span>
  );
}

/**
 * A resource meter. The number and percentage are always printed, so the bar
 * colour is never the only signal; over the limit adds the words "over limit".
 */
export function Meter({
  label,
  value,
  limit,
  format,
  note,
}: {
  label: string;
  value: number;
  limit: number | undefined;
  format: (n: number) => string;
  note?: string;
}) {
  const pct = limit ? percentOf(value, limit) : undefined;
  const over = pct !== undefined && pct > 100;
  const high = pct !== undefined && pct >= 90;
  const bar = over ? 'bg-error' : high ? 'bg-warning' : 'bg-action';
  return (
    <div className="min-w-0">
      <div className="flex items-baseline justify-between gap-2 text-xs">
        <span className="text-ink-muted">{label}</span>
        <Mono className={over ? 'font-semibold text-error' : 'text-ink'}>
          {format(value)}
          {limit !== undefined && <> / {format(limit)}</>}
          {pct !== undefined && <> ({pct}%)</>}
        </Mono>
      </div>
      <div
        className="mt-1 h-1.5 overflow-hidden rounded-pill bg-ink/10"
        role="meter"
        aria-label={label}
        aria-valuemin={0}
        aria-valuemax={limit ?? value}
        aria-valuenow={value}
      >
        <div
          className={`h-full rounded-pill ${bar}`}
          style={{ width: `${Math.min(pct ?? 0, 100)}%` }}
        />
      </div>
      {(over || note) && (
        <p
          className={`mt-0.5 text-[0.6875rem] ${over ? 'text-error' : 'text-ink-muted'}`}
        >
          {over ? 'Over limit' : note}
        </p>
      )}
    </div>
  );
}

const ROUTE_TONE: Record<string, StatusTone> = {
  DRAFT: 'neutral',
  CONFIRMED: 'info',
  DISPATCHED: 'info',
  LOADING: 'info',
  LOADED: 'info',
  IN_TRANSIT: 'info',
  COMPLETED: 'success',
  CANCELLED: 'error',
};

export function RouteStatusBadge({ status }: { status: string }) {
  return (
    <StatusBadge tone={ROUTE_TONE[status] ?? 'neutral'}>
      {humanize(status)}
    </StatusBadge>
  );
}

const LEG_TONE: Record<string, StatusTone> = {
  PENDING: 'neutral',
  EN_ROUTE: 'info',
  ARRIVED: 'info',
  DELIVERED: 'success',
  FAILED: 'error',
  SKIPPED: 'warning',
};

export function LegStatusBadge({ status }: { status: string }) {
  return (
    <StatusBadge tone={LEG_TONE[status] ?? 'neutral'}>
      {humanize(status)}
    </StatusBadge>
  );
}

export function ButtonLink({
  href,
  children,
  variant = 'secondary',
}: {
  href: string;
  children: ReactNode;
  variant?: 'primary' | 'secondary';
}) {
  return (
    <Link
      href={href}
      className={
        variant === 'primary'
          ? 'tap-target inline-flex items-center justify-center gap-2 rounded-control bg-action px-4 text-sm font-semibold text-card hover:bg-action/90'
          : 'tap-target inline-flex items-center justify-center gap-2 rounded-control bg-card px-4 text-sm font-medium text-ink ring-1 ring-ink/15 hover:bg-page'
      }
    >
      {children}
    </Link>
  );
}

export const buttonClass = {
  primary:
    'tap-target inline-flex items-center justify-center gap-2 rounded-control bg-action px-4 text-sm font-semibold text-card hover:bg-action/90 disabled:cursor-not-allowed disabled:bg-ink/25',
  secondary:
    'tap-target inline-flex items-center justify-center gap-2 rounded-control bg-card px-4 text-sm font-medium text-ink ring-1 ring-ink/15 hover:bg-page disabled:cursor-not-allowed disabled:text-ink-muted',
} as const;

/** Key/value pair for detail panels. */
export function Fact({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  return (
    <div className="min-w-0">
      <dt className="text-[0.6875rem] font-semibold uppercase tracking-wide text-ink-muted">
        {label}
      </dt>
      <dd className="mt-0.5 text-sm text-ink">{children}</dd>
    </div>
  );
}
