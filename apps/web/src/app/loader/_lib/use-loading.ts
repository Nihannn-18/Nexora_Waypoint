'use client';

import { useCallback, useEffect, useState } from 'react';
import { WaypointApiError } from '@waypoint/api-client';
import type { LoaderRouteSummary, RouteLoading } from '@waypoint/api-client';
import { api } from '../../../lib/api';

/**
 * Turns a failed request into a sentence the loader can act on. Server text is
 * never shown raw: a Go error on a dock tablet tells Nadeesha nothing and may
 * leak internals. The status is what decides the wording.
 */
export function readableError(error: unknown, subject: string): string {
  if (error instanceof WaypointApiError) {
    if (error.isOffline) {
      return 'No connection to the server. Check the dock Wi-Fi and try again — nothing has been lost.';
    }
    switch (error.status) {
      case 401:
        return 'Your session has expired. Sign in again to continue.';
      case 403:
        return 'Your account is not allowed to load this route.';
      case 404:
        return `${subject} was not found, or it belongs to another depot.`;
      case 409:
        return 'This route is no longer confirmed, so it cannot be loaded. Ask the dispatcher.';
      case 422:
      case 400:
        return (
          error.fieldErrors?.[0]?.message ??
          'The counts were rejected. Check the quantities and try again.'
        );
      default:
        return 'Something went wrong on our side. Try again in a moment.';
    }
  }
  return 'Something went wrong. Try again in a moment.';
}

type State<T> =
  | { readonly status: 'loading' }
  | { readonly status: 'ready'; readonly data: T }
  | { readonly status: 'error'; readonly message: string };

/**
 * "Today" comes from the API, never the browser: the demo clock runs on a
 * seeded date in the past, so a device clock would show an empty dock.
 */
export function useBusinessDate() {
  const [date, setDate] = useState<string | null>(null);
  useEffect(() => {
    let live = true;
    api
      .meta()
      .then((m) => live && setDate(m.now.slice(0, 10)))
      .catch(() => live && setDate(null));
    return () => {
      live = false;
    };
  }, []);
  return date;
}

/** The depot's confirmed routes for the active run. */
export function useLoaderRoutes(): State<readonly LoaderRouteSummary[]> & {
  readonly date: string | null;
  readonly reload: () => void;
} {
  // The API clock is only a fallback for the empty case; the real delivery day
  // is the route date the server resolved, so the screen shows the run that
  // actually exists rather than a date taken from the device.
  const businessDate = useBusinessDate();
  const [state, setState] = useState<State<readonly LoaderRouteSummary[]>>({
    status: 'loading',
  });
  const [nonce, setNonce] = useState(0);

  useEffect(() => {
    let live = true;
    setState({ status: 'loading' });
    // No date: the server resolves the depot's active run.
    api
      .getLoaderRoutes()
      .then((routes) => live && setState({ status: 'ready', data: routes }))
      .catch(
        (e) =>
          live &&
          setState({
            status: 'error',
            message: readableError(e, 'The route list'),
          }),
      );
    return () => {
      live = false;
    };
  }, [nonce]);

  const date =
    state.status === 'ready'
      ? (state.data[0]?.routeDate ?? businessDate)
      : businessDate;

  return { ...state, date, reload: () => setNonce((n) => n + 1) };
}

/** One route's picking list, refreshed from the server after every save. */
export function useRouteLoading(routeId: string): State<RouteLoading> & {
  readonly reload: () => void;
  readonly replace: (next: RouteLoading) => void;
} {
  const [state, setState] = useState<State<RouteLoading>>({
    status: 'loading',
  });
  const [nonce, setNonce] = useState(0);

  useEffect(() => {
    let live = true;
    setState({ status: 'loading' });
    api
      .getRouteLoading(routeId)
      .then((data) => live && setState({ status: 'ready', data }))
      .catch(
        (e) =>
          live &&
          setState({ status: 'error', message: readableError(e, 'This route') }),
      );
    return () => {
      live = false;
    };
  }, [routeId, nonce]);

  const replace = useCallback(
    (next: RouteLoading) => setState({ status: 'ready', data: next }),
    [],
  );

  return { ...state, reload: () => setNonce((n) => n + 1), replace };
}
