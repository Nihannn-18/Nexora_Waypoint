import type { Metadata } from 'next';
import { DispatcherShell } from './_components/dispatcher-shell';

export const metadata: Metadata = { title: 'Dispatch Control' };

/**
 * Dispatcher workspace. Every screen inside shares the API clock, the depot
 * being planned and the delivery day (see DispatcherScopeProvider).
 */
export default function DispatcherLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return <DispatcherShell>{children}</DispatcherShell>;
}
