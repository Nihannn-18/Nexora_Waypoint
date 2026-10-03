import type { Metadata } from 'next';
import { DriverShell } from './_components/ui';
import { RegisterServiceWorker } from './_components/register-sw';

export const metadata: Metadata = { title: 'Run sheet' };

/**
 * Driver shell — phone at 402, used only while safely stopped. Figma
 * "04 · Driver — phone": Mobile App Bar, Sync Status Bar, Mobile Tab Bar.
 */
export default function DriverLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <>
      <RegisterServiceWorker />
      <DriverShell>{children}</DriverShell>
    </>
  );
}
