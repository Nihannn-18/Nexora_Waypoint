/**
 * @jest-environment node
 */
import 'fake-indexeddb/auto';
import { WaypointApiError, type SyncEventOutcome } from '@waypoint/api-client';
import {
  backoffMs,
  enqueue,
  getEvent,
  listEvents,
  resetDbForTests,
  syncNow,
  toRequest,
  type SyncDeps,
} from './outbox';

const offline = () => new WaypointApiError('offline', { status: 0, isOffline: true });

/** Server stub answering every event with one status. */
function server(status: SyncEventOutcome['status'], reason?: string) {
  const calls: string[][] = [];
  const deps: SyncDeps = {
    uploadPod: jest.fn(async (legId: string) => `pod/${legId}/obj`),
    syncEvents: jest.fn(async ({ events }) => {
      calls.push(events.map((e) => e.clientEventId));
      return { results: events.map((e) => ({ clientEventId: e.clientEventId, status, reason })) };
    }),
  };
  return { deps, calls };
}

beforeEach(async () => {
  await resetDbForTests();
  await new Promise((done) => {
    const req = indexedDB.deleteDatabase('waypoint-driver');
    req.onsuccess = req.onerror = req.onblocked = done;
  });
});

it('persists an outcome in IndexedDB as PENDING with a client event id', async () => {
  const e = await enqueue({ legId: 'leg-1', outcome: 'FAILED', reasonCode: 'OUTLET_CLOSED' });
  await resetDbForTests(); // a fresh connection reads it back from storage
  expect(await getEvent(e.clientEventId)).toMatchObject({
    legId: 'leg-1',
    status: 'PENDING',
    retryCount: 0,
  });
  expect(e.clientEventId).toMatch(/^[0-9a-f-]{36}$/);
});

it.each([
  ['ACCEPTED', 'SYNCED'],
  ['DUPLICATE', 'SYNCED'],
  ['REJECTED', 'REJECTED'],
  ['CONFLICT', 'REJECTED'],
] as const)('%s from the server leaves the event %s', async (result, local) => {
  const e = await enqueue({ legId: 'leg-1', outcome: 'DELAYED' });
  await syncNow(server(result, 'leg reallocated').deps);
  const after = await getEvent(e.clientEventId);
  expect(after?.status).toBe(local);
  if (local === 'REJECTED') expect(after?.reason).toBe('leg reallocated');
});

it('keeps the same clientEventId across a failed and a successful retry', async () => {
  const e = await enqueue({ legId: 'leg-1', outcome: 'DELAYED' });
  const { deps, calls } = server('ACCEPTED');
  (deps.syncEvents as jest.Mock).mockRejectedValueOnce(offline());

  await expect(syncNow(deps)).rejects.toThrow('offline');
  expect(await getEvent(e.clientEventId)).toMatchObject({ status: 'PENDING', retryCount: 1 });

  await syncNow(deps);
  expect(calls).toEqual([[e.clientEventId]]);
  expect((await getEvent(e.clientEventId))?.status).toBe('SYNCED');
});

it('uploads a POD image once, even when the event upload is retried', async () => {
  const e = await enqueue({
    legId: 'leg-1',
    outcome: 'DELIVERED',
    receiverName: 'Receiver',
    photo: new Blob(['x'], { type: 'image/jpeg' }),
  });
  const { deps } = server('ACCEPTED');
  (deps.syncEvents as jest.Mock).mockRejectedValueOnce(offline());

  await syncNow(deps).catch(() => undefined);
  await syncNow(deps);

  expect(deps.uploadPod).toHaveBeenCalledTimes(1);
  const sent = (deps.syncEvents as jest.Mock).mock.calls[1][0].events[0];
  expect(sent.proofOfDelivery).toEqual({
    type: 'PHOTO',
    receiverName: 'Receiver',
    signature: undefined,
    fileRef: 'pod/leg-1/obj',
  });
  expect((await getEvent(e.clientEventId))?.status).toBe('SYNCED');
});

it('rejects only the event whose image the server refuses', async () => {
  const bad = await enqueue({
    legId: 'leg-1',
    outcome: 'DELIVERED',
    receiverName: 'Receiver',
    photo: new Blob(['x'], { type: 'image/jpeg' }),
  });
  const good = await enqueue({ legId: 'leg-2', outcome: 'DELAYED' });
  const { deps, calls } = server('ACCEPTED');
  (deps.uploadPod as jest.Mock).mockRejectedValueOnce(
    new WaypointApiError('forbidden', { status: 403 }),
  );

  await syncNow(deps);

  expect((await getEvent(bad.clientEventId))?.status).toBe('REJECTED');
  expect((await getEvent(good.clientEventId))?.status).toBe('SYNCED');
  expect(calls).toEqual([[good.clientEventId]]);
});

it('uploads in capture order and sends nothing when the queue is empty', async () => {
  const a = await enqueue({ legId: 'leg-1', outcome: 'DELAYED' });
  await new Promise((r) => setTimeout(r, 2));
  const b = await enqueue({ legId: 'leg-2', outcome: 'DELAYED' });
  const { deps, calls } = server('ACCEPTED');
  await syncNow(deps);
  await syncNow(deps);
  expect(calls).toEqual([[a.clientEventId, b.clientEventId]]);
  expect((await listEvents()).map((e) => e.status)).toEqual(['SYNCED', 'SYNCED']);
});

it('omits proofOfDelivery for an outcome without one', async () => {
  const e = await enqueue({ legId: 'leg-1', outcome: 'FAILED', reasonCode: 'OTHER' });
  expect(toRequest(e).proofOfDelivery).toBeUndefined();
});

it('backs off 5 s, doubling, capped at 5 minutes', () => {
  expect([1, 2, 3, 10].map(backoffMs)).toEqual([5_000, 10_000, 20_000, 300_000]);
});
