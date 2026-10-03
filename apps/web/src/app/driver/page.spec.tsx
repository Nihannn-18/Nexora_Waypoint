import { render, screen, waitFor } from '@testing-library/react';
import type { DriverRoute } from '@waypoint/api-client';
import DriverCockpitPage from './page';
import { getSelectedRoute } from './_lib/outbox';

const stop = (legId: string, outletId: string, name: string) => ({
  legId,
  seq: 0,
  outletId,
  outletName: name,
  windowOpen: '05:00',
  windowClose: '08:00',
  status: 'PENDING',
});

const routes: DriverRoute[] = [
  {
    routeId: 'r-veh001',
    routeDate: '2026-09-26',
    vehicleId: 'VEH001',
    tripNo: 1,
    brand: 'FRESH',
    district: 'Colombo',
    status: 'DISPATCHED',
    stops: [stop('leg-001', 'OUT001', 'Other depot run')],
  },
  {
    routeId: 'r-veh014',
    routeDate: '2026-09-26',
    vehicleId: 'VEH014',
    tripNo: 1,
    brand: 'FRESH',
    district: 'Colombo',
    status: 'DISPATCHED',
    stops: [stop('leg-014', 'OUT014', 'Fresh Colombo 14')],
  },
];

jest.mock('./_lib/use-outbox', () => ({
  useOutbox: () => ({
    online: true,
    events: [],
    pending: 0,
    syncing: false,
    sync: jest.fn(),
  }),
  useRoutes: () => ({
    status: 'ready',
    date: '2026-09-26',
    routes,
    cached: false,
  }),
  useLeg: () => ({ status: 'loading' }),
  prefetchLegs: jest.fn(),
}));

jest.mock('./_lib/outbox', () => ({
  getSelectedRoute: jest.fn(),
  setSelectedRoute: jest.fn(),
  clearSelectedRoute: jest.fn(),
}));

const mockedGetSelectedRoute = getSelectedRoute as jest.Mock;

beforeEach(() => jest.clearAllMocks());

describe('Driver cockpit route selection (R-01)', () => {
  it('asks the driver to choose rather than silently showing the first route', async () => {
    mockedGetSelectedRoute.mockResolvedValue(undefined);
    render(<DriverCockpitPage />);

    await waitFor(() => screen.getByText('Which run is yours?'));
    // Both runs are offered, but neither is presented as the active run.
    expect(screen.getAllByText(/VEH001/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/VEH014/).length).toBeGreaterThan(0);
    expect(screen.queryByText('Target waypoint')).toBeNull();
  });

  it("shows only the remembered vehicle, never another vehicle's stops", async () => {
    mockedGetSelectedRoute.mockResolvedValue('r-veh014');
    render(<DriverCockpitPage />);

    await waitFor(() => screen.getByText('Target waypoint'));
    expect(screen.getAllByText(/OUT014/).length).toBeGreaterThan(0);
    expect(screen.queryAllByText(/OUT001/).length).toBe(0);
    expect(screen.queryByText('Which run is yours?')).toBeNull();
  });
});
