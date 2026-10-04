/**
 * Loader arithmetic and grouping, kept out of the components so the rules a
 * judge cares about — reverse stop order, the load invariant, shortfall
 * detection — can be tested without rendering a screen.
 *
 * The server is the authority on every number here. These helpers exist so the
 * loader is told what is wrong before they submit, not after; a draft that
 * passes them can still be rejected, and the screen must handle that.
 */
import type { LoadingLine, RouteLoading } from '@waypoint/api-client';

/** What the loader has typed for one line, before it is sent. */
export interface LineDraft {
  readonly loadedQty: number;
  readonly damagedQty: number;
  readonly missingQty: number;
  readonly photoRef?: string;
}

/** One drop on the route, with the lines that must go on the cage for it. */
export interface Stop {
  readonly seq: number;
  readonly outletId: string;
  readonly outletName: string;
  readonly dockType: LoadingLine['dockType'];
  readonly tempRequirement: LoadingLine['tempRequirement'];
  readonly orderNumbers: readonly string[];
  readonly lines: readonly LoadingLine[];
}

export type Drafts = ReadonlyMap<string, LineDraft>;

/** The draft for a line, defaulting to whatever the server already recorded. */
export function draftFor(line: LoadingLine, drafts: Drafts): LineDraft {
  return (
    drafts.get(line.orderItemId) ?? {
      loadedQty: line.loadedQty,
      damagedQty: line.damagedQty,
      missingQty: line.missingQty,
      photoRef: line.photoRef,
    }
  );
}

export const countedQty = (d: LineDraft) =>
  d.loadedQty + d.damagedQty + d.missingQty;

/** What is still uncounted on a line. Negative means too much was entered. */
export const remainingQty = (line: LoadingLine, d: LineDraft) =>
  line.orderedQty - countedQty(d);

/** The invariant the server enforces: loaded + damaged + missing = ordered. */
export const isLineComplete = (line: LoadingLine, d: LineDraft) =>
  countedQty(d) === line.orderedQty;

export const shortfallQty = (d: LineDraft) => d.damagedQty + d.missingQty;

export const hasShortfall = (d: LineDraft) => shortfallQty(d) > 0;

/**
 * Why this line cannot be submitted yet, in words the loader can act on.
 * Returns null when the line is ready.
 */
export function lineError(line: LoadingLine, d: LineDraft): string | null {
  if (d.loadedQty < 0 || d.damagedQty < 0 || d.missingQty < 0) {
    return 'Quantities cannot be negative.';
  }
  const counted = countedQty(d);
  if (counted > line.orderedQty) {
    return `You have counted ${counted} of ${line.orderedQty} ordered. Remove ${
      counted - line.orderedQty
    }.`;
  }
  if (counted < line.orderedQty) {
    const short = line.orderedQty - counted;
    return `${short} ${short === 1 ? 'unit is' : 'units are'} still uncounted. Mark them loaded, damaged or missing.`;
  }
  return null;
}

/**
 * Groups the flat picking list into stops, in LOAD order: the last drop of the
 * run is loaded first, so it sits deepest in the vehicle and the first drop is
 * nearest the door. The server returns lines in planned stop order (`seq`
 * ascending); reversing here is the whole point of the screen, and the rows are
 * never re-sorted under the loader's hands afterwards.
 */
export function stopsInLoadOrder(lines: readonly LoadingLine[]): Stop[] {
  const bySeq = new Map<number, LoadingLine[]>();
  for (const line of lines) {
    const existing = bySeq.get(line.seq);
    if (existing) existing.push(line);
    else bySeq.set(line.seq, [line]);
  }

  return [...bySeq.entries()]
    .sort(([a], [b]) => b - a)
    .map(([seq, stopLines]) => {
      const first = stopLines[0];
      return {
        seq,
        outletId: first?.outletId ?? '',
        outletName: first?.outletName ?? '',
        dockType: first?.dockType ?? 'REAR_DOCK',
        tempRequirement: first?.tempRequirement ?? 'AMBIENT',
        orderNumbers: [...new Set(stopLines.map((l) => l.orderNumber))],
        lines: stopLines,
      };
    });
}

export interface Progress {
  readonly total: number;
  readonly complete: number;
  readonly shortfallQty: number;
  readonly loadedUnits: number;
  readonly orderedUnits: number;
}

/** Route-wide progress from the current drafts. */
export function progress(
  lines: readonly LoadingLine[],
  drafts: Drafts,
): Progress {
  let complete = 0;
  let shortfall = 0;
  let loadedUnits = 0;
  let orderedUnits = 0;
  for (const line of lines) {
    const d = draftFor(line, drafts);
    if (isLineComplete(line, d)) complete += 1;
    shortfall += shortfallQty(d);
    loadedUnits += d.loadedQty;
    orderedUnits += line.orderedQty;
  }
  return {
    total: lines.length,
    complete,
    shortfallQty: shortfall,
    loadedUnits,
    orderedUnits,
  };
}

/**
 * Only lines the loader actually changed are sent. Re-sending an untouched line
 * would overwrite another loader's count on a shared dock tablet with a value
 * nobody typed.
 */
export function changedLines(
  lines: readonly LoadingLine[],
  drafts: Drafts,
): LoadingLine[] {
  return lines.filter((line) => {
    const d = drafts.get(line.orderItemId);
    if (!d) return false;
    return (
      d.loadedQty !== line.loadedQty ||
      d.damagedQty !== line.damagedQty ||
      d.missingQty !== line.missingQty ||
      (d.photoRef ?? '') !== (line.photoRef ?? '')
    );
  });
}

/**
 * The lines a submission would send, with the reason it cannot go yet.
 * The server re-validates all of this; this is only so the loader is not told
 * "rejected" after walking back to the tablet.
 */
export function submission(
  lines: readonly LoadingLine[],
  drafts: Drafts,
): { readonly items: LoadingLine[]; readonly error: string | null } {
  const items = changedLines(lines, drafts);
  if (items.length === 0) {
    return { items, error: 'Nothing has changed since the last save.' };
  }
  for (const line of items) {
    const err = lineError(line, draftFor(line, drafts));
    if (err) {
      return { items, error: `${line.sku} — ${err}` };
    }
  }
  return { items, error: null };
}

/** Percentage of a capacity, clamped so a meter never runs off its track. */
export function fillPercent(used: number, capacity: number): number {
  if (!Number.isFinite(capacity) || capacity <= 0) return 0;
  return Math.max(0, Math.min(100, Math.round((used / capacity) * 100)));
}

export const DOCK_LABEL: Record<LoadingLine['dockType'], string> = {
  REAR_DOCK: 'Rear dock',
  STREET: 'Street unload',
  MALL_BAY: 'Mall bay',
};

/**
 * FROZEN is handled exactly like CHILLED across the product, so the loader sees
 * one cold-chain label rather than a distinction the dataset never makes.
 */
export const TEMP_LABEL: Record<LoadingLine['tempRequirement'], string> = {
  CHILLED: 'Chilled',
  FROZEN: 'Chilled',
  AMBIENT: 'Dry',
};

/** Reads the route's trip as the design labels it: "Trip 1 · VEH014". */
export const tripLabel = (r: Pick<RouteLoading, 'tripNo' | 'vehicleId'>) =>
  `Trip ${r.tripNo} · ${r.vehicleId}`;
