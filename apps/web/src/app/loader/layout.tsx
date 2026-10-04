import type { Metadata } from 'next';
import { OfflineBanner } from '@waypoint/ui';
import { LoaderTabs } from './_components/ui';

export const metadata: Metadata = { title: 'Loading' };

/**
 * Loader shell — a phone at 402 and a shared dock tablet at 1024, where the
 * design switches to master–detail (L-T1…L-T3). Bottom tabs because Nadeesha
 * works one-handed with the other hand on a trolley.
 *
 * The depot in the header comes from the signed-in loader, not a constant: the
 * same build serves both depots and the server scopes every read to the
 * caller's own dock.
 */
export default function LoaderLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <div className="flex min-h-dvh flex-col bg-page">
      <OfflineBanner />
      <main className="mx-auto w-full max-w-5xl flex-1 p-3 pb-24">
        {children}
      </main>
      <LoaderTabs />
    </div>
  );
}
