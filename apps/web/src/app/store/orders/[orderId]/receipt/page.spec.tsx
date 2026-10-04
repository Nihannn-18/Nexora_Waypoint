import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react';
import { Suspense } from 'react';
import { WaypointApiError } from '@waypoint/api-client';
import type { Receipt, ReceiptView } from '@waypoint/shared-types';
import { StoreShell } from '../../../_components/store-shell';
import ReceiptPage from './page';
import { demoMeta, ownOutlet } from '../../../_lib/fixtures';
import { api } from '../../../../../lib/api';
import { fetchMediaObjectUrl } from '../../../../../lib/media';

jest.mock('next/navigation', () => ({
  usePathname: () => '/store/orders/o1/receipt',
}));
jest.mock('../../../../../lib/api', () => ({
  api: {
    getOutlets: jest.fn(),
    meta: jest.fn(),
    getReceipt: jest.fn(),
    createReceipt: jest.fn(),
  },
  tokenStore: { get: () => null, set: jest.fn(), clear: jest.fn() },
}));
jest.mock('../../../../../lib/media', () => ({
  fetchMediaObjectUrl: jest.fn(),
}));

const mocked = api as unknown as {
  getOutlets: jest.Mock;
  meta: jest.Mock;
  getReceipt: jest.Mock;
  createReceipt: jest.Mock;
};
const mockedMedia = fetchMediaObjectUrl as jest.Mock;

const deliveredView: ReceiptView = {
  orderId: 'o1',
  orderNumber: 'ORD-2026-000001',
  orderStatus: 'DELIVERED',
  lines: [
    {
      orderItemId: 'l1',
      sku: 'FR-001',
      name: 'Fresh milk 1L',
      orderedQty: 10,
      expectedQty: 8,
      expectedSource: 'LOADER',
      loaderFlag: { missingQty: 2, damagedQty: 0 },
    },
    {
      orderItemId: 'l2',
      sku: 'FR-002',
      name: 'Sandwich bread',
      orderedQty: 5,
      expectedQty: 5,
      expectedSource: 'ORDER',
    },
  ],
  proofOfDelivery: {
    outcome: 'DELIVERED',
    receiverName: 'Ishara',
    fileRef: 'pod/LEG1/abc',
    occurredAt: '2026-09-26T07:42:00+05:30',
  },
};

const recordedWithIssue: Receipt = {
  receiptId: 'r1',
  orderId: 'o1',
  status: 'RECEIVED_WITH_ISSUE',
  receivedAt: '2026-09-26T07:50:00+05:30',
  receivedBy: 'u-store',
  receivedByName: 'Ishara S.',
  lines: [
    {
      orderItemId: 'l1',
      sku: 'FR-001',
      name: 'Fresh milk 1L',
      orderedQty: 10,
      expectedQty: 8,
      receivedQty: 7,
      damagedQty: 1,
      shortQty: 0,
      condition: 'DAMAGED',
    },
    {
      orderItemId: 'l2',
      sku: 'FR-002',
      name: 'Sandwich bread',
      orderedQty: 5,
      expectedQty: 5,
      receivedQty: 5,
      damagedQty: 0,
      shortQty: 0,
      condition: 'GOOD',
    },
  ],
  issues: [
    {
      type: 'DAMAGED',
      orderItemId: 'l1',
      sku: 'FR-001',
      name: 'Fresh milk 1L',
      quantity: 1,
    },
  ],
};

const renderPage = async () => {
  await act(async () => {
    render(
      <StoreShell>
        <Suspense fallback={null}>
          <ReceiptPage params={Promise.resolve({ orderId: 'o1' })} />
        </Suspense>
      </StoreShell>,
    );
  });
};

const sendButton = () =>
  screen.findByRole('button', {
    name: /send grn/i,
  }) as Promise<HTMLButtonElement>;

beforeEach(() => {
  jest.clearAllMocks();
  URL.revokeObjectURL = jest.fn();
  mocked.getOutlets.mockResolvedValue([ownOutlet]);
  mocked.meta.mockResolvedValue(demoMeta);
  mocked.getReceipt.mockResolvedValue(deliveredView);
  mockedMedia.mockResolvedValue('blob:pod');
});

