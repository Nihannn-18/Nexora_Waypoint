import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { Suspense } from 'react';
import type { LoadingLine, RouteLoading } from '@waypoint/api-client';
import PickingListPage from './page';
import { api } from '../../../../lib/api';

jest.mock('../../../../lib/api', () => ({
  api: {
    getRouteLoading: jest.fn(),
    recordShortfall: jest.fn(),
    createShortfallUpload: jest.fn(),
  },
}));

const mockApi = api as unknown as {
  getRouteLoading: jest.Mock;
  recordShortfall: jest.Mock;
};

const line = (over: Partial<LoadingLine> = {}): LoadingLine => ({
  orderItemId: 'OI1',
  orderId: 'O1',
  itemId: 'IT1',
  sku: 'WF-MLK-01',
  name: 'Fresh Milk 1L',
  orderedQty: 10,
  loadedQty: 0,
  damagedQty: 0,
  missingQty: 0,
  shortfallQty: 0,
  seq: 1,
  outletId: 'OUT001',
  outletName: 'Waypoint Fresh One',
  orderNumber: 'S1-001',
  dockType: 'REAR_DOCK',
  tempRequirement: 'CHILLED',
  weightKg: 12,
  volumeM3: 0.3,
  ...over,
});

const route = (lines: LoadingLine[]): RouteLoading => ({
  routeId: 'R1',
  vehicleId: 'VEH014',
  depotId: 'd-peli',
  routeDate: '2026-09-26',
  tripNo: 1,
  brand: 'FRESH',
  district: 'Colombo',
  status: 'CONFIRMED',
  routeReady: false,
  lines,
  vehicleType: 'TRUCK',
  vehicleTemp: 'REEFER',
  weightCapKg: 6500,
  volumeCapM3: 22,
  loadedWeightKg: 0,
  loadedVolumeM3: 0,
  stops: lines.length,
});

/**
 * The route params arrive as a promise in Next 16, so the page suspends on its
 * first render; the Suspense boundary plus an awaited act is what a real
 * navigation gives it.
 */
const renderPage = async () => {
  await act(async () => {
    render(
      <Suspense fallback={null}>
        <PickingListPage params={Promise.resolve({ routeId: 'R1' })} />
      </Suspense>,
    );
  });
  await waitFor(() =>
    expect(screen.queryByText(/loading the picking list/i)).toBeNull(),
  );
};

beforeEach(() => jest.clearAllMocks());

