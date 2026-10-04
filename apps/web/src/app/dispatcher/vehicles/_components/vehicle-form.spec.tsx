import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { WaypointApiError } from '@waypoint/api-client';
import type { Depot, Vehicle } from '@waypoint/shared-types';
import { api } from '../../../../lib/api';
import { DispatcherScopeProvider } from '../../_components/dispatcher-context';
import { VehicleForm } from './vehicle-form';

jest.mock('../../../../lib/api', () => ({
  api: {
    meta: jest.fn(),
    getDepots: jest.fn(),
    getUnreadNotificationCount: jest.fn(),
    createVehicle: jest.fn(),
    updateVehicle: jest.fn(),
  },
}));

let push = jest.fn();
jest.mock('next/navigation', () => ({
  useRouter: () => ({ push }),
  usePathname: () => '/dispatcher/vehicles/new',
}));

const mocked = api as jest.Mocked<typeof api>;

const depots: Depot[] = [
  { depotId: 'd-peli', code: 'PELIYAGODA', name: 'Peliyagoda' },
];

function renderForm(vehicle?: Vehicle) {
  mocked.meta.mockResolvedValue({
    now: '2026-09-25T15:40:00+05:30',
    demoMode: true,
    timezone: 'Asia/Colombo',
  });
  mocked.getDepots.mockResolvedValue(depots);
  mocked.getUnreadNotificationCount.mockResolvedValue(0);
  render(
    <DispatcherScopeProvider>
      <VehicleForm depots={depots} vehicle={vehicle} />
    </DispatcherScopeProvider>,
  );
}

describe('Vehicle form', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    push = jest.fn();
  });

  it('blocks submit and marks fields when numeric values are invalid', async () => {
    renderForm();
    fireEvent.change(screen.getByLabelText(/Weight capacity/i), {
      target: { value: '0' },
    });
    fireEvent.change(screen.getByLabelText(/Volume capacity/i), {
      target: { value: '18' },
    });
    fireEvent.change(screen.getByLabelText(/Fuel efficiency/i), {
      target: { value: '6' },
    });
    fireEvent.change(screen.getByLabelText(/Weekly fuel quota/i), {
      target: { value: '400' },
    });

    fireEvent.submit(document.querySelector('form')!);

    expect(
      (await screen.findAllByText(/greater than zero/i)).length,
    ).toBeGreaterThan(0);
    expect(mocked.createVehicle).not.toHaveBeenCalled();
  });

  it('creates a vehicle and navigates to its detail page', async () => {
    mocked.createVehicle.mockResolvedValue({
      vehicleId: 'VEH061',
      type: 'TRUCK',
      tempClass: 'AMBIENT',
      weightCapKg: 3000,
      volumeCapM3: 18,
      fuelType: 'diesel',
      kmPerL: 6,
      weeklyFuelQuotaL: 400,
      depotId: 'd-peli',
      status: 'AVAILABLE',
    });
    renderForm();
    fireEvent.change(screen.getByLabelText(/Weight capacity/i), { target: { value: '3000' } });
    fireEvent.change(screen.getByLabelText(/Volume capacity/i), { target: { value: '18' } });
    fireEvent.change(screen.getByLabelText(/Fuel efficiency/i), { target: { value: '6' } });
    fireEvent.change(screen.getByLabelText(/Weekly fuel quota/i), { target: { value: '400' } });

    fireEvent.submit(document.querySelector('form')!);

    await waitFor(() => expect(mocked.createVehicle).toHaveBeenCalled());
    expect(push).toHaveBeenCalledWith('/dispatcher/vehicles/VEH061?saved=1');
  });

  it('shows a server field error in place without losing the form', async () => {
    mocked.updateVehicle.mockRejectedValue(
      new WaypointApiError('invalid', {
        status: 400,
        fieldErrors: [{ field: 'kmPerL', message: 'kmPerL must be greater than zero' }],
      }),
    );
    renderForm({
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
    });

    fireEvent.submit(document.querySelector('form')!);

    expect(
      (await screen.findAllByText(/kmPerL must be greater than zero/i)).length,
    ).toBeGreaterThan(0);
    expect(push).not.toHaveBeenCalled();
  });
});
