import type { AppNotification } from '@waypoint/shared-types';
import { notificationLink, notificationType } from './notification-model';

const base: AppNotification = {
  id: 'n1',
  type: 'SHORTFALL',
  title: 'Loading shortfall',
  message: 'Missing items',
  read: false,
  createdAt: '2026-09-26T03:40:00+05:30',
};

describe('notification links', () => {
  it('opens the route a shortfall was raised on', () => {
    expect(notificationLink({ ...base, reference: 'shortfall:R-9' })).toEqual({
      href: '/dispatcher/tracker/R-9',
      label: 'Open the route',
    });
  });

  it('opens the outlet’s orders for a delivery problem', () => {
    const link = notificationLink({
      ...base,
      type: 'DELIVERY_FAILED',
      reference: 'delivery:ev-1',
      outletId: 'OUT027',
    });
    expect(link?.href).toBe('/dispatcher/queue?q=OUT027&days=all');
  });

  it('offers no link when the reference leads nowhere', () => {
    expect(
      notificationLink({ ...base, reference: 'delivery:ev-1' }),
    ).toBeNull();
  });

  it('labels failed deliveries as errors and shortfalls as warnings', () => {
    expect(notificationType('DELIVERY_FAILED').tone).toBe('error');
    expect(notificationType('SHORTFALL')).toEqual({
      label: 'Loading shortfall',
      tone: 'warning',
    });
  });
});
