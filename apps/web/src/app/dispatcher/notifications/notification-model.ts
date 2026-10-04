import type { StatusTone } from '@waypoint/ui';
import type { AppNotification } from '@waypoint/shared-types';
import { humanize } from '../../../lib/format';

const TYPE: Record<string, { label: string; tone: StatusTone }> = {
  SHORTFALL: { label: 'Loading shortfall', tone: 'warning' },
  DELIVERY_FAILED: { label: 'Delivery failed', tone: 'error' },
  DELIVERY_DELAYED: { label: 'Delivery delayed', tone: 'warning' },
  ROUTE_ATTENTION: { label: 'Route needs attention', tone: 'info' },
  RECEIPT_ISSUE: { label: 'Receipt issue', tone: 'warning' },
};

export function notificationType(type: string): {
  label: string;
  tone: StatusTone;
} {
  return TYPE[type] ?? { label: humanize(type), tone: 'neutral' };
}

/**
 * Where a notification leads. The reference names the originating event:
 * `shortfall:<routeId>` opens that route; a delivery event has no dispatcher
 * view of its own, so it opens the outlet's orders instead.
 */
export function notificationLink(
  n: AppNotification,
): { href: string; label: string } | null {
  const [kind, id] = (n.reference ?? '').split(':', 2);
  if (kind === 'shortfall' && id) {
    return { href: `/dispatcher/tracker/${id}`, label: 'Open the route' };
  }
  if (n.outletId) {
    return {
      href: `/dispatcher/queue?q=${encodeURIComponent(n.outletId)}&days=all`,
      label: `Open ${n.outletId}’s orders`,
    };
  }
  return null;
}
