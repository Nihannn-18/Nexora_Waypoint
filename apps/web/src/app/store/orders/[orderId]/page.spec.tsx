import { act, render, screen, waitFor } from '@testing-library/react';
import { Suspense } from 'react';
import { WaypointApiError } from '@waypoint/api-client';
import { StoreShell } from '../../_components/store-shell';
import OrderDetailPage from './page';
import { makeOrder, demoMeta, ownOutlet } from '../../_lib/fixtures';
import { api } from '../../../../lib/api';

jest.mock('next/navigation', () => ({
  usePathname: () => '/store/orders/o1',
}));
jest.mock('../../../../lib/api', () => ({
  api: {
    getOutlets: jest.fn(),
    meta: jest.fn(),
    getOrder: jest.fn(),
  },
  tokenStore: { get: () => null, set: jest.fn(), clear: jest.fn() },
}));

const mocked = api as unknown as {
  getOutlets: jest.Mock;
  meta: jest.Mock;
  getOrder: jest.Mock;
};

const renderPage = async () => {
  await act(async () => {
    render(
      <StoreShell>
        <Suspense fallback={null}>
          <OrderDetailPage params={Promise.resolve({ orderId: 'o1' })} />
        </Suspense>
      </StoreShell>,
    );
  });
};

beforeEach(() => {
  jest.clearAllMocks();
  mocked.getOutlets.mockResolvedValue([ownOutlet]);
  mocked.meta.mockResolvedValue(demoMeta);
  mocked.getOrder.mockResolvedValue(makeOrder());
});

describe('S-04 order detail', () => {
  it('shows the order, its status, window and lines', async () => {
    await renderPage();

    expect(await screen.findByText('ORD-2026-000001')).toBeTruthy();
    expect(screen.getByText(/Placed/)).toBeTruthy();
    expect(screen.getAllByText(/05:00–08:00/).length).toBeGreaterThan(0);
    expect(screen.getByText('Demo ambient')).toBeTruthy();
  });

  it('calls GET /orders/{id} with the id from the route only', async () => {
    await renderPage();

    await waitFor(() => expect(mocked.getOrder).toHaveBeenCalledWith('o1'));
  });

  it('explains a deferred order without inventing a reason', async () => {
    mocked.getOrder.mockResolvedValue(makeOrder({ status: 'DEFERRED' }));
    await renderPage();

    expect(await screen.findByText(/this order was deferred/i)).toBeTruthy();
    expect(screen.getByText(/deferral log holds the reason/i)).toBeTruthy();
  });

  it('shows a loading state while the order is in flight', async () => {
    mocked.getOrder.mockReturnValue(new Promise(() => undefined));
    await renderPage();

    expect(screen.getByText(/loading this order/i)).toBeTruthy();
  });

  it('shows an error when the order is out of scope or missing', async () => {
    mocked.getOrder.mockRejectedValue(
      new WaypointApiError('not found', { status: 404 }),
    );
    await renderPage();

    expect(await screen.findByText(/couldn't open this order/i)).toBeTruthy();
    expect(screen.getByText(/belongs to another outlet/i)).toBeTruthy();
  });
});
