import { WaypointClient } from '@waypoint/api-client';

/**
 * The API base URL is compiled into the client bundle at build time, which is
 * why it is NEXT_PUBLIC_ and why changing it needs a rebuild rather than a
 * restart. See .env.example.
 */
export const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_BASE_URL ?? 'http://localhost:8080/api/v1';

const TOKEN_STORAGE_KEY = 'waypoint.token';

/**
 * Where the access token lives.
 *
 * sessionStorage, not localStorage: the Loader's dock tablet is shared between
 * shifts, so a token must not outlive the browser session on a device several
 * people use. Reads are guarded because this module is imported by server
 * components too, where there is no window.
 */
export const tokenStore = {
  get(): string | null {
    if (typeof window === 'undefined') return null;
    try {
      return window.sessionStorage.getItem(TOKEN_STORAGE_KEY);
    } catch {
      // Private mode or blocked storage. Treated as signed out.
      return null;
    }
  },

  set(token: string): void {
    if (typeof window === 'undefined') return;
    try {
      window.sessionStorage.setItem(TOKEN_STORAGE_KEY, token);
    } catch {
      /* ignore — the session simply will not persist across a reload */
    }
  },

  clear(): void {
    if (typeof window === 'undefined') return;
    try {
      window.sessionStorage.removeItem(TOKEN_STORAGE_KEY);
    } catch {
      /* ignore */
    }
  },
};

/** The client every browser-side screen uses. */
export const api = new WaypointClient({
  baseUrl: API_BASE_URL,
  getToken: () => tokenStore.get(),
  onUnauthorized: () => {
    tokenStore.clear();
    if (typeof window !== 'undefined') {
      window.location.assign('/signin');
    }
  },
});

/**
 * A client for server components and route handlers, carrying a token taken
 * from the request rather than from storage.
 */
export function serverClient(token: string | null): WaypointClient {
  return new WaypointClient({
    baseUrl: process.env.API_INTERNAL_URL ?? API_BASE_URL,
    getToken: () => token,
  });
}
