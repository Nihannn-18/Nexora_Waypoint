import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import { WaypointApiError } from '@waypoint/api-client';
import type {
  CustomerOrder,
  Outlet,
  PlanningResults,
  Vehicle,
} from '@waypoint/shared-types';
import { api } from '../../../lib/api';
import { DispatcherScopeProvider } from '../_components/dispatcher-context';
import { PlanScreen } from './plan-screen';

jest.mock('../../../lib/api', () => ({
  api: {
    meta: jest.fn(),
    getDepots: jest.fn(),
    getUnreadNotificationCount: jest.fn(),
    getPlanningResults: jest.fn(),
    listOrders: jest.fn(),
    getVehicles: jest.fn(),
    getOutlets: jest.fn(),
    listRoutes: jest.fn(),
    suggestPlan: jest.fn(),
    getPlanningJob: jest.fn(),
    confirmAllocation: jest.fn(),
  },
}));

let searchParams = new URLSearchParams('job=JOB-1');
const replace = jest.fn();
jest.mock('next/navigation', () => ({
  useRouter: () => ({ replace }),
  useSearchParams: () => searchParams,
  usePathname: () => '/dispatcher/plan',
}));

const mocked = api as jest.Mocked<typeof api>;

const order = (
  id: string,
  status: CustomerOrder['status'] = 'CONFIRMED',
): CustomerOrder => ({
  orderId: id,
  orderNumber: `ORD-${id}`,
  outletId: `OUT-${id}`,
  brand: 'FRESH',
  orderDate: '2026-09-25',
  requestedDeliveryDate: '2026-09-26',
  totalUnits: 5,
  totalWeightKg: 100,
  totalVolumeM3: 1,
  temperatureRequirement: 'CHILLED',
  status,
  afterCutoff: false,
});

const outlets: Outlet[] = ['a', 'x', 'y'].map((id) => ({
  outletId: `OUT-${id}`,
  name: `Outlet ${id}`,
  brand: 'FRESH',
  district: 'Colombo',
  depotId: 'd1',
  dockType: 'REAR_DOCK',
  parkingConstraint: 'NORMAL',
  mallWindow: null,
  windowOpenTime: '05:00',
  windowCloseTime: '08:00',
}));

const vehicle: Vehicle = {
  vehicleId: 'VEH014',
  type: 'TRUCK',
  tempClass: 'REEFER',
  weightCapKg: 2500,
  volumeCapM3: 18,
  fuelType: 'diesel',
  kmPerL: 6,
  weeklyFuelQuotaL: 400,
  depotId: 'd1',
  status: 'AVAILABLE',
};

const results: PlanningResults = {
  job: {
    jobId: 'JOB-1',
    planningDate: '2026-09-26',
    depotId: 'd1',
    status: 'COMPLETED',
    requestedBy: 'seed-dispatcher',
    createdAt: '2026-09-25T15:45:00+05:30',
  },
  proposals: [
    {
      orderId: 'a',
      decision: 'SERVE',
      vehicleId: 'VEH014',
      tripNo: 1,
      tripMinutes: 101,
      explanation: 'VEH014, trip 1',
    },
    {
      orderId: 'x',
      decision: 'DEFER',
      constraintCode: 'REEFER_REQUIRED',
      explanation: 'No reefer trip left.',
    },
    {
      orderId: 'y',
      decision: 'DEFER',
      constraintCode: 'REEFER_REQUIRED',
      explanation: 'No reefer trip left.',
    },
  ],
};

function withOrders(statuses: Record<string, CustomerOrder['status']> = {}) {
  mocked.listOrders.mockResolvedValue({
    orders: ['a', 'x', 'y'].map((id) => order(id, statuses[id])),
    total: 3,
    limit: 200,
    offset: 0,
  });
}

beforeEach(() => {
  jest.clearAllMocks();
  searchParams = new URLSearchParams('job=JOB-1');
  mocked.meta.mockResolvedValue({
    now: '2026-09-25T15:45:00+05:30',
    demoMode: true,
    timezone: 'Asia/Colombo',
  });
  mocked.getDepots.mockResolvedValue([
    { depotId: 'd1', code: 'PELIYAGODA', name: 'Peliyagoda' },
  ]);
  mocked.getUnreadNotificationCount.mockResolvedValue(0);
  mocked.getPlanningResults.mockResolvedValue(results);
  mocked.getVehicles.mockResolvedValue([vehicle]);
  mocked.getOutlets.mockResolvedValue(outlets);
  mocked.listRoutes.mockResolvedValue([]);
  withOrders();
});

function renderPlan() {
  return render(
    <DispatcherScopeProvider>
      <PlanScreen />
    </DispatcherScopeProvider>,
  );
}

const confirmButton = () =>
  screen.getByRole('button', { name: 'Review & confirm plan' });