describe('L-02 picking list', () => {
  // The behaviour the design is built around: the LAST stop appears first,
  // because it is loaded first and travels deepest in the vehicle.
  it('lists stops in reverse delivery order', async () => {
    mockApi.getRouteLoading.mockResolvedValue(
      route([
        line({ orderItemId: 'a', seq: 1, outletId: 'OUT001' }),
        line({ orderItemId: 'b', seq: 2, outletId: 'OUT002' }),
        line({ orderItemId: 'c', seq: 3, outletId: 'OUT003' }),
      ]),
    );
    await renderPage();

    const headings = screen
      .getAllByText(/^OUT00\d$/)
      .map((el) => el.textContent);
    expect(headings).toEqual(['OUT003', 'OUT002', 'OUT001']);
    expect(screen.getByText(/deepest/i)).toBeTruthy();
  });

  it('explains the load order on the screen', async () => {
    mockApi.getRouteLoading.mockResolvedValue(route([line()]));
    await renderPage();

    expect(screen.getByText(/last drop goes in first/i)).toBeTruthy();
  });

  it('cannot save until something has changed', async () => {
    mockApi.getRouteLoading.mockResolvedValue(route([line()]));
    await renderPage();

    expect(
      screen.getByRole<HTMLButtonElement>('button', { name: /nothing to save/i })
        .disabled,
    ).toBe(true);
  });

  it('enables saving once a line reconciles', async () => {
    mockApi.getRouteLoading.mockResolvedValue(route([line()]));
    await renderPage();

    fireEvent.click(screen.getByRole('button', { name: /load all/i }));

    expect(
      screen.getByRole<HTMLButtonElement>('button', { name: /save 1 line/i })
        .disabled,
    ).toBe(false);
  });

  it('sends the counted quantities and shows the server verdict', async () => {
    mockApi.getRouteLoading.mockResolvedValue(route([line()]));
    mockApi.recordShortfall.mockResolvedValue({
      ...route([line({ loadedQty: 10 })]),
      routeReady: true,
    });
    await renderPage();

    fireEvent.click(screen.getByRole('button', { name: /load all/i }));
    fireEvent.click(screen.getByRole('button', { name: /save 1 line/i }));

    await waitFor(() =>
      expect(mockApi.recordShortfall).toHaveBeenCalledWith('R1', {
        items: [
          {
            orderItemId: 'OI1',
            loadedQty: 10,
            damagedQty: 0,
            missingQty: 0,
          },
        ],
      }),
    );
    expect(await screen.findByText(/ready to leave/i)).toBeTruthy();
  });

  // Readiness is the server's word, never the screen's: a client that decided
  // for itself could tell a loader to seal a truck the server will not dispatch.
  it('does not claim the route is ready when the server says otherwise', async () => {
    mockApi.getRouteLoading.mockResolvedValue(route([line()]));
    mockApi.recordShortfall.mockResolvedValue({
      ...route([line({ loadedQty: 10 })]),
      routeReady: false,
    });
    await renderPage();

    fireEvent.click(screen.getByRole('button', { name: /load all/i }));
    fireEvent.click(screen.getByRole('button', { name: /save 1 line/i }));

    expect(await screen.findByText(/still outstanding/i)).toBeTruthy();
    expect(screen.queryByText(/ready to leave/i)).toBeNull();
  });

  it('keeps the counts and explains a rejected save', async () => {
    mockApi.getRouteLoading.mockResolvedValue(route([line()]));
    mockApi.recordShortfall.mockRejectedValue(
      Object.assign(new Error('nope'), {
        name: 'WaypointApiError',
        status: 409,
      }),
    );
    await renderPage();

    fireEvent.click(screen.getByRole('button', { name: /load all/i }));
    fireEvent.click(screen.getByRole('button', { name: /save 1 line/i }));

    expect(await screen.findByText(/couldn't save your counts/i)).toBeTruthy();
    // The work is not thrown away: the line is still counted and resendable.
    expect(
      screen.getByRole<HTMLButtonElement>('button', { name: /save 1 line/i })
        .disabled,
    ).toBe(false);
  });

  it('shows a readable message when the route cannot be opened', async () => {
    mockApi.getRouteLoading.mockRejectedValue(new Error('boom'));
    await renderPage();

    expect(await screen.findByText(/couldn't open this route/i)).toBeTruthy();
  });
});

describe('L-03 flag sheet', () => {
  it('records damaged and missing separately and derives what was loaded', async () => {
    mockApi.getRouteLoading.mockResolvedValue(route([line()]));
    mockApi.recordShortfall.mockResolvedValue(route([line({ loadedQty: 8 })]));
    await renderPage();

    fireEvent.click(screen.getByRole('button', { name: /flag short/i }));
    fireEvent.change(screen.getByLabelText('Damaged'), {
      target: { value: '1' },
    });
    fireEvent.change(screen.getByLabelText('Missing'), {
      target: { value: '1' },
    });
    fireEvent.click(screen.getByRole('button', { name: /apply/i }));
    fireEvent.click(screen.getByRole('button', { name: /save 1 line/i }));

    await waitFor(() =>
      expect(mockApi.recordShortfall).toHaveBeenCalledWith('R1', {
        items: [
          {
            orderItemId: 'OI1',
            loadedQty: 8,
            damagedQty: 1,
            missingQty: 1,
          },
        ],
      }),
    );
  });

  // Over-counting is stopped as it is typed rather than on submit: the loader
  // cannot enter more damaged + missing than the line ordered, so the invariant
  // the server enforces can never be broken from this sheet.
  it('will not let more than the ordered quantity be counted', async () => {
    mockApi.getRouteLoading.mockResolvedValue(route([line({ orderedQty: 2 })]));
    await renderPage();

    fireEvent.click(screen.getByRole('button', { name: /flag short/i }));
    const damaged = screen.getByLabelText<HTMLInputElement>('Damaged');
    const missing = screen.getByLabelText<HTMLInputElement>('Missing');

    fireEvent.change(damaged, { target: { value: '2' } });
    fireEvent.change(missing, { target: { value: '2' } });

    expect(damaged.value).toBe('2');
    expect(missing.value).toBe('0');
    // Nothing loaded, nothing over-counted, so the sheet still applies cleanly.
    expect(
      screen.getByRole<HTMLButtonElement>('button', { name: /apply/i }).disabled,
    ).toBe(false);
  });
});
