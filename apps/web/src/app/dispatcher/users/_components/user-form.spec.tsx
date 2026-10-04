import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { WaypointApiError } from '@waypoint/api-client';
import type { Depot, ManagedUser, Outlet } from '@waypoint/shared-types';
import { api } from '../../../../lib/api';
import { UserForm } from './user-form';

jest.mock('../../../../lib/api', () => ({
  api: { createUser: jest.fn(), updateUser: jest.fn() },
}));

const push = jest.fn();
jest.mock('next/navigation', () => ({
  useRouter: () => ({ push }),
}));

const mocked = api as jest.Mocked<typeof api>;

const depots: Depot[] = [
  { depotId: 'd-peli', code: 'PELIYAGODA', name: 'Peliyagoda' },
  { depotId: 'd-kandy', code: 'KANDY', name: 'Kandy' },
];

const outlets: Outlet[] = [
  {
    outletId: 'OUT014',
    name: 'Outlet OUT014',
    brand: 'FRESH',
    district: 'Colombo',
    depotId: 'd-peli',
    dockType: 'REAR_DOCK',
    parkingConstraint: 'NORMAL',
    mallWindow: null,
    windowOpenTime: '08:00',
    windowCloseTime: '12:00',
  },
];

describe('User form (create)', () => {
  beforeEach(() => jest.clearAllMocks());

  it('shows a depot for a driver and no outlet field', async () => {
    render(<UserForm depots={depots} outlets={outlets} />);
    expect(screen.getByLabelText(/^Depot/i)).toBeTruthy();
    expect(screen.queryByLabelText(/^Outlet/i)).toBeNull();
  });

  it('swaps to an outlet field for a store manager', () => {
    render(<UserForm depots={depots} outlets={outlets} />);
    fireEvent.change(screen.getByLabelText(/^Role/i), {
      target: { value: 'STORE_MANAGER' },
    });
    expect(screen.getByLabelText(/^Outlet/i)).toBeTruthy();
    expect(screen.queryByLabelText(/^Depot/i)).toBeNull();
  });

  it('does not offer a dispatcher role', () => {
    render(<UserForm depots={depots} outlets={outlets} />);
    const options = screen
      .getAllByRole('option')
      .map((o) => o.textContent ?? '');
    expect(options.some((t) => /dispatcher/i.test(t))).toBe(false);
    expect(options.some((t) => /driver/i.test(t))).toBe(true);
    expect(options.some((t) => /loader/i.test(t))).toBe(true);
    expect(options.some((t) => /store manager/i.test(t))).toBe(true);
  });

  it('blocks submit and flags mismatched passwords', async () => {
    render(<UserForm depots={depots} outlets={outlets} />);
    fireEvent.change(screen.getByLabelText(/^Name/i), {
      target: { value: 'Dee' },
    });
    fireEvent.change(screen.getByLabelText(/^Email/i), {
      target: { value: 'd@waypoint.lk' },
    });
    fireEvent.change(screen.getByLabelText(/Initial password/i), {
      target: { value: 'secret-pass' },
    });
    fireEvent.change(screen.getByLabelText(/Confirm password/i), {
      target: { value: 'different' },
    });
    fireEvent.submit(document.querySelector('form')!);

    expect(await screen.findByText(/Passwords do not match/i)).toBeTruthy();
    expect(mocked.createUser).not.toHaveBeenCalled();
  });

  it('creates a driver and navigates to the detail page', async () => {
    mocked.createUser.mockResolvedValue({
      userId: 'usr_new',
      email: 'd@waypoint.lk',
      displayName: 'Dee',
      role: 'DRIVER',
      depotId: 'd-peli',
      outletId: null,
      active: true,
      createdAt: '2026-09-25T10:00:00+05:30',
    });
    render(<UserForm depots={depots} outlets={outlets} />);
    fireEvent.change(screen.getByLabelText(/^Name/i), {
      target: { value: 'Dee' },
    });
    fireEvent.change(screen.getByLabelText(/^Email/i), {
      target: { value: 'd@waypoint.lk' },
    });
    fireEvent.change(screen.getByLabelText(/Initial password/i), {
      target: { value: 'secret-pass' },
    });
    fireEvent.change(screen.getByLabelText(/Confirm password/i), {
      target: { value: 'secret-pass' },
    });
    fireEvent.submit(document.querySelector('form')!);

    await waitFor(() =>
      expect(mocked.createUser).toHaveBeenCalledWith({
        email: 'd@waypoint.lk',
        displayName: 'Dee',
        role: 'DRIVER',
        depotId: 'd-peli',
        initialPassword: 'secret-pass',
      }),
    );
    expect(push).toHaveBeenCalledWith('/dispatcher/users/usr_new?saved=1');
  });

  it('sends only an outlet for a store manager', async () => {
    mocked.createUser.mockResolvedValue({
      userId: 'usr_sm',
      email: 's@waypoint.lk',
      displayName: 'Sam',
      role: 'STORE_MANAGER',
      depotId: 'd-peli',
      outletId: 'OUT014',
      active: true,
      createdAt: '2026-09-25T10:00:00+05:30',
    });
    render(<UserForm depots={depots} outlets={outlets} />);
    fireEvent.change(screen.getByLabelText(/^Role/i), {
      target: { value: 'STORE_MANAGER' },
    });
    fireEvent.change(screen.getByLabelText(/^Name/i), {
      target: { value: 'Sam' },
    });
    fireEvent.change(screen.getByLabelText(/^Email/i), {
      target: { value: 's@waypoint.lk' },
    });
    fireEvent.change(screen.getByLabelText(/^Outlet/i), {
      target: { value: 'OUT014' },
    });
    fireEvent.change(screen.getByLabelText(/Initial password/i), {
      target: { value: 'secret-pass' },
    });
    fireEvent.change(screen.getByLabelText(/Confirm password/i), {
      target: { value: 'secret-pass' },
    });
    fireEvent.submit(document.querySelector('form')!);

    await waitFor(() =>
      expect(mocked.createUser).toHaveBeenCalledWith({
        email: 's@waypoint.lk',
        displayName: 'Sam',
        role: 'STORE_MANAGER',
        outletId: 'OUT014',
        initialPassword: 'secret-pass',
      }),
    );
  });

  it('shows a duplicate-email conflict without losing the form', async () => {
    mocked.createUser.mockRejectedValue(
      new WaypointApiError('conflict', { status: 409 }),
    );
    render(<UserForm depots={depots} outlets={outlets} />);
    fireEvent.change(screen.getByLabelText(/^Name/i), {
      target: { value: 'Dee' },
    });
    fireEvent.change(screen.getByLabelText(/^Email/i), {
      target: { value: 'dup@waypoint.lk' },
    });
    fireEvent.change(screen.getByLabelText(/Initial password/i), {
      target: { value: 'secret-pass' },
    });
    fireEvent.change(screen.getByLabelText(/Confirm password/i), {
      target: { value: 'secret-pass' },
    });
    fireEvent.submit(document.querySelector('form')!);

    expect(
      await screen.findByText(/already exists/i),
    ).toBeTruthy();
    expect(push).not.toHaveBeenCalled();
  });

  it('toggles password visibility only for the field being entered', () => {
    render(<UserForm depots={depots} outlets={outlets} />);
    const password = screen.getByLabelText(/Initial password/i) as HTMLInputElement;
    expect(password.type).toBe('password');
    fireEvent.click(screen.getByRole('button', { name: /^Show$/i }));
    expect((screen.getByLabelText(/Initial password/i) as HTMLInputElement).type).toBe('text');
  });
});