describe('S-06 goods received note', () => {
  it('shows the loader’s flag pre-filled and the driver’s POD attached', async () => {
    await renderPage();

    expect(await screen.findByText(/loader flagged 2 missing/i)).toBeTruthy();
    const received = screen.getAllByLabelText(
      /received in good condition/i,
    ) as HTMLInputElement[];
    expect(received.map((input) => input.value)).toEqual(['8', '5']);
    expect(screen.getByText(/nothing to report/i)).toBeTruthy();
    expect(screen.getByText(/driver’s proof of delivery/i)).toBeTruthy();
    expect(await screen.findByAltText(/proof of delivery photo/i)).toBeTruthy();
    expect(mockedMedia).toHaveBeenCalledWith('pod/LEG1/abc');
  });

  it('disables Send GRN while a line counts more than was sent, and says why', async () => {
    await renderPage();

    const [milk] = screen.getAllByLabelText(/received in good condition/i);
    fireEvent.change(milk as HTMLInputElement, { target: { value: '9' } });

    expect((await sendButton()).disabled).toBe(true);
    expect(screen.getByText(/more than the 8 sent/i)).toBeTruthy();
    expect(screen.getByRole('alert').textContent).toMatch(
      /1 line need a valid count/i,
    );
    expect(mocked.createReceipt).not.toHaveBeenCalled();
  });

  it('sends the counts, then shows the GRN and the issues raised', async () => {
    mocked.createReceipt.mockResolvedValue(recordedWithIssue);
    await renderPage();

    const [milkReceived] = screen.getAllByLabelText(
      /received in good condition/i,
    );
    const [milkDamaged] = screen.getAllByLabelText(/^damaged$/i);
    fireEvent.change(milkReceived as HTMLInputElement, {
      target: { value: '7' },
    });
    fireEvent.change(milkDamaged as HTMLInputElement, {
      target: { value: '1' },
    });
    expect(screen.getByText(/reports 1 damaged and 0 short/i)).toBeTruthy();

    fireEvent.click(await sendButton());

    await waitFor(() =>
      expect(mocked.createReceipt).toHaveBeenCalledWith('o1', {
        lines: [
          { orderItemId: 'l1', receivedQty: 7, damagedQty: 1 },
          { orderItemId: 'l2', receivedQty: 5, damagedQty: 0 },
        ],
      }),
    );
    expect(await screen.findByText(/grn sent · 1 issue raised/i)).toBeTruthy();
    expect(screen.getByText(/issues raised/i)).toBeTruthy();
    expect(screen.getByText(/dispatcher was notified/i)).toBeTruthy();
  });

  it('blocks a second submission while the first is in flight', async () => {
    mocked.createReceipt.mockReturnValue(new Promise(() => undefined));
    await renderPage();

    fireEvent.click(await sendButton());
    const pending = (await screen.findByRole('button', {
      name: /sending grn/i,
    })) as HTMLButtonElement;
    expect(pending.disabled).toBe(true);
    fireEvent.click(pending);
    expect(mocked.createReceipt).toHaveBeenCalledTimes(1);
  });

  it('refreshes instead of failing silently when the GRN already exists', async () => {
    mocked.createReceipt.mockRejectedValue(
      new WaypointApiError('A receipt has already been recorded', {
        status: 409,
      }),
    );
    await renderPage();

    fireEvent.click(await sendButton());

    expect(await screen.findByText(/may already have a GRN/i)).toBeTruthy();
    await waitFor(() => expect(mocked.getReceipt).toHaveBeenCalledTimes(2));
  });

  it('shows a recorded GRN instead of the form', async () => {
    mocked.getReceipt.mockResolvedValue({
      ...deliveredView,
      orderStatus: 'RECEIVED',
      receipt: recordedWithIssue,
    });
    await renderPage();

    expect(
      await screen.findByText(/grn recorded · 1 issue raised/i),
    ).toBeTruthy();
    expect(screen.queryByRole('button', { name: /send grn/i })).toBeNull();
  });

  it('explains that an undelivered order has nothing to receive yet', async () => {
    mocked.getReceipt.mockResolvedValue({
      ...deliveredView,
      orderStatus: 'IN_TRANSIT',
      proofOfDelivery: undefined,
    });
    await renderPage();

    expect(await screen.findByText(/nothing to receive yet/i)).toBeTruthy();
    expect(screen.getByText(/as soon as their phone syncs/i)).toBeTruthy();
    expect(screen.queryByRole('button', { name: /send grn/i })).toBeNull();
  });

  it('shows an error when the order is out of scope', async () => {
    mocked.getReceipt.mockRejectedValue(
      new WaypointApiError('not found', { status: 404 }),
    );
    await renderPage();

    expect(
      await screen.findByText(/couldn't open this delivery/i),
    ).toBeTruthy();
  });
});
