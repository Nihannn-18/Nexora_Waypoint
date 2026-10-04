import type { Metadata } from 'next';
import { Suspense } from 'react';
import { OutletDetailScreen } from './outlet-detail-screen';

export const metadata: Metadata = { title: 'Outlet' };

export default function OutletDetailPage({
  params,
}: {
  params: Promise<{ outletId: string }>;
}) {
  return (
    <Suspense fallback={null}>
      <OutletDetailScreen params={params} />
    </Suspense>
  );
}
