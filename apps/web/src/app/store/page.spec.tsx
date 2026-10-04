import { render, screen, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { WaypointApiError } from '@waypoint/api-client';
import { StoreShell } from './_components/store-shell';
import StoreHomePage from './page';
import { makeOrder, demoMeta, ownOutlet } from './_lib/fixtures';
import { api } from '../../lib/api';

jest.mock('next/navigation', () => ({ usePathname: () => '/store' }));
jest.mock('../../lib/api', () => ({
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
    orders: [makeOrder()],
    total: 1,
    limit: 200,
    offset: 0,
  });
});

describe('S-01 store home', () => {
  it('shows the authenticated outlet and its delivery window', async () => {
    renderStore(<StoreHomePage />);

    expect(
      await screen.findByRole('heading', { name: /Hello, Outlet OUT014/ }),
    ).toBeTruthy();
    expect(screen.getAllByText(/OUT014/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/05:00–08:00/).length).toBeGreaterThan(0);
  });

  it('renders the order the server returned for this outlet', async () => {
    renderStore(<StoreHomePage />);

    expect(await screen.findByText('ORD-2026-000001')).toBeTruthy();
    expect(screen.getByText("Tomorrow's delivery")).toBeTruthy();
  });

  // The client must never widen scope: the server pins GET /orders to the
  // caller's outlet, so the screen sends no outlet or depot filter at all.
  it('lists orders with no client-supplied outlet or depot filter', async () => {
    renderStore(<StoreHomePage />);

    await waitFor(() => expect(mocked.listOrders).toHaveBeenCalled());
    expect(mocked.listOrders).toHaveBeenCalledWith({ limit: 200 });
    const firstCall = mocked.listOrders.mock.calls[0]?.[0];
    expect(firstCall).not.toHaveProperty('outletId');
    expect(firstCall).not.toHaveProperty('depotId');
  });

  it('shows a loading state while the orders are in flight', async () => {
    mocked.listOrders.mockReturnValue(new Promise(() => undefined));
    renderStore(<StoreHomePage />);

    expect(await screen.findByText(/loading your orders/i)).toBeTruthy();
  });

  it('shows an empty state when the outlet has no orders for tomorrow', async () => {
    mocked.listOrders.mockResolvedValue({
      orders: [],
      total: 0,
      limit: 200,
      offset: 0,
    });
    renderStore(<StoreHomePage />);

    expect(await screen.findByText(/no orders for tomorrow yet/i)).toBeTruthy();
  });

  it('shows an actionable error when the order list fails', async () => {
    mocked.listOrders.mockRejectedValue(
      new WaypointApiError('boom', { status: 500 }),
    );
    renderStore(<StoreHomePage />);

    expect(await screen.findByText(/couldn't load your orders/i)).toBeTruthy();
    expect(screen.getByRole('button', { name: /try again/i })).toBeTruthy();
  });

  it('calls out a deferred order rather than hiding it', async () => {
    mocked.listOrders.mockResolvedValue({
      orders: [makeOrder({ status: 'DEFERRED' })],
      total: 1,
      limit: 200,
      offset: 0,
    });
    renderStore(<StoreHomePage />);

    expect(await screen.findByText(/deferred orders/i)).toBeTruthy();
    expect(screen.getAllByText('ORD-2026-000001').length).toBeGreaterThan(0);
  });
});
