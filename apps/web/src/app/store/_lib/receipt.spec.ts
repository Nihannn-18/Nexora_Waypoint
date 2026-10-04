import type { ReceiptView } from '@waypoint/shared-types';
import {
  awaitingReceipt,
  initialCounts,
  parseCount,
  previewLine,
  summarise,
  toReceiptRequest,
} from './receipt';
import { makeOrder } from './fixtures';

const view: ReceiptView = {
  orderId: 'o1',
  orderNumber: 'ORD-2026-000001',
  orderStatus: 'DELIVERED',
  lines: [
    {
      orderItemId: 'l1',
      sku: 'FR-001',
      name: 'Milk',
      orderedQty: 10,
      expectedQty: 8,
      expectedSource: 'LOADER',
      loaderFlag: { missingQty: 2, damagedQty: 0 },
    },
    {
      orderItemId: 'l2',
      sku: 'FR-002',
      name: 'Bread',
      orderedQty: 5,
      expectedQty: 5,
      expectedSource: 'ORDER',
    },
  ],
};

const [milk, bread] = view.lines;
if (!milk || !bread) throw new Error('fixture lines missing');

describe('GRN helpers', () => {
  it('pre-fills the loader’s count, so a flagged shortfall is not a new issue', () => {
    const counts = initialCounts(view);
    expect(counts['l1']).toEqual({ received: '8', damaged: '0' });
    expect(summarise(view.lines, counts)).toEqual({
      damaged: 0,
      short: 0,
      invalidLines: 0,
    });
  });

  it('derives short and condition from the counts', () => {
    expect(previewLine(milk, { received: '6', damaged: '1' })).toMatchObject({
      short: 1,
      condition: 'DAMAGED_AND_SHORT',
      error: null,
    });
    expect(previewLine(milk, { received: '7', damaged: '1' })).toMatchObject({
      short: 0,
      condition: 'DAMAGED',
    });
    expect(previewLine(bread, { received: '4', damaged: '0' })).toMatchObject({
      short: 1,
      condition: 'SHORT',
    });
  });

  it('accepts received + damaged exactly at what was sent, and refuses one more', () => {
    expect(previewLine(milk, { received: '7', damaged: '1' }).error).toBeNull();
    expect(previewLine(milk, { received: '8', damaged: '1' }).error).toMatch(
      /more than the 8 sent/,
    );
  });

  it('refuses blanks, negatives and fractions', () => {
    expect(parseCount('')).toBeNull();
    expect(parseCount('-1')).toBeNull();
    expect(parseCount('1.5')).toBeNull();
    expect(parseCount(' 3 ')).toBe(3);
    expect(previewLine(milk, { received: '', damaged: '0' }).error).toMatch(
      /whole numbers/,
    );
  });

  it('sends only counts and a trimmed note', () => {
    const body = toReceiptRequest(
      view.lines,
      {
        l1: { received: '7', damaged: '1' },
        l2: { received: '5', damaged: '0' },
      },
      '  one carton crushed ',
    );
    expect(body).toEqual({
      lines: [
        { orderItemId: 'l1', receivedQty: 7, damagedQty: 1 },
        { orderItemId: 'l2', receivedQty: 5, damagedQty: 0 },
      ],
      notes: 'one carton crushed',
    });
    expect(
      toReceiptRequest(view.lines, initialCounts(view), '  '),
    ).not.toHaveProperty('notes');
  });

  it('lists only delivered orders as awaiting a receipt', () => {
    const orders = [
      makeOrder({ orderId: 'a', status: 'DELIVERED' }),
      makeOrder({ orderId: 'b', status: 'RECEIVED' }),
      makeOrder({ orderId: 'c', status: 'IN_TRANSIT' }),
    ];
    expect(awaitingReceipt(orders).map((o) => o.orderId)).toEqual(['a']);
  });
});
