import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { WaypointApiError } from '@waypoint/api-client';
import { api } from '../../../lib/api';
import { DemoControls } from './demo-controls';
import { DispatcherScopeProvider } from './dispatcher-context';

jest.mock('../../../lib/api', () => ({
  api: {
    meta: jest.fn(),
    getDepots: jest.fn(),
    getUnreadNotificationCount: jest.fn(),
    setDemoClock: jest.fn(),
    resetDemo: jest.fn(),
  },
}));

const mocked = api as jest.Mocked<typeof api>;

function meta(now: string, demoMode = true) {
  return { now, demoMode, timezone: 'Asia/Colombo' };
}

beforeEach(() => {
  jest.clearAllMocks();
  mocked.meta.mockResolvedValue(meta('2026-09-25T15:40:00+05:30'));
  mocked.getDepots.mockResolvedValue([]);
  mocked.getUnreadNotificationCount.mockResolvedValue(0);
});

function renderControls(onReset = jest.fn()) {
  render(
    <DispatcherScopeProvider>
      <DemoControls onReset={onReset} />
    </DispatcherScopeProvider>,
  );
  return { onReset };
}

describe('DemoControls', () => {
  it('renders nothing when the API is not in demo mode', async () => {
    mocked.meta.mockResolvedValue(meta('2026-10-04T10:00:00+05:30', false));
    renderControls();
    await waitFor(() => expect(mocked.meta).toHaveBeenCalled());
    expect(screen.queryByRole('button', { name: 'Reset demo' })).toBeNull();
    expect(screen.queryByRole('combobox')).toBeNull();
  });

  it('jumps the clock to a stage and re-reads /meta', async () => {
    mocked.setDemoClock.mockResolvedValue({
      ...meta('2026-09-26T03:30:00+05:30'),
      stage: 'LOADING',
    });
    renderControls();
    const select = await screen.findByRole('combobox');
    const metaCalls = mocked.meta.mock.calls.length;

    fireEvent.change(select, { target: { value: 'LOADING' } });

    await waitFor(() =>
      expect(mocked.setDemoClock).toHaveBeenCalledWith({ stage: 'LOADING' }),
    );
    await waitFor(() =>
      expect(mocked.meta.mock.calls.length).toBeGreaterThan(metaCalls),
    );
    expect(
      await screen.findByRole('option', {
        name: 'Loading · Sat 03:30',
        selected: true,
      }),
    ).toBeTruthy();
  });

  it('says the clock did not move when the jump fails', async () => {
    mocked.setDemoClock.mockRejectedValue(
      new WaypointApiError('Forbidden', { status: 403 }),
    );
    renderControls();
    fireEvent.change(await screen.findByRole('combobox'), {
      target: { value: 'ON_ROUTE' },
    });
    expect((await screen.findByRole('alert')).textContent).toMatch(
      /did not move/,
    );
  });

  it('resets only after a deliberate confirmation', async () => {
    mocked.resetDemo.mockResolvedValue({
      ...meta('2026-09-25T15:40:00+05:30'),
      cleared: { customer_order: 4 },
      demoOrders: 85,
      demoVehicleDays: 60,
    });
    const { onReset } = renderControls();

    fireEvent.click(await screen.findByRole('button', { name: 'Reset demo' }));
    expect(mocked.resetDemo).not.toHaveBeenCalled();
    expect(screen.getByRole('dialog')).toBeTruthy();

    fireEvent.click(screen.getByRole('button', { name: 'Reset demo data' }));
    await waitFor(() => expect(onReset).toHaveBeenCalledTimes(1));
    expect(mocked.resetDemo).toHaveBeenCalledTimes(1);
  });

  it('keeps the dialog open and says nothing changed when the reset fails', async () => {
    mocked.resetDemo.mockRejectedValue(
      new WaypointApiError('boom', { status: 500 }),
    );
    const { onReset } = renderControls();
    fireEvent.click(await screen.findByRole('button', { name: 'Reset demo' }));
    fireEvent.click(screen.getByRole('button', { name: 'Reset demo data' }));

    expect((await screen.findByRole('alert')).textContent).toMatch(
      /Nothing was changed/,
    );
    expect(screen.getByRole('dialog')).toBeTruthy();
    expect(onReset).not.toHaveBeenCalled();
  });
});
