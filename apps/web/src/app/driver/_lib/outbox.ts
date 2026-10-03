/**
 * The Driver's offline outbox.
 *
 * Every outcome is written to IndexedDB first and only then uploaded, so a
 * delivery is never lost to a dead network. The clientEventId is minted once,
 * at capture, and reused on every retry — the API's idempotency key
 * (delivery_event.client_event_id), so a retry can never double-record.
 *
 * POD images are stored as Blobs on the event and uploaded just before the
 * event itself; the server-minted `pod/<legId>/…` key is saved back on the
 * record, so a retried sync never uploads the same image twice.
 */
import {
  WaypointApiError,
  type DriverRoute,
  type LegContext,
  type SyncEventOutcome,
} from '@waypoint/api-client';
import type {
  DeliveryFailureReason,
  DeliveryOutcome,
  SyncEventsRequest,
} from '@waypoint/shared-types';

export type OutboxStatus = 'PENDING' | 'SYNCED' | 'REJECTED';

export interface OutboxEvent {
  readonly clientEventId: string;
  readonly legId: string;
  /** For display only: the outlet the leg delivers to. */
  readonly outletId?: string;
  readonly outcome: DeliveryOutcome;
  readonly occurredAt: string;
  readonly createdOffline: boolean;
  readonly reasonCode?: DeliveryFailureReason;
  readonly notes?: string;
  readonly receiverName?: string;
  readonly photo?: Blob;
  readonly signature?: Blob;
  photoRef?: string;
  signatureRef?: string;
  status: OutboxStatus;
  retryCount: number;
  /** Server explanation when REJECTED / CONFLICT. */
  reason?: string;
  syncedAt?: string;
}

export type NewEvent = Omit<
  OutboxEvent,
  | 'clientEventId'
  | 'occurredAt'
  | 'createdOffline'
  | 'status'
  | 'retryCount'
  | 'photoRef'
  | 'signatureRef'
  | 'reason'
  | 'syncedAt'
>;

const DB_NAME = 'waypoint-driver';
const EVENTS = 'outbox';
const LEGS = 'legs';
const ROUTES = 'routes';
const SELECTION = 'selection';

let dbPromise: Promise<IDBDatabase> | null = null;

function openDb(): Promise<IDBDatabase> {
  dbPromise ??= new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, 3);
    req.onupgradeneeded = (ev) => {
      // Each version bump creates only the stores it introduced: the previous
      // unconditional `createObjectStore(ROUTES)` threw on any upgrade past v1.
      if (ev.oldVersion < 1) {
        req.result.createObjectStore(EVENTS, { keyPath: 'clientEventId' });
        req.result.createObjectStore(LEGS, { keyPath: 'legId' });
      }
      if (ev.oldVersion < 2) {
        req.result.createObjectStore(ROUTES, { keyPath: 'date' });
      }
      if (ev.oldVersion < 3) {
        req.result.createObjectStore(SELECTION, { keyPath: 'date' });
      }
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
  return dbPromise;
}

