'use client';

import { useEffect } from 'react';

/** Production only: a dev service worker would serve stale HMR bundles. */
export function RegisterServiceWorker() {
  useEffect(() => {
    if (process.env.NODE_ENV === 'production' && 'serviceWorker' in navigator) {
      navigator.serviceWorker.register('/sw.js').catch(() => undefined);
    }
  }, []);
  return null;
}
