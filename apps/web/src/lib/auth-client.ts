'use client';

import { createAuthClient } from 'better-auth/react';
import { tokenStore } from './api';

/**
 * The browser Better Auth client.
 *
 * It talks to the app's own `/api/auth/*` route handler (same origin), so no
 * cross-origin configuration is needed. On a successful sign-in the Bearer
 * plugin exposes the session token in the `set-auth-token` response header; we
 * hand it to the existing `tokenStore`, which the Waypoint API client already
 * sends as `Authorization: Bearer` to the Go API. There is deliberately no
 * second token store.
 */
export const authClient = createAuthClient({
  fetchOptions: {
    onSuccess: (ctx) => {
      const token = ctx.response.headers.get('set-auth-token');
      if (token) {
        tokenStore.set(token);
      }
    },
  },
});
