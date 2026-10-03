'use client';

import { useCallback, useEffect, useState } from 'react';
import {
  WaypointApiError,
  type DriverRoute,
  type LegContext,
} from '@waypoint/api-client';
import { useOnlineStatus } from '@waypoint/ui';
import { api, tokenStore } from '../../../lib/api';
import {
  backoffMs,
  cacheLeg,
  cacheRoutes,
  getCachedLeg,
  latestCachedRoutes,
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
  | {
      readonly status: 'ready';
      readonly leg: LegContext;
      readonly cached: boolean;
    }
  | { readonly status: 'error'; readonly message: string };

/** Says why a driver read failed; a 401 is never dressed up as success. */
export function loadError(err: unknown, offline: string, what: string): string {
  if (!(err instanceof WaypointApiError))
    return `Couldn’t load ${what}. Try again in a moment.`;
  if (err.isOffline) return offline;
  if (err.status === 401)
    return 'Your session has ended. Sign in again to load ' + what + '.';
  if (err.status === 403 || err.status === 404)
    return `Couldn’t find ${what} on your depot’s routes.`;
  return `Couldn’t load ${what}. Try again in a moment.`;
}

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
          message: loadError(
            err,
            'This stop isn’t saved on the phone yet. Open it once with signal.',
            'this stop',
          ),
        });
      });
    return () => {
      live = false;
    };
  }, [legId]);

  return state;
}

export type RoutesState =
  | { readonly status: 'loading' }
  | {
      readonly status: 'ready';
      readonly date: string;
      readonly routes: readonly DriverRoute[];
      readonly cached: boolean;
    }
  | { readonly status: 'error'; readonly message: string };

/**
 * Today's run sheet: "today" is the API clock (demo clock in demo mode), never
 * the phone's. Cached in IndexedDB; offline falls back to the saved copy.
 */
export function useRoutes(): RoutesState {
  const [state, setState] = useState<RoutesState>({ status: 'loading' });

  useEffect(() => {
    let live = true;
    const done = (s: RoutesState) => live && setState(s);
    (async () => {
      try {
        // `now` carries the Asia/Colombo offset, so its date part is the business date.
        const date = (await api.meta()).now.slice(0, 10);
        const routes = await api.getDriverRoutes(date);
        await cacheRoutes({ date, routes });
        done({ status: 'ready', date, routes, cached: false });
      } catch (err) {
        const saved = await latestCachedRoutes().catch(() => undefined);
        if (saved) return done({ status: 'ready', ...saved, cached: true });
        done({
          status: 'error',
          message: loadError(
            err,
            'Today’s route isn’t saved on this phone yet. Open the cockpit once with signal.',
            'today’s route',
          ),
        });
      }
    })();
    return () => {
      live = false;
    };
  }, []);

  return state;
}

/** Saves every stop of a route on the phone so each opens without signal. */
export function prefetchLegs(route: DriverRoute): void {
  for (const s of route.stops) {
    api.getLeg(s.legId).then(cacheLeg, () => undefined);
  }
}
