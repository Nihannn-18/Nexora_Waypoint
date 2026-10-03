'use client';

import { useCallback, useEffect, useState } from 'react';
import { WaypointApiError, type LegContext } from '@waypoint/api-client';
import { useOnlineStatus } from '@waypoint/ui';
import { api, tokenStore } from '../../../lib/api';
import {
  backoffMs,
  cacheLeg,
  getCachedLeg,
  isSyncing,
  listEvents,
  subscribe,
  syncNow,
  type OutboxEvent,
  type SyncDeps,
} from './outbox';

/** Mints a POD key on the API, then PUTs the bytes where it says. */
async function uploadPod(legId: string, blob: Blob): Promise<string> {
  const up = await api.createPodUpload(legId, blob.type);
  const headers: Record<string, string> = { ...up.headers };
  if (up.uploadMode === 'inline') {
    // The local backend's PUT route is our own API: authenticate it.
    headers['Content-Type'] = blob.type;
    const token = tokenStore.get();
    if (token) headers['Authorization'] = `Bearer ${token}`;
  }
  let res: Response;
  try {
    res = await fetch(up.uploadUrl, { method: 'PUT', headers, body: blob });
  } catch {
    throw new WaypointApiError('Could not reach the server.', {
      status: 0,
      isOffline: true,
    });
  }
  if (!res.ok) {
    throw new WaypointApiError(`Upload failed (${res.status})`, {
      status: res.status,
    });
  }
  return up.fileRef;
}

const deps: SyncDeps = {
  uploadPod,
  syncEvents: (body) => api.syncEvents(body),
};

let failures = 0;
let retryTimer: ReturnType<typeof setTimeout> | undefined;

/** Syncs now; on a transient failure schedules one retry with backoff. */
export async function trySync(): Promise<void> {
  clearTimeout(retryTimer);
  if (!navigator.onLine) return;
  try {
    await syncNow(deps);
    failures = 0;
  } catch {
    failures += 1;
    retryTimer = setTimeout(trySync, backoffMs(failures));
  }
}

export interface OutboxState {
  readonly events: readonly OutboxEvent[];
  readonly pending: number;
  readonly online: boolean;
  readonly syncing: boolean;
  readonly sync: () => Promise<void>;
}

/** Live view of the outbox; also kicks a sync on mount and when signal returns. */
export function useOutbox(): OutboxState {
  const online = useOnlineStatus();
  const [events, setEvents] = useState<OutboxEvent[]>([]);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    const load = () => {
      setBusy(isSyncing());
      listEvents().then(setEvents, () => setEvents([]));
    };
    load();
    return subscribe(load);
  }, []);

  useEffect(() => {
    if (online) void trySync();
  }, [online]);

  const sync = useCallback(() => trySync(), []);

  return {
    events,
    pending: events.filter((e) => e.status === 'PENDING').length,
    online,
    syncing: busy,
    sync,
  };
}

export type LegState =
  | { readonly status: 'loading' }
  | { readonly status: 'ready'; readonly leg: LegContext; readonly cached: boolean }
  | { readonly status: 'error'; readonly message: string };

/** GET /legs/{id}, cached in IndexedDB; offline falls back to the cached copy. */
export function useLeg(legId: string): LegState {
  const [state, setState] = useState<LegState>({ status: 'loading' });

  useEffect(() => {
    let live = true;
    const done = (s: LegState) => live && setState(s);
    api
      .getLeg(legId)
      .then(async (leg) => {
        await cacheLeg(leg);
        done({ status: 'ready', leg, cached: false });
      })
      .catch(async (err: unknown) => {
        const leg = await getCachedLeg(legId).catch(() => undefined);
        if (leg) return done({ status: 'ready', leg, cached: true });
        done({
          status: 'error',
          message:
            err instanceof WaypointApiError && err.isOffline
              ? 'This stop isn’t saved on the phone yet. Open it once with signal.'
              : err instanceof WaypointApiError && err.status === 404
                ? 'This stop doesn’t exist or isn’t on your depot’s routes.'
                : 'Couldn’t load this stop. Try again in a moment.',
        });
      });
    return () => {
      live = false;
    };
  }, [legId]);

  return state;
}
