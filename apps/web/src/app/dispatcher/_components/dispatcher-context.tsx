'use client';

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';
import type { Depot, IsoDate, MetaResponse } from '@waypoint/shared-types';
import { api } from '../../../lib/api';
import { addDays, businessDate } from '../../../lib/format';
import { useApiQuery } from '../../../lib/use-api-query';

/**
 * Everything every dispatcher screen shares: the API clock, the depot being
 * planned and the delivery day in focus.
 *
 * "Now" is the API's clock, never the browser's — the seeded demo day is in the
 * past (CLAUDE.md rule 8). The dispatcher plans both depots from Peliyagoda, so
 * the depot is a choice, defaulting to Peliyagoda.
 */
interface DispatcherScope {
  readonly meta: MetaResponse | undefined;
  /** API clock minus browser clock, measured at the last /meta sync. */
  readonly clockOffsetMs: number | undefined;
  /** Today on the API clock, `YYYY-MM-DD`. */
  readonly today: IsoDate | undefined;
  readonly depots: readonly Depot[] | undefined;
  readonly depotsError: unknown;
  readonly reloadDepots: () => void;
  readonly depot: Depot | undefined;
  readonly setDepotId: (depotId: string) => void;
  /** The delivery day every screen filters by. */
  readonly deliveryDate: IsoDate | undefined;
  readonly setDeliveryDate: (date: IsoDate) => void;
  readonly unreadCount: number | undefined;
  readonly refreshUnread: () => void;
}

const Ctx = createContext<DispatcherScope | null>(null);

const DEPOT_KEY = 'waypoint.dispatcher.depot';
const DATE_KEY = 'waypoint.dispatcher.date';

function readSession(key: string): string | null {
  try {
    return window.sessionStorage.getItem(key);
  } catch {
    return null;
  }
}

function writeSession(key: string, value: string) {
  try {
    window.sessionStorage.setItem(key, value);
  } catch {
    /* a convenience only; the screen works without it */
  }
}

/** Re-sync with the API clock this often, to pick up demo-clock jumps. */
const META_SYNC_MS = 60_000;
const UNREAD_POLL_MS = 30_000;

export function DispatcherScopeProvider({ children }: { children: ReactNode }) {
  const meta = useApiQuery('meta', () => api.meta(), { pollMs: META_SYNC_MS });
  const depots = useApiQuery('depots', () => api.getDepots());
  const unread = useApiQuery('unread', () => api.getUnreadNotificationCount(), {
    pollMs: UNREAD_POLL_MS,
  });

  // Offset between the API clock and the browser clock, measured when /meta
  // answers, so a countdown can tick locally (useApiNow) without polling.
  const [offsetMs, setOffsetMs] = useState<number | undefined>(undefined);
  useEffect(() => {
    if (meta.data) setOffsetMs(Date.parse(meta.data.now) - Date.now());
  }, [meta.data]);
  const today = meta.data ? businessDate(new Date(meta.data.now)) : undefined;

  const [depotId, setDepotIdState] = useState<string | null>(null);
  const [deliveryDate, setDateState] = useState<IsoDate | undefined>(undefined);

  useEffect(() => {
    setDepotIdState(readSession(DEPOT_KEY));
    const saved = readSession(DATE_KEY);
    if (saved && /^\d{4}-\d{2}-\d{2}$/.test(saved)) setDateState(saved);
  }, []);

  // Default the delivery day to tomorrow on the API clock: orders placed today
  // before the 16:00 cutoff are for the next day. The planner, not this
  // default, decides whether that day is an operating day.
  useEffect(() => {
    if (!deliveryDate && today) setDateState(addDays(today, 1));
  }, [deliveryDate, today]);

  const depot = useMemo(() => {
    const list = depots.data;
    if (!list || list.length === 0) return undefined;
    return (
      list.find((d) => d.depotId === depotId) ??
      list.find((d) => d.code === 'PELIYAGODA') ??
      list[0]
    );
  }, [depots.data, depotId]);

  const setDepotId = useCallback((id: string) => {
    setDepotIdState(id);
    writeSession(DEPOT_KEY, id);
  }, []);
  const setDeliveryDate = useCallback((date: IsoDate) => {
    setDateState(date);
    writeSession(DATE_KEY, date);
  }, []);

  const value: DispatcherScope = {
    meta: meta.data,
    clockOffsetMs: offsetMs,
    today,
    depots: depots.data,
    depotsError: depots.error,
    reloadDepots: depots.reload,
    depot,
    setDepotId,
    deliveryDate,
    setDeliveryDate,
    unreadCount: unread.data,
    refreshUnread: unread.reload,
  };

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useDispatcherScope(): DispatcherScope {
  const scope = useContext(Ctx);
  if (!scope) {
    throw new Error(
      'useDispatcherScope must be used inside DispatcherScopeProvider',
    );
  }
  return scope;
}

/**
 * The API clock's current instant, re-rendered every `intervalMs`. Only the
 * components that show a live time use this, so the rest of the workspace does
 * not re-render every second.
 */
export function useApiNow(intervalMs = 1000): Date | undefined {
  const { clockOffsetMs } = useDispatcherScope();
  const [now, setNow] = useState<Date | undefined>(undefined);
  useEffect(() => {
    if (clockOffsetMs === undefined) return;
    const update = () => setNow(new Date(Date.now() + clockOffsetMs));
    update();
    const id = window.setInterval(update, intervalMs);
    return () => window.clearInterval(id);
  }, [clockOffsetMs, intervalMs]);
  return now;
}