describe('User form (edit)', () => {
  beforeEach(() => jest.clearAllMocks());

  const existing: ManagedUser = {
    userId: 'usr_driver',
    email: 'kasun.p@waypoint.lk',
    displayName: 'Kasun P.',
    role: 'DRIVER',
    depotId: 'd-peli',
    outletId: null,
    active: true,
    createdAt: '2026-09-25T10:00:00+05:30',
  };

  it('does not offer password fields when editing', () => {
    render(<UserForm user={existing} depots={depots} outlets={outlets} />);
    expect(screen.queryByLabelText(/Initial password/i)).toBeNull();
    expect(screen.queryByLabelText(/Confirm password/i)).toBeNull();
  });

  it('saves the permitted profile fields', async () => {
    mocked.updateUser.mockResolvedValue({ ...existing, displayName: 'Kasun Perera' });
    render(<UserForm user={existing} depots={depots} outlets={outlets} />);
    fireEvent.change(screen.getByLabelText(/^Name/i), {
      target: { value: 'Kasun Perera' },
    });
    fireEvent.submit(document.querySelector('form')!);

    await waitFor(() =>
      expect(mocked.updateUser).toHaveBeenCalledWith('usr_driver', {
        displayName: 'Kasun Perera',
        depotId: 'd-peli',
      }),
    );
  });
});
