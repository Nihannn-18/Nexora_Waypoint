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
  type LegContext,
  type SyncEventOutcome,
} from '@waypoint/api-client';
import type { DeliveryOutcome, SyncEventsRequest } from '@waypoint/shared-types';

export type OutboxStatus = 'PENDING' | 'SYNCED' | 'REJECTED';

export interface OutboxEvent {
  readonly clientEventId: string;
  readonly legId: string;
  /** For display only: the outlet the leg delivers to. */
  readonly outletId?: string;
  readonly outcome: DeliveryOutcome;
  readonly occurredAt: string;
  readonly createdOffline: boolean;
  readonly reasonCode?: string;
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

let dbPromise: Promise<IDBDatabase> | null = null;

function openDb(): Promise<IDBDatabase> {
  dbPromise ??= new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, 1);
    req.onupgradeneeded = () => {
      req.result.createObjectStore(EVENTS, { keyPath: 'clientEventId' });
      req.result.createObjectStore(LEGS, { keyPath: 'legId' });
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

/* --- Sync ----------------------------------------------------------------- */

export interface SyncDeps {
  /** Uploads one POD image and returns its server-minted media key. */
  uploadPod(legId: string, blob: Blob): Promise<string>;
  syncEvents(
    body: SyncEventsRequest,
  ): Promise<{ results: readonly SyncEventOutcome[] }>;
}

export function toRequest(
  e: OutboxEvent,
): SyncEventsRequest['events'][number] {
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
    return { ...e, status: 'SYNCED', syncedAt: new Date().toISOString() };
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
 * A 4xx (other than auth/timeout/rate-limit) means this payload can never
 * succeed as-is, so retrying would loop forever. Everything else — offline,
 * 5xx, 401 (re-sign-in) — is transient and keeps the event PENDING.
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

async function doSync(deps: SyncDeps): Promise<void> {
  const pending = (await listEvents()).filter((e) => e.status === 'PENDING');
  const ready: OutboxEvent[] = [];
  const refused = new Set<string>();

  try {
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
        await put({ ...e, status: 'REJECTED', reason: 'POD image upload refused' });
      }
    }
    if (ready.length === 0) return;

    const { results } = await deps.syncEvents({ events: ready.map(toRequest) });
    const byId = new Map(results.map((r) => [r.clientEventId, r]));
    for (const e of ready) {
      const r = byId.get(e.clientEventId);
      if (r) await put(applyResult(e, r));
    }
  } catch (err) {
    for (const e of pending.filter((p) => !refused.has(p.clientEventId))) {
      await put(
        isPermanent(err)
          ? { ...e, status: 'REJECTED', reason: err.message }
          : { ...e, retryCount: e.retryCount + 1 },
      );
    }
    if (!isPermanent(err)) throw err;
  }
}
