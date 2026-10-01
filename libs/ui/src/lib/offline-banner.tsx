'use client';

import { useEffect, useState } from 'react';

/**
 * Connectivity indicator for the Driver and Loader surfaces.
 *
 * Offline is rendered NEUTRAL, never red. Losing signal in hill country is an
 * expected condition on these routes, not a fault the driver caused — the design
 * is explicit about this (DG-B1). What matters to the driver is that work is
 * being saved and will upload, so the banner leads with that.
 */

export function useOnlineStatus(): boolean {
  // Assume online for the server render; correct it on mount. Starting at
  // "offline" would flash a banner on every page load.
  const [online, setOnline] = useState(true);

  useEffect(() => {
    const update = () => setOnline(navigator.onLine);
    update();
    window.addEventListener('online', update);
    window.addEventListener('offline', update);
    return () => {
      window.removeEventListener('online', update);
      window.removeEventListener('offline', update);
    };
  }, []);

  return online;
}

export interface OfflineBannerProps {
  /** Number of captured events still waiting to upload. */
  pendingCount?: number;
  /** Override detection, for Storybook and tests. */
  forceOffline?: boolean;
}

export function OfflineBanner({
  pendingCount = 0,
  forceOffline,
}: OfflineBannerProps) {
  const online = useOnlineStatus();
  const offline = forceOffline ?? !online;

  if (!offline && pendingCount === 0) {
    return null;
  }

  return (
    <div
      role="status"
      aria-live="polite"
      className="flex items-center gap-2 bg-offline/10 px-4 py-2.5 text-sm text-ink"
    >
      <span
        aria-hidden="true"
        className="inline-block size-2 shrink-0 rounded-pill bg-offline"
      />
      <span className="font-medium">
        {offline ? 'Working offline' : 'Uploading'}
      </span>
      <span className="text-ink-muted">
        {offline
          ? pendingCount > 0
            ? `${pendingCount} ${pendingCount === 1 ? 'entry' : 'entries'} saved on this phone — they upload when signal returns.`
            : 'Your work is saved on this phone and uploads when signal returns.'
          : `Sending ${pendingCount} saved ${pendingCount === 1 ? 'entry' : 'entries'}…`}
      </span>
    </div>
  );
}
