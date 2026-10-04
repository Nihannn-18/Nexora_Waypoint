import type { LoadingLine } from '@waypoint/api-client';
import {
  changedLines,
  draftFor,
  fillPercent,
  hasShortfall,
  isLineComplete,
  lineError,
  progress,
  remainingQty,
  stopsInLoadOrder,
  submission,
} from './loading';
import type { Drafts, LineDraft } from './loading';

const line = (over: Partial<LoadingLine> = {}): LoadingLine => ({
  orderItemId: 'OI1',
  orderId: 'O1',
  itemId: 'IT1',
  sku: 'WF-YGT-08',
  name: 'Greek Yogurt Multi-pack',
  orderedQty: 16,
  loadedQty: 0,
  damagedQty: 0,
  missingQty: 0,
  shortfallQty: 0,
  seq: 1,
  outletId: 'OUT014',
  outletName: 'Waypoint Fresh Colombo Central',
  orderNumber: 'S1-001',
  dockType: 'REAR_DOCK',
  tempRequirement: 'CHILLED',
  weightKg: 32,
  volumeM3: 0.8,
  ...over,
});

const drafts = (entries: Record<string, LineDraft>): Drafts =>
  new Map(Object.entries(entries));

describe('load order', () => {
  // The behaviour a judge checks on L-02: the LAST drop is loaded FIRST, so it
  // sits deepest in the vehicle. Getting this backwards makes the truck
  // unloadable without taking everything out at the first stop.
  it('puts the last stop of the run first', () => {
    const stops = stopsInLoadOrder([
      line({ orderItemId: 'a', seq: 1, outletId: 'OUT001' }),
      line({ orderItemId: 'b', seq: 3, outletId: 'OUT003' }),
      line({ orderItemId: 'c', seq: 2, outletId: 'OUT002' }),
    ]);

    expect(stops.map((s) => s.seq)).toEqual([3, 2, 1]);
    expect(stops[0]?.outletId).toBe('OUT003');
  });

  it('keeps every line of a stop together', () => {
    const stops = stopsInLoadOrder([
      line({ orderItemId: 'a', seq: 2, orderNumber: 'S1-002' }),
      line({ orderItemId: 'b', seq: 2, orderNumber: 'S1-002' }),
      line({ orderItemId: 'c', seq: 1, orderNumber: 'S1-001' }),
    ]);

    expect(stops).toHaveLength(2);
    expect(stops[0]?.lines.map((l) => l.orderItemId)).toEqual(['a', 'b']);
    expect(stops[0]?.orderNumbers).toEqual(['S1-002']);
  });

  it('returns nothing for an empty picking list', () => {
    expect(stopsInLoadOrder([])).toEqual([]);
  });
});

describe('the load invariant', () => {
  const l = line({ orderedQty: 10 });

  it.each([
    ['all loaded', { loadedQty: 10, damagedQty: 0, missingQty: 0 }, true],
    ['split across states', { loadedQty: 8, damagedQty: 1, missingQty: 1 }, true],
    ['one short', { loadedQty: 8, damagedQty: 1, missingQty: 0 }, false],
    ['one over', { loadedQty: 9, damagedQty: 1, missingQty: 1 }, false],
  ])('%s', (_name, draft, expected) => {
    expect(isLineComplete(l, draft as LineDraft)).toBe(expected);
  });

  it('reports what is still uncounted', () => {
    expect(
      remainingQty(l, { loadedQty: 6, damagedQty: 0, missingQty: 0 }),
    ).toBe(4);
  });

  it('names the problem when too much is counted', () => {
    expect(
      lineError(l, { loadedQty: 10, damagedQty: 1, missingQty: 0 }),
    ).toContain('Remove 1');
  });

  it('names the problem when too little is counted', () => {
    expect(lineError(l, { loadedQty: 6, damagedQty: 0, missingQty: 0 })).toContain(
      '4 units are still uncounted',
    );
  });

  it('rejects negative quantities', () => {
    expect(lineError(l, { loadedQty: -1, damagedQty: 0, missingQty: 0 })).toBe(
      'Quantities cannot be negative.',
    );
  });

  it('passes a reconciled line', () => {
    expect(
      lineError(l, { loadedQty: 8, damagedQty: 1, missingQty: 1 }),
    ).toBeNull();
  });
});

describe('shortfall detection', () => {
  it('is a shortfall when anything is damaged or missing', () => {
    expect(hasShortfall({ loadedQty: 9, damagedQty: 1, missingQty: 0 })).toBe(
      true,
    );
    expect(hasShortfall({ loadedQty: 9, damagedQty: 0, missingQty: 1 })).toBe(
      true,
    );
  });

  it('is not a shortfall when everything loaded', () => {
    expect(hasShortfall({ loadedQty: 10, damagedQty: 0, missingQty: 0 })).toBe(
      false,
    );
  });
});

describe('drafts and submission', () => {
  const lines = [
    line({ orderItemId: 'a', orderedQty: 10 }),
    line({ orderItemId: 'b', orderedQty: 5, loadedQty: 5 }),
  ];

  it('falls back to what the server already recorded', () => {
    expect(draftFor(lines[1] as LoadingLine, new Map())).toEqual({
      loadedQty: 5,
      damagedQty: 0,
      missingQty: 0,
      photoRef: undefined,
    });
  });

  // A shared dock tablet must not resubmit values nobody typed, or one loader
  // silently overwrites another's count.
  it('sends only the lines that actually changed', () => {
    const changed = changedLines(
      lines,
      drafts({
        a: { loadedQty: 10, damagedQty: 0, missingQty: 0 },
        b: { loadedQty: 5, damagedQty: 0, missingQty: 0 },
      }),
    );
    expect(changed.map((l) => l.orderItemId)).toEqual(['a']);
  });

  it('refuses a submission with nothing to send', () => {
    expect(submission(lines, new Map()).error).toBe(
      'Nothing has changed since the last save.',
    );
  });

  it('refuses a submission whose line does not reconcile', () => {
    const { error } = submission(
      lines,
      drafts({ a: { loadedQty: 3, damagedQty: 0, missingQty: 0 } }),
    );
    expect(error).toContain('WF-YGT-08');
  });

  it('allows a reconciled change through', () => {
    const { items, error } = submission(
      lines,
      drafts({ a: { loadedQty: 8, damagedQty: 1, missingQty: 1 } }),
    );
    expect(error).toBeNull();
    expect(items).toHaveLength(1);
  });

  it('counts progress across the route', () => {
    const p = progress(
      lines,
      drafts({ a: { loadedQty: 8, damagedQty: 2, missingQty: 0 } }),
    );
    expect(p).toEqual({
      total: 2,
      complete: 2,
      shortfallQty: 2,
      loadedUnits: 13,
      orderedUnits: 15,
    });
  });
});

describe('capacity meters', () => {
  it('reads a percentage of capacity', () => {
    expect(fillPercent(4.82, 6.5)).toBe(74);
  });

  it('never exceeds the track or divides by zero', () => {
    expect(fillPercent(10, 5)).toBe(100);
    expect(fillPercent(1, 0)).toBe(0);
    expect(fillPercent(-1, 5)).toBe(0);
  });
});
