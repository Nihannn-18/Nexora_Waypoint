import type { ReactNode } from 'react';
import type { OrderStatus } from '@waypoint/shared-types';

/**
 * Status is never signalled by colour alone — the style guide requires a glyph
 * or a word alongside it, so the badge reads correctly in greyscale, on a dock
 * tablet in daylight, and for a colour-blind loader.
 */

export type StatusTone = 'neutral' | 'info' | 'success' | 'warning' | 'error';

const TONE_CLASS: Record<StatusTone, string> = {
  neutral: 'bg-page text-ink-muted ring-ink-muted/25',
  info: 'bg-brand/10 text-link ring-link/25',
  success: 'bg-success/10 text-success ring-success/30',
  warning: 'bg-warning/10 text-warning ring-warning/30',
  error: 'bg-error/10 text-error ring-error/30',
};

/** The glyph is the non-colour carrier of meaning. */
const TONE_GLYPH: Record<StatusTone, string> = {
  neutral: '•',
  info: '◆',
  success: '✓',
  warning: '!',
  error: '✕',
};

export interface StatusBadgeProps {
  tone?: StatusTone;
  children: ReactNode;
  /** Hide the glyph only where an adjacent icon already carries the meaning. */
  glyph?: boolean;
  className?: string;
}

export function StatusBadge({
  tone = 'neutral',
  children,
  glyph = true,
  className = '',
}: StatusBadgeProps) {
  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-pill px-2.5 py-1 text-xs font-medium ring-1 ring-inset ${TONE_CLASS[tone]} ${className}`}
    >
      {glyph && (
        <span aria-hidden="true" className="leading-none">
          {TONE_GLYPH[tone]}
        </span>
      )}
      {children}
    </span>
  );
}

/* -------------------------------------------------------------------------- */

const ORDER_STATUS_TONE: Record<OrderStatus, StatusTone> = {
  PLACED: 'neutral',
  CONFIRMED: 'info',
  ALLOCATED: 'info',
  LOADED: 'info',
  IN_TRANSIT: 'info',
  DELIVERED: 'success',
  RECEIVED: 'success',
  FAILED: 'error',
  DEFERRED: 'warning',
};

/** Title-cased, human-readable form of a wire status. */
export function formatOrderStatus(status: OrderStatus): string {
  return status
    .toLowerCase()
    .split('_')
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(' ');
}

export function OrderStatusBadge({
  status,
  className,
}: {
  status: OrderStatus;
  className?: string;
}) {
  return (
    <StatusBadge tone={ORDER_STATUS_TONE[status]} className={className}>
      {formatOrderStatus(status)}
    </StatusBadge>
  );
}
