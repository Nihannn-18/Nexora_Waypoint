import type { Metadata } from 'next';
import { StoreShell } from './_components/store-shell';

export const metadata: Metadata = { title: 'My store' };

/**
 * Store manager workspace. Every screen shares the authenticated outlet and
 * the API clock through StoreShell, so the identity shown in the header is
 * always the caller's own outlet.
 */
export default function StoreLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return <StoreShell>{children}</StoreShell>;
}
