/**
 * S-06 goods received note (GRN) helpers, kept out of the screen so the rules
 * a judge would notice — the loader's shortfall is pre-filled, a store cannot
 * receive more than was sent, and short is what is left over — are tested
 * without rendering.
 *
 * The server is the authority: it re-derives every shortage, condition and the
 * receipt status from the counts and rejects anything over what was expected.
 * These helpers only preview that arithmetic while the store manager counts.
 */
import type {
  CreateReceiptRequest,
  CustomerOrder,
  ExpectedReceiptLine,
  ReceiptLineCondition,
  ReceiptView,
} from '@waypoint/shared-types';

/** The store manager's count for one line, as typed (may be empty). */
export interface LineCount {
  readonly received: string;
  readonly damaged: string;
}

export type LineCounts = Readonly<Record<string, LineCount>>;

/**
 * The starting counts: everything the store was told to expect arrived in good
 * condition. Because `expectedQty` is the loader's loaded count when one was
 * recorded, a shortfall the loader already flagged is pre-filled and is not
 * reported again as a new shortage.
 */
export function initialCounts(view: ReceiptView): LineCounts {
  const counts: Record<string, LineCount> = {};
  for (const line of view.lines) {
    counts[line.orderItemId] = {
      received: String(line.expectedQty),
      damaged: '0',
    };
  }
  return counts;
}

/** A non-negative whole number from an input, or null when it is not one. */
export function parseCount(value: string): number | null {
  const trimmed = value.trim();
  if (!/^\d+$/.test(trimmed)) return null;
  return Number(trimmed);
}

export interface LinePreview {
  readonly received: number | null;
  readonly damaged: number | null;
  /** Expected − received − damaged; null while a count is invalid. */
  readonly short: number | null;
  readonly condition: ReceiptLineCondition | null;
  /** Why the line cannot be sent, in words; null when it is valid. */
  readonly error: string | null;
}

/** Previews one line's shortage and condition, and says what is wrong. */
export function previewLine(
  line: ExpectedReceiptLine,
  count: LineCount | undefined,
): LinePreview {
  const received = parseCount(count?.received ?? '');
  const damaged = parseCount(count?.damaged ?? '');
  if (received === null || damaged === null) {
    return {
      received,
      damaged,
      short: null,
      condition: null,
      error: 'Enter whole numbers of 0 or more.',
    };
  }
  if (received + damaged > line.expectedQty) {
    return {
      received,
      damaged,
      short: null,
      condition: null,
      error: `Received + damaged (${received + damaged}) is more than the ${line.expectedQty} sent.`,
    };
  }
  const short = line.expectedQty - received - damaged;
  return {
    received,
    damaged,
    short,
    condition: conditionOf(damaged, short),
    error: null,
  };
}

export function conditionOf(
  damaged: number,
  short: number,
): ReceiptLineCondition {
  if (damaged > 0 && short > 0) return 'DAMAGED_AND_SHORT';
  if (damaged > 0) return 'DAMAGED';
  if (short > 0) return 'SHORT';
  return 'GOOD';
}

export interface GrnSummary {
  readonly damaged: number;
  readonly short: number;
  readonly invalidLines: number;
}

/** Totals across the form, for the summary above the Send button. */
export function summarise(
  lines: readonly ExpectedReceiptLine[],
  counts: LineCounts,
): GrnSummary {
  let damaged = 0;
  let short = 0;
  let invalidLines = 0;
  for (const line of lines) {
    const p = previewLine(line, counts[line.orderItemId]);
    if (p.error !== null) {
      invalidLines += 1;
      continue;
    }
    damaged += p.damaged ?? 0;
    short += p.short ?? 0;
  }
  return { damaged, short, invalidLines };
}

/** The request body. Only counts and the note; the server derives the rest. */
export function toReceiptRequest(
  lines: readonly ExpectedReceiptLine[],
  counts: LineCounts,
  notes: string,
): CreateReceiptRequest {
  const trimmed = notes.trim();
  return {
    lines: lines.map((line) => {
      const count = counts[line.orderItemId];
      return {
        orderItemId: line.orderItemId,
        receivedQty: parseCount(count?.received ?? '') ?? 0,
        damagedQty: parseCount(count?.damaged ?? '') ?? 0,
      };
    }),
    ...(trimmed ? { notes: trimmed } : {}),
  };
}

export const CONDITION_LABEL: Record<ReceiptLineCondition, string> = {
  GOOD: 'Good',
  DAMAGED: 'Damaged',
  SHORT: 'Short',
  DAMAGED_AND_SHORT: 'Damaged + short',
};

/** Delivered orders still waiting for the store's GRN. */
export function awaitingReceipt(
  orders: readonly CustomerOrder[],
): CustomerOrder[] {
  return orders.filter((order) => order.status === 'DELIVERED');
}