async function run<T>(
  store: string,
  mode: IDBTransactionMode,
  op: (s: IDBObjectStore) => IDBRequest<T>,
): Promise<T> {
  const db = await openDb();
  return new Promise((resolve, reject) => {
    const req = op(db.transaction(store, mode).objectStore(store));
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

/** Test hook: close and forget the cached connection. */
export async function resetDbForTests(): Promise<void> {
  (await dbPromise?.catch(() => null))?.close();
  dbPromise = null;
}

const put = (e: OutboxEvent) => run(EVENTS, 'readwrite', (s) => s.put(e));

/** Listeners re-render the UI whenever the outbox changes. */
const listeners = new Set<() => void>();
export function subscribe(fn: () => void): () => void {
  listeners.add(fn);
  return () => listeners.delete(fn);
}
const notify = () => listeners.forEach((fn) => fn());

/** Saves the outcome on the phone. Returns immediately; upload happens later. */
export async function enqueue(input: NewEvent): Promise<OutboxEvent> {
  const event: OutboxEvent = {
    ...input,
    clientEventId: crypto.randomUUID(),
    occurredAt: new Date().toISOString(),
    createdOffline: typeof navigator !== 'undefined' && !navigator.onLine,
    status: 'PENDING',
    retryCount: 0,
  };
  await put(event);
  notify();
  return event;
}

/** All events in capture order — the order they upload in. */
export async function listEvents(): Promise<OutboxEvent[]> {
  const all = await run<OutboxEvent[]>(EVENTS, 'readonly', (s) => s.getAll());
  return all.sort((a, b) => a.occurredAt.localeCompare(b.occurredAt));
}

export function getEvent(id: string): Promise<OutboxEvent | undefined> {
  return run(EVENTS, 'readonly', (s) => s.get(id));
}

/** Leg context cached for offline use — only what GET /legs/{id} returns. */
export function cacheLeg(leg: LegContext): Promise<IDBValidKey> {
  return run(LEGS, 'readwrite', (s) => s.put(leg));
}
export function getCachedLeg(legId: string): Promise<LegContext | undefined> {
  return run(LEGS, 'readonly', (s) => s.get(legId));
}
export function listLegs(): Promise<LegContext[]> {
  return run(LEGS, 'readonly', (s) => s.getAll());
}

/** The run sheet (GET /driver/routes) for one date, saved while online. */
export interface CachedRoutes {
  readonly date: string;
  readonly routes: readonly DriverRoute[];
}
export function cacheRoutes(c: CachedRoutes): Promise<IDBValidKey> {
  return run(ROUTES, 'readwrite', (s) => s.put(c));
}
/** Offline there is no API clock, so the newest saved run sheet is today's. */
export async function latestCachedRoutes(): Promise<CachedRoutes | undefined> {
  const all = await run<CachedRoutes[]>(ROUTES, 'readonly', (s) => s.getAll());
  return all.sort((a, b) => a.date.localeCompare(b.date)).at(-1);
}

/**
 * The route this driver has explicitly chosen for a date. The schema has no
 * driver→vehicle link, so the run cannot be derived server-side; the driver
 * picks their own run and we remember it, rather than guessing from the
 * depot's first unfinished route (which could be another vehicle's).
 */
interface RouteSelection {
  readonly date: string;
  readonly routeId: string;
}
export function setSelectedRoute(
  date: string,
  routeId: string,
): Promise<IDBValidKey> {
  return run(SELECTION, 'readwrite', (s) => s.put({ date, routeId }));
}
export async function getSelectedRoute(
  date: string,
): Promise<string | undefined> {
  const row = await run<RouteSelection | undefined>(
    SELECTION,
    'readonly',
    (s) => s.get(date),
  );
  return row?.routeId;
}
export function clearSelectedRoute(date: string): Promise<undefined> {
  return run<undefined>(SELECTION, 'readwrite', (s) => s.delete(date));
}

/* --- Sync ----------------------------------------------------------------- */

export interface SyncDeps {
  /** Uploads one POD image and returns its server-minted media key. */
  uploadPod(legId: string, blob: Blob): Promise<string>;
  syncEvents(
    body: SyncEventsRequest,
  ): Promise<{ results: readonly SyncEventOutcome[] }>;
}

export function toRequest(e: OutboxEvent): SyncEventsRequest['events'][number] {
  const hasPod = Boolean(e.receiverName || e.photoRef || e.signatureRef);
  return {
    legId: e.legId,
    clientEventId: e.clientEventId,
    outcome: e.outcome,
    occurredAt: e.occurredAt,
    createdOffline: e.createdOffline,
    reasonCode: e.reasonCode,
    notes: e.notes,
    proofOfDelivery: hasPod
      ? {
          type: e.photoRef ? 'PHOTO' : e.signatureRef ? 'SIGNATURE' : 'NONE',
          receiverName: e.receiverName,
          signature: e.signatureRef,
          fileRef: e.photoRef,
        }
      : undefined,
  };
}

/**
 * Applies the server's per-event verdicts. ACCEPTED and DUPLICATE both mean the
 * server holds this event — a DUPLICATE is our own earlier upload whose reply
 * was lost. REJECTED and CONFLICT are kept, visible, and never marked synced.
 */
export function applyResult(e: OutboxEvent, r: SyncEventOutcome): OutboxEvent {
  if (r.status === 'ACCEPTED' || r.status === 'DUPLICATE') {
    // Clear any transient note so a once-warned event reads clean once synced.
    return {
      ...e,
      status: 'SYNCED',
      syncedAt: new Date().toISOString(),
      reason: undefined,
    };
  }
  return { ...e, status: 'REJECTED', reason: r.reason ?? r.status };
}

/** Retry delay after the nth consecutive failure: 5 s doubling, capped at 5 min. */
export const backoffMs = (failures: number) =>
  Math.min(5_000 * 2 ** Math.max(0, failures - 1), 300_000);

let inFlight: Promise<void> | null = null;

/**
 * Uploads every PENDING event. Single-flight: concurrent calls share one run.
 * Throws when the network or server failed, so the caller can back off; the
 * events stay PENDING with the same clientEventId.
 */
export function syncNow(deps: SyncDeps): Promise<void> {
  if (!inFlight) {
    inFlight = doSync(deps).finally(() => {
      inFlight = null;
      notify();
    });
    notify();
  }
  return inFlight;
}

export const isSyncing = () => inFlight !== null;

/**
 * A per-event 4xx (other than auth/timeout/rate-limit) means this payload —
 * usually one POD image the media endpoint refused — can never succeed as-is,
 * so retrying would loop forever. Everything else (offline, 5xx, 401) stays
 * PENDING. A 403 here is a refused image, not an auth problem.
 */
function isPermanent(err: unknown): err is WaypointApiError {
  return (
    err instanceof WaypointApiError &&
    !err.isOffline &&
    err.status >= 400 &&
    err.status < 500 &&
    ![401, 408, 429].includes(err.status)
  );
}

/**
 * A terminal failure of the whole `/sync/events` call. It is deliberately
 * narrower than isPermanent: `/sync/events` reports per-event verdicts as a
 * 200, so a top-level 401/403 is an auth/scope problem that may resolve on
 * retry and must never discard the driver's queued work. A top-level 400/422
 * (a malformed batch) is terminal but still only marks events REJECTED — they
 * remain visible, never silently lost.
 */
function isBatchTerminal(err: unknown): err is WaypointApiError {
  return (
    err instanceof WaypointApiError &&
    !err.isOffline &&
    err.status >= 400 &&
    err.status < 500 &&
    ![401, 403, 408, 429].includes(err.status)
  );
}

/** A short, visible note for a transient auth/scope failure, else undefined. */
function transientNote(err: unknown): string | undefined {
  if (
    err instanceof WaypointApiError &&
    (err.status === 401 || err.status === 403)
  ) {
    return 'Couldn’t authenticate the upload — kept on this phone, will retry';
  }
  return undefined;
}

/** Wraps a thrown batch-sync error so the outer catch does not reconcile twice. */
class BatchSyncError extends Error {
  readonly source: unknown;
  constructor(source: unknown) {
    super(source instanceof Error ? source.message : 'sync failed');
    this.source = source;
  }
}

async function doSync(deps: SyncDeps): Promise<void> {
  const pending = (await listEvents()).filter((e) => e.status === 'PENDING');
  const ready: OutboxEvent[] = [];
  const refused = new Set<string>();

  try {
    // 1. Upload POD images. A per-event permanent refusal rejects just that
    //    event and lets the rest proceed.
    for (const e of pending) {
      try {
        if (e.photo && !e.photoRef) {
          e.photoRef = await deps.uploadPod(e.legId, e.photo);
          await put(e);
        }
        if (e.signature && !e.signatureRef) {
          e.signatureRef = await deps.uploadPod(e.legId, e.signature);
          await put(e);
        }
        ready.push(e);
      } catch (err) {
        if (!isPermanent(err)) throw err;
        refused.add(e.clientEventId);
        await put({
          ...e,
          status: 'REJECTED',
          reason: 'POD image upload refused',
        });
      }
    }
    if (ready.length === 0) return;

    // 2. Batch sync. A top-level auth/scope failure must never discard the
    //    queue: transient errors keep every event PENDING for a later retry.
    let results: readonly SyncEventOutcome[];
    try {
      ({ results } = await deps.syncEvents({ events: ready.map(toRequest) }));
    } catch (err) {
      const terminal = isBatchTerminal(err);
      const note = transientNote(err);
      for (const e of ready) {
        await put(
          terminal
            ? {
                ...e,
                status: 'REJECTED',
                reason: err instanceof Error ? err.message : String(err),
              }
            : { ...e, retryCount: e.retryCount + 1, reason: note },
        );
      }
      if (!terminal) throw new BatchSyncError(err);
      return;
    }

    const byId = new Map(results.map((r) => [r.clientEventId, r]));
    for (const e of ready) {
      const r = byId.get(e.clientEventId);
      if (r) await put(applyResult(e, r));
    }
  } catch (err) {
    // The batch path already reconciled; rethrow for the caller's backoff.
    if (err instanceof BatchSyncError) throw err;

    // A transient failure while uploading an image: keep the queue, bump the
    // retry count so the UI shows the events are still trying.
    const note = transientNote(err);
    for (const e of pending.filter((p) => !refused.has(p.clientEventId))) {
      await put({ ...e, retryCount: e.retryCount + 1, reason: note });
    }
    throw err;
  }
}
