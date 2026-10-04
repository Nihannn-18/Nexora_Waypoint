import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { api } from '../../../../lib/api';
import { ManagerAssignment } from './manager-assignment';

jest.mock('../../../../lib/api', () => ({
  api: {
    listStoreManagers: jest.fn(),
    getOutletManager: jest.fn(),
    assignManager: jest.fn(),
    unassignManager: jest.fn(),
  },
}));

const mocked = api as jest.Mocked<typeof api>;

function renderAssignment() {
  mocked.listStoreManagers.mockResolvedValue([
    { userId: 'u-store', name: 'Ishara S.', email: 'ishara@waypoint.lk', outletId: 'OUT014' },
    { userId: 'u-store2', name: 'Fathima R.', email: 'fathima@waypoint.lk', outletId: '' },
  ]);
  render(<ManagerAssignment outletId="OUT014" />);
}

describe('ManagerAssignment', () => {
  beforeEach(() => jest.clearAllMocks());

  it('shows the manager currently responsible for the outlet', async () => {
    mocked.getOutletManager.mockResolvedValue({
      outletId: 'OUT014',
      userId: 'u-store',
      name: 'Ishara S.',
      email: 'ishara@waypoint.lk',
      depotId: 'd-peli',
    });
    renderAssignment();

    expect((await screen.findAllByText(/Ishara S\./)).length).toBeGreaterThan(
      0,
    );
    expect(await screen.findByText(/manages this outlet/i)).toBeTruthy();
  });

  it('assigns a different manager', async () => {
    mocked.getOutletManager.mockResolvedValue(null);
    mocked.assignManager.mockResolvedValue({
      outletId: 'OUT014',
      userId: 'u-store2',
      name: 'Fathima R.',
      email: 'fathima@waypoint.lk',
      depotId: 'd-peli',
    });
    renderAssignment();
    await screen.findByText(/No store manager is assigned/i);

    fireEvent.change(screen.getByLabelText(/Assigned store manager/i), {
      target: { value: 'u-store2' },
    });
    fireEvent.click(screen.getByRole('button', { name: /Assign manager/i }));

    await waitFor(() =>
      expect(mocked.assignManager).toHaveBeenCalledWith('OUT014', {
        userId: 'u-store2',
      }),
    );
  });

  it('removes the manager after confirmation', async () => {
    mocked.getOutletManager.mockResolvedValue({
      outletId: 'OUT014',
      userId: 'u-store',
      name: 'Ishara S.',
      email: 'ishara@waypoint.lk',
      depotId: 'd-peli',
    });
    mocked.unassignManager.mockResolvedValue({
      outletId: 'OUT014',
      userId: 'u-store',
      name: 'Ishara S.',
      email: 'ishara@waypoint.lk',
      depotId: 'd-peli',
    });
    renderAssignment();
    await screen.findByText(/manages this outlet/i);

    fireEvent.click(screen.getByRole('button', { name: /Remove assignment/i }));
    fireEvent.click(screen.getByRole('button', { name: /Confirm removal/i }));

    await waitFor(() =>
      expect(mocked.unassignManager).toHaveBeenCalledWith('OUT014'),
    );
  });
});
