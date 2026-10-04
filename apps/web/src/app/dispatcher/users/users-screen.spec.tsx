import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { WaypointApiError } from '@waypoint/api-client';
import type { ManagedUser } from '@waypoint/shared-types';
import { api } from '../../../lib/api';
import { UsersScreen } from './users-screen';

jest.mock('../../../lib/api', () => ({
  api: { listUsers: jest.fn() },
}));

const mocked = api as jest.Mocked<typeof api>;

const user = (over: Partial<ManagedUser> = {}): ManagedUser => ({
  userId: 'usr_driver',
  email: 'kasun.p@waypoint.lk',
  displayName: 'Kasun P.',
  role: 'DRIVER',
  depotId: 'd-peli',
  outletId: null,
  active: true,
  createdAt: '2026-09-25T10:00:00+05:30',
  ...over,
});

describe('Dispatcher users list', () => {
  beforeEach(() => jest.clearAllMocks());

  it('lists accounts with role and status but no credential', async () => {
    mocked.listUsers.mockResolvedValue([
      user(),
      user({
        userId: 'usr_store',
        email: 'ishara.s@waypoint.lk',
        displayName: 'Ishara S.',
        role: 'STORE_MANAGER',
        depotId: 'd-peli',
        outletId: 'OUT014',
        active: false,
      }),
    ]);
    render(<UsersScreen />);

    expect(await screen.findByText('Kasun P.')).toBeTruthy();
    expect(screen.getByText('ishara.s@waypoint.lk')).toBeTruthy();
    // The outlet assignment is shown for a store manager.
    expect(screen.getByText('OUT014')).toBeTruthy();
    // No password or hash ever appears.
    expect(screen.queryByText(/password/i)).toBeNull();
    expect(screen.queryByText(/argon2|\$argon/i)).toBeNull();
  });

  it('filters by role', async () => {
    mocked.listUsers.mockResolvedValue([
      user(),
      user({
        userId: 'usr_store',
        email: 'ishara.s@waypoint.lk',
        displayName: 'Ishara S.',
        role: 'STORE_MANAGER',
        outletId: 'OUT014',
      }),
      user({
        userId: 'usr_loader',
        email: 'nadeesha.p@waypoint.lk',
        displayName: 'Nadeesha P.',
        role: 'LOADER',
      }),
    ]);
    render(<UsersScreen />);
    await screen.findByText('Kasun P.');

    fireEvent.click(screen.getByRole('button', { name: /^Loaders$/i }));
    await waitFor(() => expect(screen.queryByText('Kasun P.')).toBeNull());
    expect(screen.getByText('Nadeesha P.')).toBeTruthy();
  });

  it('filters by active state', async () => {
    mocked.listUsers.mockResolvedValue([
      user(),
      user({
        userId: 'usr_store',
        email: 'ishara.s@waypoint.lk',
        displayName: 'Ishara S.',
        role: 'STORE_MANAGER',
        outletId: 'OUT014',
        active: false,
      }),
    ]);
    render(<UsersScreen />);
    await screen.findByText('Kasun P.');

    fireEvent.change(screen.getByLabelText(/Filter by status/i), {
      target: { value: 'INACTIVE' },
    });
    await waitFor(() => expect(screen.queryByText('Kasun P.')).toBeNull());
    expect(screen.getByText('Ishara S.')).toBeTruthy();
  });

  it('searches by name or email', async () => {
    mocked.listUsers.mockResolvedValue([
      user(),
      user({
        userId: 'usr_store',
        email: 'ishara.s@waypoint.lk',
        displayName: 'Ishara S.',
        role: 'STORE_MANAGER',
        outletId: 'OUT014',
      }),
    ]);
    render(<UsersScreen />);
    await screen.findByText('Kasun P.');

    fireEvent.change(screen.getByRole('searchbox'), {
      target: { value: 'ishara' },
    });
    await waitFor(() => expect(screen.queryByText('Kasun P.')).toBeNull());
    expect(screen.getByText('Ishara S.')).toBeTruthy();
  });

  it('shows an error state when the load fails', async () => {
    mocked.listUsers.mockRejectedValue(
      new WaypointApiError('boom', { status: 500 }),
    );
    render(<UsersScreen />);

    expect(await screen.findByRole('alert')).toBeTruthy();
  });

  it('shows an empty state when there are no accounts', async () => {
    mocked.listUsers.mockResolvedValue([]);
    render(<UsersScreen />);

    expect(await screen.findByText(/No accounts match/i)).toBeTruthy();
  });
});
