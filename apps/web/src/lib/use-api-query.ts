'use client';

import { useCallback, useEffect, useRef, useState } from 'react';

export interface ApiQuery<T> {
  /** The latest data for the current key. Kept while a reload is in flight. */
  readonly data: T | undefined;
  readonly error: unknown;
  /** True until the first response for the current key arrives. */
  readonly loading: boolean;
  /** True while any request (first load, reload or poll) is in flight. */
  readonly refreshing: boolean;
  readonly reload: () => void;
}

interface State<T> {
  key: string | null;
  data?: T;
  error?: unknown;
  inFlight: boolean;
}

/**
 * Fetch through the shared API client and re-fetch when `key` changes.
 *
 * `key` names everything the request depends on (endpoint and filters); pass
 * null to skip fetching until inputs are ready. Data from a previous key is
 * never shown under a new key, so a filtered list cannot briefly display the
 * wrong filter's results. `pollMs` re-fetches in the background without
 * blanking the screen — the trip tracker polls every 30 s (D-06).
 */
export function useApiQuery<T>(
  key: string | null,
  fetcher: () => Promise<T>,
  options: { pollMs?: number } = {},
): ApiQuery<T> {
  const [state, setState] = useState<State<T>>({ key, inFlight: key !== null });
  const [nonce, setNonce] = useState(0);
  const fetcherRef = useRef(fetcher);

  useEffect(() => {
    fetcherRef.current = fetcher;
  });

  useEffect(() => {
    if (key === null) return;
    let cancelled = false;
    setState((s) =>
      s.key === key ? { ...s, inFlight: true } : { key, inFlight: true },
    );
    fetcherRef.current().then(
      (data) => {
        if (!cancelled) setState({ key, data, inFlight: false });
      },
      (error: unknown) => {
        if (!cancelled)
          setState((s) => ({
            key,
            data: s.key === key ? s.data : undefined,
            error,
            inFlight: false,
          }));
      },
    );
    return () => {
      cancelled = true;
    };
  }, [key, nonce]);

  const { pollMs } = options;
  useEffect(() => {
    if (!pollMs || key === null) return;
    const id = window.setInterval(() => setNonce((n) => n + 1), pollMs);
    return () => window.clearInterval(id);
  }, [pollMs, key]);

  const reload = useCallback(() => setNonce((n) => n + 1), []);
  const current = state.key === key;

  return {
    data: current ? state.data : undefined,
    error: current ? state.error : undefined,
    loading:
      key !== null &&
      (!current ||
        (state.inFlight &&
          state.data === undefined &&
          state.error === undefined)),
    refreshing: key !== null && (!current || state.inFlight),
    reload,
  };
}
