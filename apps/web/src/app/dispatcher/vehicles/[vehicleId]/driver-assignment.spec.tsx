import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { WaypointApiError } from '@waypoint/api-client';
import { api } from '../../../../lib/api';
import { DispatcherScopeProvider } from '../../_components/dispatcher-context';
import { DriverAssignment } from './driver-assignment';

jest.mock('../../../../lib/api', () => ({
  api: {
    meta: jest.fn(),
    getDepots: jest.fn(),
    getUnreadNotificationCount: jest.fn(),
    listDrivers: jest.fn(),
    getVehicleAssignment: jest.fn(),
    assignDriver: jest.fn(),
    unassignDriver: jest.fn(),
  },
}));

jest.mock('next/navigation', () => ({
  useRouter: () => ({ push: jest.fn() }),
  usePathname: () => '/dispatcher/vehicles/VEH014',
}));

const mocked = api as jest.Mocked<typeof api>;

const WITH_DRIVER = {
  vehicleId: 'VEH014',
  driverId: 'u-driv',
  driverName: 'Kasun P.',
  driverEmail: 'kasun@waypoint.lk',
  date: '2026-09-26',
  depotId: 'd-peli',
};

function renderAssignment() {
  mocked.meta.mockResolvedValue({
    now: '2026-09-25T15:40:00+05:30',
    demoMode: true,
    timezone: 'Asia/Colombo',
  });
  mocked.getDepots.mockResolvedValue([
    { depotId: 'd-peli', code: 'PELIYAGODA', name: 'Peliyagoda' },
  ]);
  mocked.getUnreadNotificationCount.mockResolvedValue(0);
  mocked.listDrivers.mockResolvedValue([
    { userId: 'u-driv', name: 'Kasun P.', email: 'kasun@waypoint.lk', depotId: 'd-peli' },
    { userId: 'u-driv2', name: 'Amal S.', email: 'amal@waypoint.lk', depotId: 'd-peli' },
  ]);
  render(
    <DispatcherScopeProvider>
      <DriverAssignment vehicleId="VEH014" depotId="d-peli" />
    </DispatcherScopeProvider>,
  );
  // Set the operating date explicitly so the test does not depend on the
  // shared scope's asynchronous default; the server reads exactly this date.
  fireEvent.change(screen.getByLabelText(/Operating date/i), {
    target: { value: '2026-09-26' },
  });
}

describe('DriverAssignment', () => {
  beforeEach(() => jest.clearAllMocks());

  it('shows the currently assigned driver', async () => {
    mocked.getVehicleAssignment.mockResolvedValue(WITH_DRIVER);
    renderAssignment();

    expect(await screen.findByText(/is assigned for/i)).toBeTruthy();
  });

  it('assigns the chosen driver for the delivery date', async () => {
    mocked.getVehicleAssignment.mockResolvedValue(null);
    mocked.assignDriver.mockResolvedValue({
      ...WITH_DRIVER,
      driverId: 'u-driv2',
      driverName: 'Amal S.',
      driverEmail: 'amal@waypoint.lk',
    });
    renderAssignment();
    await screen.findByText(/No driver is assigned/i);

    fireEvent.change(screen.getByLabelText(/Assigned driver/i), {
      target: { value: 'u-driv2' },
    });
    fireEvent.click(screen.getByRole('button', { name: /Assign driver/i }));

    await waitFor(() =>
      expect(mocked.assignDriver).toHaveBeenCalledWith('VEH014', {
        driverId: 'u-driv2',
        date: '2026-09-26',
      }),
    );
  });

  it('removes an assignment after confirmation', async () => {
    mocked.getVehicleAssignment.mockResolvedValue(WITH_DRIVER);
    mocked.unassignDriver.mockResolvedValue(WITH_DRIVER);
    renderAssignment();
    await screen.findByText(/is assigned for/i);

    fireEvent.click(screen.getByRole('button', { name: /Remove assignment/i }));
    fireEvent.click(screen.getByRole('button', { name: /Confirm removal/i }));

    await waitFor(() =>
      expect(mocked.unassignDriver).toHaveBeenCalledWith('VEH014', '2026-09-26'),
    );
  });

  it('explains a same-day conflict instead of silently retrying', async () => {
    mocked.getVehicleAssignment.mockResolvedValue(null);
    mocked.assignDriver.mockRejectedValue(
      new WaypointApiError('conflict', { status: 409 }),
    );
    renderAssignment();
    await screen.findByText(/No driver is assigned/i);

    fireEvent.change(screen.getByLabelText(/Assigned driver/i), {
      target: { value: 'u-driv2' },
    });
    fireEvent.click(screen.getByRole('button', { name: /Assign driver/i }));

    expect(await screen.findByRole('alert')).toBeTruthy();
  });
});
