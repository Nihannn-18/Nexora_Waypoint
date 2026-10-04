import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { WaypointApiError } from '@waypoint/api-client';
import type { Depot, Outlet } from '@waypoint/shared-types';
import { api } from '../../../../lib/api';
import { DispatcherScopeProvider } from '../../_components/dispatcher-context';
import { OutletForm } from './outlet-form';

jest.mock('../../../../lib/api', () => ({
  api: {
    meta: jest.fn(),
    getDepots: jest.fn(),
    getUnreadNotificationCount: jest.fn(),
    createOutlet: jest.fn(),
    updateOutlet: jest.fn(),
  },
}));

let push = jest.fn();
jest.mock('next/navigation', () => ({
  useRouter: () => ({ push }),
  usePathname: () => '/dispatcher/outlets/new',
}));

const mocked = api as jest.Mocked<typeof api>;

const depots: Depot[] = [
  { depotId: 'd-peli', code: 'PELIYAGODA', name: 'Peliyagoda' },
];

function renderForm(outlet?: Outlet) {
  mocked.meta.mockResolvedValue({
    now: '2026-09-25T15:40:00+05:30',
    demoMode: true,
    timezone: 'Asia/Colombo',
  });
  mocked.getDepots.mockResolvedValue(depots);
  mocked.getUnreadNotificationCount.mockResolvedValue(0);
  render(
    <DispatcherScopeProvider>
      <OutletForm depots={depots} outlet={outlet} />
    </DispatcherScopeProvider>,
  );
}

describe('Outlet form', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    push = jest.fn();
  });

  it('requires a name and district', async () => {
    renderForm();
    fireEvent.submit(document.querySelector('form')!);
    expect(await screen.findByText(/Name is required/i)).toBeTruthy();
    expect(mocked.createOutlet).not.toHaveBeenCalled();
  });

  it('reveals mall window fields only for a MALL_DOCK outlet', async () => {
    renderForm();
    expect(screen.queryByLabelText(/Mall access opens/i)).toBeNull();

    fireEvent.change(screen.getByLabelText(/Access constraint/i), {
      target: { value: 'MALL_DOCK' },
    });
    expect(screen.getByLabelText(/Mall access opens/i)).toBeTruthy();
  });

  it('creates an outlet and navigates to its detail page', async () => {
    mocked.createOutlet.mockResolvedValue({
      outletId: 'OUT121',
      name: 'Fresh New Town',
      brand: 'FRESH',
      district: 'Colombo',
      depotId: 'd-peli',
      dockType: 'STREET',
      parkingConstraint: 'NORMAL',
      mallWindow: null,
      windowOpenTime: '05:00',
      windowCloseTime: '08:00',
    });
    renderForm();
    fireEvent.change(screen.getByLabelText(/Outlet name/i), {
      target: { value: 'Fresh New Town' },
    });
    fireEvent.change(screen.getByLabelText(/^District/i), {
      target: { value: 'Colombo' },
    });

    fireEvent.submit(document.querySelector('form')!);

    await waitFor(() => expect(mocked.createOutlet).toHaveBeenCalled());
    expect(push).toHaveBeenCalledWith('/dispatcher/outlets/OUT121?saved=1');
  });

  it('shows a server error without discarding the form', async () => {
    mocked.updateOutlet.mockRejectedValue(
      new WaypointApiError('conflict', {
        status: 400,
        fieldErrors: [{ field: 'windowCloseTime', message: 'must be after windowOpenTime' }],
      }),
    );
    renderForm({
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
    });

    fireEvent.submit(document.querySelector('form')!);
    expect(
      (await screen.findAllByText(/must be after windowOpenTime/i)).length,
    ).toBeGreaterThan(0);
    expect(push).not.toHaveBeenCalled();
  });
});
