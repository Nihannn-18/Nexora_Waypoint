import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { WaypointApiError } from '@waypoint/api-client';
import type { Vehicle } from '@waypoint/shared-types';
import { api } from '../../../lib/api';
import { VehiclesScreen } from './vehicles-screen';

jest.mock('../../../lib/api', () => ({
  api: { getVehicles: jest.fn() },
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

describe('Dispatcher vehicles list', () => {
  beforeEach(() => jest.clearAllMocks());

  it('lists vehicles with their capability and status', async () => {
    mocked.getVehicles.mockResolvedValue([
      vehicle(),
      vehicle({ vehicleId: 'VEH020', type: 'VAN', tempClass: 'AMBIENT' }),
    ]);
    render(<VehiclesScreen />);

    expect(await screen.findByText('VEH014')).toBeTruthy();
    expect(screen.getByText('VEH020')).toBeTruthy();
    expect(screen.getByText(/Refrigerated truck/i)).toBeTruthy();
    expect(screen.getAllByText(/Available/i).length).toBeGreaterThan(0);
  });

  it('filters by refrigerated and dry-box', async () => {
    mocked.getVehicles.mockResolvedValue([
      vehicle(),
      vehicle({ vehicleId: 'VEH020', type: 'VAN', tempClass: 'AMBIENT' }),
    ]);
    render(<VehiclesScreen />);
    await screen.findByText('VEH014');

    fireEvent.click(screen.getByRole('button', { name: /Reefers/i }));
    expect(screen.getByText('VEH014')).toBeTruthy();
    expect(screen.queryByText('VEH020')).toBeNull();

    fireEvent.click(screen.getByRole('button', { name: /Dry-box/i }));
    expect(screen.getByText('VEH020')).toBeTruthy();
    expect(screen.queryByText('VEH014')).toBeNull();
  });

  it('searches by vehicle id', async () => {
    mocked.getVehicles.mockResolvedValue([
      vehicle(),
      vehicle({ vehicleId: 'VEH020', type: 'VAN', tempClass: 'AMBIENT' }),
    ]);
    render(<VehiclesScreen />);
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
    render(<VehiclesScreen />);

    expect(await screen.findByRole('alert')).toBeTruthy();
  });
});
