import { fireEvent, render, screen } from '@testing-library/react';
import type { ReactNode } from 'react';
import { WaypointApiError } from '@waypoint/api-client';
import { StoreShell } from '../_components/store-shell';
import MyOrdersPage from './page';
import { makeOrder, demoMeta, ownOutlet } from '../_lib/fixtures';
import { api } from '../../../lib/api';

jest.mock('next/navigation', () => ({ usePathname: () => '/store/orders' }));
jest.mock('../../../lib/api', () => ({
  api: {
    getOutlets: jest.fn(),
    meta: jest.fn(),
    listOrders: jest.fn(),
  },
  tokenStore: { get: () => null, set: jest.fn(), clear: jest.fn() },
}));

const mocked = api as unknown as {
  getOutlets: jest.Mock;
  meta: jest.Mock;
  listOrders: jest.Mock;
};

const renderStore = (ui: ReactNode) => render(<StoreShell>{ui}</StoreShell>);

beforeEach(() => {
  jest.clearAllMocks();
  mocked.getOutlets.mockResolvedValue([ownOutlet]);
  mocked.meta.mockResolvedValue(demoMeta);
  mocked.listOrders.mockResolvedValue({
    orders: [
      makeOrder({
        orderId: 'live',
        orderNumber: 'ORD-LIVE',
        status: 'CONFIRMED',
      }),
      makeOrder({
        orderId: 'done',
        orderNumber: 'ORD-DONE',
        status: 'DELIVERED',
        requestedDeliveryDate: '2026-09-24',
      }),
    ],
    total: 2,
    limit: 200,
    offset: 0,
  });
});

describe('S-07 my orders', () => {
  it('opens on the live orders and hides the completed tail', async () => {
    renderStore(<MyOrdersPage />);

    expect(await screen.findByText('ORD-LIVE')).toBeTruthy();
    expect(screen.queryByText('ORD-DONE')).toBeNull();
  });

  it('shows delivered orders on the completed tab', async () => {
    renderStore(<MyOrdersPage />);
    await screen.findByText('ORD-LIVE');

    fireEvent.click(screen.getByRole('button', { name: /completed/i }));

    expect(screen.getByText('ORD-DONE')).toBeTruthy();
    expect(screen.queryByText('ORD-LIVE')).toBeNull();
  });

  describe('deferred view (reached from the Store home "Deferred" tile)', () => {
    beforeEach(() => {
      mocked.listOrders.mockResolvedValue({
        orders: [
          makeOrder({
            orderId: 'live',
            orderNumber: 'ORD-LIVE',
            status: 'CONFIRMED',
          }),
          makeOrder({
            orderId: 'def',
            orderNumber: 'ORD-DEF',
            status: 'DEFERRED',
            requestedDeliveryDate: '2026-09-26',
          }),
        ],
        total: 2,
        limit: 200,
        offset: 0,
      });
    });
    afterEach(() => window.history.replaceState(null, '', '/'));

    it('opens on the deferred orders when the URL asks for them', async () => {
      window.history.replaceState(null, '', '/store/orders?tab=deferred');
      renderStore(<MyOrdersPage />);

      expect(await screen.findByText('ORD-DEF')).toBeTruthy();
      expect(screen.queryByText('ORD-LIVE')).toBeNull();
      expect(
        screen.getByRole('heading', { name: 'Deferred orders' }),
      ).toBeTruthy();
    });

    it('lists deferred orders on their own tab, not under completed', async () => {
      renderStore(<MyOrdersPage />);
      await screen.findByText('ORD-LIVE');
      expect(screen.queryByText('ORD-DEF')).toBeNull();

      fireEvent.click(screen.getByRole('button', { name: /deferred/i }));
      expect(screen.getByText('ORD-DEF')).toBeTruthy();
      expect(window.location.search).toBe('?tab=deferred');

      fireEvent.click(screen.getByRole('button', { name: /completed/i }));
      expect(screen.queryByText('ORD-DEF')).toBeNull();
    });
  });

  it('filters by order number in the search box', async () => {
    renderStore(<MyOrdersPage />);
    await screen.findByText('ORD-LIVE');

    fireEvent.change(screen.getByRole('searchbox'), {
      target: { value: 'nope' },
    });

    expect(screen.getByText(/no order matches that number/i)).toBeTruthy();
  });

  it('explains an empty live list', async () => {
    mocked.listOrders.mockResolvedValue({
      orders: [],
      total: 0,
      limit: 200,
      offset: 0,
    });
    renderStore(<MyOrdersPage />);

    expect(await screen.findByText(/no live orders/i)).toBeTruthy();
  });

  it('shows a retryable error when the list fails', async () => {
    mocked.listOrders.mockRejectedValue(
      new WaypointApiError('offline', { status: 0, isOffline: true }),
    );
    renderStore(<MyOrdersPage />);

    expect(await screen.findByText(/couldn't load your orders/i)).toBeTruthy();
    expect(screen.getByRole('button', { name: /try again/i })).toBeTruthy();
  });
});
