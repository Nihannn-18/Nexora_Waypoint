import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { WaypointApiError } from '@waypoint/api-client';
import { StoreShell } from '../_components/store-shell';
import PlaceOrderPage from './page';
import { makeItem, makeOrder, demoMeta, ownOutlet } from '../_lib/fixtures';
import { api } from '../../../lib/api';

jest.mock('next/navigation', () => ({ usePathname: () => '/store/order' }));
jest.mock('../../../lib/api', () => ({
  api: {
    getOutlets: jest.fn(),
    meta: jest.fn(),
    listItems: jest.fn(),
    createOrder: jest.fn(),
  },
  tokenStore: { get: () => null, set: jest.fn(), clear: jest.fn() },
}));

const mocked = api as unknown as {
  getOutlets: jest.Mock;
  meta: jest.Mock;
  listItems: jest.Mock;
  createOrder: jest.Mock;
};

const ambient = makeItem({ itemId: 'it-ambient', name: 'Demo ambient' });
const chilled = makeItem({
  itemId: 'it-chilled',
  sku: 'DEMO-FRESH-CHILLED',
  name: 'Demo chilled',
  temperatureRequirement: 'CHILLED',
});

const renderStore = (ui: ReactNode) => render(<StoreShell>{ui}</StoreShell>);

beforeEach(() => {
  jest.clearAllMocks();
  mocked.getOutlets.mockResolvedValue([ownOutlet]);
  mocked.meta.mockResolvedValue(demoMeta);
  mocked.listItems.mockResolvedValue([ambient, chilled]);
  mocked.createOrder.mockResolvedValue(makeOrder());
});

describe('S-02 place order', () => {
  it('orders only for the authenticated outlet — there is no outlet picker', async () => {
    renderStore(<PlaceOrderPage />);

    expect(await screen.findByText(/Order for/)).toBeTruthy();
    expect(screen.queryByRole('combobox', { name: /outlet/i })).toBeNull();
    // The catalogue is requested for the outlet's own brand.
    await waitFor(() =>
      expect(mocked.listItems).toHaveBeenCalledWith({ brand: 'FRESH' }),
    );
  });

  it('cannot submit until at least one quantity is entered', async () => {
    renderStore(<PlaceOrderPage />);
    const submit = await screen.findByRole('button', { name: /submit order/i });
    const quantity = await screen.findByLabelText('Quantity for Demo ambient');

    expect((submit as HTMLButtonElement).disabled).toBe(true);
    // An untouched form is not an error state.
    expect(screen.queryByRole('alert')).toBeNull();
    fireEvent.change(quantity, { target: { value: '3' } });
    expect((submit as HTMLButtonElement).disabled).toBe(false);
  });

  // The Fresh design rule: chilled and dry are separate orders.
  it('blocks a draft that mixes chilled and dry lines', async () => {
    renderStore(<PlaceOrderPage />);
    await screen.findByLabelText('Quantity for Demo ambient');

    fireEvent.change(screen.getByLabelText('Quantity for Demo ambient'), {
      target: { value: '1' },
    });
    fireEvent.change(screen.getByLabelText('Quantity for Demo chilled'), {
      target: { value: '1' },
    });

    expect(await screen.findByText(/separate orders/i)).toBeTruthy();
    expect(
      (
        screen.getByRole('button', {
          name: /submit order/i,
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
    expect(mocked.createOrder).not.toHaveBeenCalled();
  });

  it('submits the order server-side and shows the generated order number', async () => {
    renderStore(<PlaceOrderPage />);
    await screen.findByLabelText('Quantity for Demo ambient');

    fireEvent.change(screen.getByLabelText('Quantity for Demo ambient'), {
      target: { value: '3' },
    });
    fireEvent.click(screen.getByRole('button', { name: /submit order/i }));

    await waitFor(() =>
      expect(mocked.createOrder).toHaveBeenCalledWith({
        outletId: 'OUT014',
        requestedDeliveryDate: '2026-09-26',
        items: [{ itemId: 'it-ambient', quantity: 3 }],
      }),
    );
    expect(await screen.findByText(/order submitted/i)).toBeTruthy();
    expect(screen.getByText('ORD-2026-000001')).toBeTruthy();
  });

  it('shows a readable error when the server rejects the order', async () => {
    mocked.createOrder.mockRejectedValue(
      new WaypointApiError('invalid', {
        status: 400,
        fieldErrors: [
          { field: 'lines.itemId', message: 'that SKU is not for this brand' },
        ],
      }),
    );
    renderStore(<PlaceOrderPage />);
    await screen.findByLabelText('Quantity for Demo ambient');

    fireEvent.change(screen.getByLabelText('Quantity for Demo ambient'), {
      target: { value: '1' },
    });
    fireEvent.click(screen.getByRole('button', { name: /submit order/i }));

    expect(
      await screen.findByText(/that SKU is not for this brand/i),
    ).toBeTruthy();
    // The draft is not thrown away, so the store can correct and resubmit.
    expect(
      (
        screen.getByRole('button', {
          name: /submit order/i,
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(false);
  });

  it('shows a catalogue error with a retry', async () => {
    mocked.listItems.mockRejectedValue(
      new WaypointApiError('down', { status: 500 }),
    );
    renderStore(<PlaceOrderPage />);

    expect(
      await screen.findByText(/couldn't load the catalogue/i),
    ).toBeTruthy();
    expect(screen.getByRole('button', { name: /try again/i })).toBeTruthy();
  });
});
