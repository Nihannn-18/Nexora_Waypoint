import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { WaypointApiError } from '@waypoint/api-client';
import type { Vehicle } from '@waypoint/shared-types';
import { api } from '../../../lib/api';
import { FleetScreen } from './fleet-screen';

jest.mock('../../../lib/api', () => ({
  api: { getVehicles: jest.fn() },
}));

jest.mock('../_components/dispatcher-context', () => ({
  useDispatcherScope: () => ({
    deliveryDate: '2026-09-26',
    depot: { depotId: 'd-peli', name: 'Peliyagoda' },
  }),
}));

const mocked = api as jest.Mocked<typeof api>;

const vehicle = (over: Partial<Vehicle> = {}): Vehicle => ({
  vehicleId: 'VEH014',
  type: 'TRUCK',
  tempClass: 'REEFER',
  weightCapKg: 2500,
  volumeCapM3: 18,
  fuelType: 'diesel',
  kmPerL: 6,
  weeklyFuelQuotaL: 400,
  depotId: 'd-peli',
  status: 'AVAILABLE',
  ...over,
});

const FLEET = [
  vehicle(),
  vehicle({ vehicleId: 'VEH020', tempClass: 'AMBIENT' }),
  vehicle({
    vehicleId: 'VEH030',
    type: 'VAN',
    tempClass: 'AMBIENT',
    status: 'IN_WORKSHOP',
  }),
];

/** D-09 is both the delivery-day availability view and the vehicle list. */
describe('Dispatcher fleet (D-09)', () => {
  beforeEach(() => jest.clearAllMocks());

  it('reads the selected depot on the delivery day', async () => {
    mocked.getVehicles.mockResolvedValue(FLEET);
    render(<FleetScreen />);
    await screen.findByText('VEH014');
    expect(mocked.getVehicles).toHaveBeenCalledWith({
      depotId: 'd-peli',
      date: '2026-09-26',
    });
  });

  it('lists vehicles with capability and availability, each opening its detail', async () => {
    mocked.getVehicles.mockResolvedValue(FLEET);
    render(<FleetScreen />);

    expect(await screen.findByText('VEH014')).toBeTruthy();
    expect(screen.getByText(/Refrigerated truck/i)).toBeTruthy();
    expect(screen.getAllByText(/Available/i).length).toBeGreaterThan(0);
    expect(
      screen.getByRole('link', { name: 'VEH020' }).getAttribute('href'),
    ).toBe('/dispatcher/vehicles/VEH020');
    expect(
      screen.getByRole('link', { name: 'Add vehicle' }).getAttribute('href'),
    ).toBe('/dispatcher/vehicles/new');
  });

  it('filters refrigerated, dry-box and vans', async () => {
    mocked.getVehicles.mockResolvedValue(FLEET);
    render(<FleetScreen />);
    await screen.findByText('VEH014');

    fireEvent.click(screen.getByRole('button', { name: /Refrigerated/i }));
    expect(screen.getByText('VEH014')).toBeTruthy();
    expect(screen.queryByText('VEH020')).toBeNull();

    fireEvent.click(screen.getByRole('button', { name: /Dry-box/i }));
    expect(screen.getByText('VEH020')).toBeTruthy();
    expect(screen.queryByText('VEH014')).toBeNull();

    fireEvent.click(screen.getByRole('button', { name: /Vans/i }));
    expect(screen.getByText('VEH030')).toBeTruthy();
    expect(screen.queryByText('VEH020')).toBeNull();
  });

  it('searches by vehicle id', async () => {
    mocked.getVehicles.mockResolvedValue(FLEET);
    render(<FleetScreen />);
    await screen.findByText('VEH014');

    fireEvent.change(screen.getByRole('searchbox'), {
      target: { value: 'veh020' },
    });
    await waitFor(() => expect(screen.queryByText('VEH014')).toBeNull());
    expect(screen.getByText('VEH020')).toBeTruthy();
  });

  it('shows an error state when the load fails', async () => {
    mocked.getVehicles.mockRejectedValue(
      new WaypointApiError('boom', { status: 500 }),
    );
    render(<FleetScreen />);
    expect(await screen.findByRole('alert')).toBeTruthy();
  });
});