describe('Plan review (D-03 / DG-A)', () => {
  it('leads with the cause when orders cannot be served', async () => {
    renderPlan();
    expect(
      await screen.findByText(/2 of 3 orders can’t be served on Sat 26 Sep/),
    ).toBeTruthy();
    expect(screen.getByText(/refrigeration is the main limit/)).toBeTruthy();
  });

  it('keeps Confirm disabled until every deferral has a reason', async () => {
    renderPlan();
    await screen.findByText(/Proposed deferrals/);
    expect(confirmButton()).toHaveProperty('disabled', true);
    expect(screen.getByText(/2 deferrals still need a reason/)).toBeTruthy();

    fireEvent.click(
      screen.getByRole('button', { name: /Use the engine’s reason for all 2/ }),
    );
    expect(confirmButton()).toHaveProperty('disabled', false);
  });

  it('re-blocks Confirm if a reason is cleared', async () => {
    renderPlan();
    await screen.findByText(/Proposed deferrals/);
    fireEvent.click(
      screen.getByRole('button', { name: /Use the engine’s reason for all 2/ }),
    );
    const [first] = screen.getAllByRole('textbox', {
      name: /Reason recorded for the store/,
    });
    fireEvent.change(first as HTMLElement, { target: { value: '  ' } });
    expect(confirmButton()).toHaveProperty('disabled', true);
  });

  it('confirms only from the dialog, then shows the confirmed state', async () => {
    mocked.confirmAllocation.mockResolvedValue({
      routeIds: ['R1'],
      allocatedOrders: ['a'],
      deferredOrders: ['x', 'y'],
    });
    renderPlan();
    await screen.findByText(/Proposed deferrals/);
    fireEvent.click(
      screen.getByRole('button', { name: /Use the engine’s reason for all 2/ }),
    );
    fireEvent.click(confirmButton());

    const dialog = await screen.findByRole('dialog');
    expect(
      within(dialog).getByText(/cannot be undone from this screen/),
    ).toBeTruthy();
    expect(mocked.confirmAllocation).not.toHaveBeenCalled();
    fireEvent.click(
      within(dialog).getByRole('button', { name: 'Confirm 1 route' }),
    );

    expect(
      await screen.findByText(/Plan confirmed for Sat 26 Sep/),
    ).toBeTruthy();
    expect(mocked.confirmAllocation).toHaveBeenCalledWith({
      jobId: 'JOB-1',
      routes: [{ vehicleId: 'VEH014', tripNo: 1, orderIds: ['a'] }],
      deferrals: [
        {
          orderId: 'x',
          reasonType: 'CONSTRAINT',
          constraintCode: 'REEFER_REQUIRED',
          reasonText: 'No reefer trip left.',
        },
        {
          orderId: 'y',
          reasonType: 'CONSTRAINT',
          constraintCode: 'REEFER_REQUIRED',
          reasonText: 'No reefer trip left.',
        },
      ],
    });
    // The plan cannot be sent twice.
    expect(
      screen.queryByRole('button', { name: 'Review & confirm plan' }),
    ).toBeNull();
  });

  it('reports a conflict honestly and does not claim success', async () => {
    mocked.confirmAllocation.mockRejectedValue(
      new WaypointApiError(
        'The plan conflicts with current state; reload and retry',
        {
          status: 409,
          code: 'CONFLICT',
        },
      ),
    );
    renderPlan();
    await screen.findByText(/Proposed deferrals/);
    fireEvent.click(
      screen.getByRole('button', { name: /Use the engine’s reason for all 2/ }),
    );
    fireEvent.click(confirmButton());
    fireEvent.click(
      within(await screen.findByRole('dialog')).getByRole('button', {
        name: 'Confirm 1 route',
      }),
    );

    expect(
      await screen.findByText(
        /Not confirmed — the plan conflicts with current data/,
      ),
    ).toBeTruthy();
    expect(screen.queryByText(/Plan confirmed for/)).toBeNull();
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('shows an already-confirmed plan as final', async () => {
    withOrders({ a: 'ALLOCATED', x: 'DEFERRED', y: 'DEFERRED' });
    renderPlan();
    expect(
      await screen.findByText(/This plan has already been confirmed/),
    ).toBeTruthy();
    expect(
      screen.queryByRole('button', { name: 'Review & confirm plan' }),
    ).toBeNull();
  });

  it('refuses to confirm a proposal that went stale', async () => {
    withOrders({ x: 'DEFERRED' });
    renderPlan();
    expect(
      await screen.findByText(/1 order changed since this proposal was made/),
    ).toBeTruthy();
    fireEvent.click(
      screen.getByRole('button', { name: /Use the engine’s reason for all/ }),
    );
    expect(confirmButton()).toHaveProperty('disabled', true);
  });
});

describe('Plan start', () => {
  beforeEach(() => {
    searchParams = new URLSearchParams();
  });

  it('runs planning and moves to the proposal', async () => {
    mocked.listOrders.mockResolvedValue({
      orders: [],
      total: 85,
      limit: 1,
      offset: 0,
    });
    mocked.suggestPlan.mockResolvedValue({ jobId: 'JOB-9', status: 'QUEUED' });
    mocked.getPlanningJob.mockResolvedValue({ ...results.job, jobId: 'JOB-9' });
    renderPlan();
    expect(await screen.findByText(/85 confirmed orders waiting/)).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Run planning' }));
    await waitFor(() =>
      expect(replace).toHaveBeenCalledWith('/dispatcher/plan?job=JOB-9'),
    );
    expect(mocked.suggestPlan).toHaveBeenCalledWith({
      planningDate: '2026-09-26',
      depotId: 'd1',
    });
  });

  it('does not offer a run when nothing is confirmed for the day', async () => {
    mocked.listOrders.mockResolvedValue({
      orders: [],
      total: 0,
      limit: 1,
      offset: 0,
    });
    renderPlan();
    expect(await screen.findByText(/nothing to plan/)).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Run planning' })).toHaveProperty(
      'disabled',
      true,
    );
  });
});
