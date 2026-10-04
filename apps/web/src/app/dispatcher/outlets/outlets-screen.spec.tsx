import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { WaypointApiError } from '@waypoint/api-client';
import type { Outlet } from '@waypoint/shared-types';
import { api } from '../../../lib/api';
import { DispatcherScopeProvider } from '../_components/dispatcher-context';
import { OutletsScreen } from './outlets-screen';

jest.mock('../../../lib/api', () => ({
  api: {
    meta: jest.fn(),
    getDepots: jest.fn(),
    getUnreadNotificationCount: jest.fn(),
    getOutlets: jest.fn(),
  },
}));

jest.mock('next/navigation', () => ({
  useRouter: () => ({ push: jest.fn() }),
  usePathname: () => '/dispatcher/outlets',
}));

const mocked = api as jest.Mocked<typeof api>;

const outlet = (over: Partial<Outlet> = {}): Outlet => ({
  outletId: 'OUT014',
  name: 'Fresh Colombo 14',
  brand: 'FRESH',
  district: 'Colombo',
  depotId: 'd-peli',
  dockType: 'STREET',
  parkingConstraint: 'NORMAL',
  mallWindow: null,
  windowOpenTime: '05:00',
  windowCloseTime: '08:00',
  ...over,
});

function renderScreen() {
  mocked.meta.mockResolvedValue({
    now: '2026-09-25T15:40:00+05:30',
    demoMode: true,
    timezone: 'Asia/Colombo',
  });
  mocked.getDepots.mockResolvedValue([
    { depotId: 'd-peli', code: 'PELIYAGODA', name: 'Peliyagoda' },
  ]);
  mocked.getUnreadNotificationCount.mockResolvedValue(0);
  render(
    <DispatcherScopeProvider>
      <OutletsScreen />
    </DispatcherScopeProvider>,
  );
}

describe('Dispatcher outlets list', () => {
  beforeEach(() => jest.clearAllMocks());

  it('lists outlets with brand, district and window', async () => {
    mocked.getOutlets.mockResolvedValue([
      outlet(),
      outlet({
        outletId: 'OUT090',
        name: 'Style Mall',
        brand: 'STYLE',
        parkingConstraint: 'MALL_DOCK',
        dockType: 'MALL_BAY',
        mallWindow: { open: '10:00', close: '12:00' },
      }),
    ]);
    renderScreen();

    expect(await screen.findByText('OUT014')).toBeTruthy();
    expect(screen.getByText('OUT090')).toBeTruthy();
    expect(screen.getAllByText('Colombo').length).toBeGreaterThan(0);
    expect(screen.getAllByText(/05:00–08:00/).length).toBeGreaterThan(0);
  });

  it('searches by name', async () => {
    mocked.getOutlets.mockResolvedValue([
      outlet(),
      outlet({ outletId: 'OUT090', name: 'Style Mall', brand: 'STYLE' }),
    ]);
    renderScreen();
    await screen.findByText('OUT014');

    fireEvent.change(screen.getByRole('searchbox'), {
      target: { value: 'style' },
    });
    await waitFor(() => expect(screen.queryByText('OUT014')).toBeNull());
    expect(screen.getByText('OUT090')).toBeTruthy();
  });

  it('shows an error state when the load fails', async () => {
    mocked.getOutlets.mockRejectedValue(
      new WaypointApiError('boom', { status: 500 }),
    );
    renderScreen();
    expect(await screen.findByRole('alert')).toBeTruthy();
  });
});
