import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
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
    confirmOrder: jest.fn(),
  },
  tokenStore: { get: () => null, set: jest.fn(), clear: jest.fn() },
}));

const mocked = api as unknown as {
  getOutlets: jest.Mock;
  meta: jest.Mock;
  getOrder: jest.Mock;
  confirmOrder: jest.Mock;
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

  it('explains a deferred order with the recorded reason when available', async () => {
    mocked.getOrder.mockResolvedValue(
      makeOrder({
        status: 'DEFERRED',
        deferral: {
          reasonText: 'Refrigerated capacity ran out for this district.',
          constraintCode: 'FRESH_TIME_BUDGET',
          decidedAt: '2026-09-25T16:20:00+05:30',
          deferredToDate: '2026-09-28',
        },
      }),
    );
    await renderPage();

    expect(await screen.findByText(/this order was deferred/i)).toBeTruthy();
    expect(
      screen.getByText(/refrigerated capacity ran out/i),
    ).toBeTruthy();
    expect(screen.getByText('FRESH_TIME_BUDGET')).toBeTruthy();
  });

  it('does not invent a reason when the deferral record is absent', async () => {
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

  it('offers an explicit confirm action for a placed order', async () => {
    await renderPage();

    const confirm = await screen.findByRole('button', {
      name: /confirm order/i,
    });
    expect(confirm).toBeTruthy();
    expect(mocked.confirmOrder).not.toHaveBeenCalled();
  });

  it('confirms the order only on explicit click and reports success', async () => {
    mocked.confirmOrder.mockResolvedValue(makeOrder({ status: 'CONFIRMED' }));
    await renderPage();

    fireEvent.click(
      await screen.findByRole('button', { name: /confirm order/i }),
    );

    await waitFor(() =>
      expect(mocked.confirmOrder).toHaveBeenCalledWith('o1'),
    );
    expect(await screen.findByText(/order confirmed/i)).toBeTruthy();
  });

  it('disables the confirm action while the request is in flight', async () => {
    mocked.confirmOrder.mockReturnValue(new Promise(() => undefined));
    await renderPage();

    fireEvent.click(
      await screen.findByRole('button', { name: /confirm order/i }),
    );

    const pending = await screen.findByRole('button', { name: /confirming/i });
    expect((pending as HTMLButtonElement).disabled).toBe(true);
  });

  it('leads a delivered order to its goods received note', async () => {
    mocked.getOrder.mockResolvedValue(makeOrder({ status: 'DELIVERED' }));
    await renderPage();

    const link = await screen.findByRole('link', { name: /open the grn/i });
    expect(link.getAttribute('href')).toBe('/store/orders/o1/receipt');
  });

  it('lets a received order reopen its recorded GRN', async () => {
    mocked.getOrder.mockResolvedValue(makeOrder({ status: 'RECEIVED' }));
    await renderPage();

    const link = await screen.findByRole('link', { name: /view the grn/i });
    expect(link.getAttribute('href')).toBe('/store/orders/o1/receipt');
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
