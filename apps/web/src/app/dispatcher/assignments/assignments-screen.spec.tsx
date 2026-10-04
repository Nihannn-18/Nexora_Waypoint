import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { WaypointApiError } from '@waypoint/api-client';
import { api } from '../../../lib/api';
import { AssignmentsScreen } from './assignments-screen';

jest.mock('../../../lib/api', () => ({
  api: {
    getVehicles: jest.fn(),
    listDrivers: jest.fn(),
    listVehicleAssignments: jest.fn(),
    assignDriver: jest.fn(),
    unassignDriver: jest.fn(),
    listStoreManagers: jest.fn(),
    getOutlets: jest.fn(),
    assignManager: jest.fn(),
    unassignManager: jest.fn(),
    listLoaders: jest.fn(),
    getDepots: jest.fn(),
    assignLoader: jest.fn(),
    unassignLoader: jest.fn(),
  },
}));

jest.mock('../_components/dispatcher-context', () => ({
  useDispatcherScope: () => ({
    deliveryDate: '2026-09-26',
    depot: { depotId: 'd-peli', name: 'Peliyagoda' },
    depots: [{ depotId: 'd-peli', name: 'Peliyagoda' }],
  }),
}));

const mocked = api as jest.Mocked<typeof api>;

beforeEach(() => {
  jest.clearAllMocks();
  mocked.getVehicles.mockResolvedValue([
    { vehicleId: 'VEH014', type: 'TRUCK', tempClass: 'REEFER', weightCapKg: 2500, volumeCapM3: 20, fuelType: 'diesel', kmPerL: 8, weeklyFuelQuotaL: 200, depotId: 'd-peli', status: 'AVAILABLE' },
  ]);
  mocked.listDrivers.mockResolvedValue([
    { userId: 'u-driv', name: 'Kasun P.', email: 'kasun@waypoint.lk', depotId: 'd-peli' },
  ]);
  mocked.listVehicleAssignments.mockResolvedValue([]);
  mocked.listStoreManagers.mockResolvedValue([
    { userId: 'u-store', name: 'Ishara S.', email: 'ishara@waypoint.lk', outletId: 'OUT014', depotId: 'd-peli' },
  ]);
  mocked.getOutlets.mockResolvedValue([
    { outletId: 'OUT014', name: 'Central', brand: 'FRESH', district: 'Colombo', depotId: 'd-peli', dockType: 'REAR_DOCK', parkingConstraint: 'NORMAL', windowOpenTime: '08:00', windowCloseTime: '12:00' },
  ]);
  mocked.listLoaders.mockResolvedValue([
    { userId: 'u-load', name: 'Nadeesha P.', email: 'nadeesha@waypoint.lk', depotId: 'd-peli' },
  ]);
  mocked.getDepots.mockResolvedValue([
    { depotId: 'd-peli', code: 'PELIYAGODA', name: 'Peliyagoda', active: true },
  ]);
});

const vehicleSelect = () => document.getElementById('dv-vehicle') as HTMLSelectElement;
const driverSelect = () => document.getElementById('dv-driver') as HTMLSelectElement;

describe('Dispatcher assignments', () => {
  it('shows all three assignment workflows', async () => {
    render(<AssignmentsScreen />);
    expect(screen.getByRole('tab', { name: /Driver → Vehicle/i })).toBeTruthy();
    expect(screen.getByRole('tab', { name: /Store Manager → Outlet/i })).toBeTruthy();
    expect(screen.getByRole('tab', { name: /Loader → Depot/i })).toBeTruthy();
  });

  it('assigns a driver to a vehicle', async () => {
    mocked.assignDriver.mockResolvedValue({
      vehicleId: 'VEH014', driverId: 'u-driv', driverName: 'Kasun P.', driverEmail: 'kasun@waypoint.lk', date: '2026-09-26', depotId: 'd-peli',
    });
    render(<AssignmentsScreen />);

    await waitFor(() => expect(vehicleSelect()).toBeTruthy());
    fireEvent.change(vehicleSelect(), { target: { value: "VEH014" } });
    fireEvent.change(driverSelect(), { target: { value: "u-driv" } });
    fireEvent.click(screen.getByRole('button', { name: /Assign \/ change/i }));

    await waitFor(() =>
      expect(mocked.assignDriver).toHaveBeenCalledWith('VEH014', { driverId: 'u-driv', date: '2026-09-26' }),
    );
    expect(await screen.findByText(/Driver assigned/i)).toBeTruthy();
  });

  it('shows a conflict in plain language', async () => {
    mocked.assignDriver.mockRejectedValue(
      new WaypointApiError('conflict', { status: 409 }),
    );
    render(<AssignmentsScreen />);
    await waitFor(() => expect(vehicleSelect()).toBeTruthy());
    fireEvent.change(vehicleSelect(), { target: { value: "VEH014" } });
    fireEvent.change(driverSelect(), { target: { value: "u-driv" } });
    fireEvent.click(screen.getByRole('button', { name: /Assign \/ change/i }));
    expect(
      await screen.findByText(/already assigned for this date/i),
    ).toBeTruthy();
  });

  it('assigns a loader to a depot and refreshes', async () => {
    mocked.assignLoader.mockResolvedValue({ userId: 'u-load', name: 'Nadeesha P.', email: 'nadeesha@waypoint.lk', depotId: 'd-peli' });
    render(<AssignmentsScreen />);
    fireEvent.click(screen.getByRole('tab', { name: /Loader → Depot/i }));
    fireEvent.change(await screen.findByLabelText(/Depot for Nadeesha/i), {
      target: { value: 'd-peli' },
    });
    await waitFor(() =>
      expect(mocked.assignLoader).toHaveBeenCalledWith('u-load', { depotId: 'd-peli' }),
    );
  });

  it('reassigns a store manager outlet', async () => {
    mocked.assignManager.mockResolvedValue({ outletId: 'OUT014', userId: 'u-store', name: 'Ishara S.', email: 'ishara@waypoint.lk', depotId: 'd-peli' });
    render(<AssignmentsScreen />);
    fireEvent.click(screen.getByRole('tab', { name: /Store Manager → Outlet/i }));
    fireEvent.change(await screen.findByLabelText(/Outlet for Ishara/i), {
      target: { value: 'OUT014' },
    });
    await waitFor(() =>
      expect(mocked.assignManager).toHaveBeenCalledWith('OUT014', { userId: 'u-store' }),
    );
  });
});
