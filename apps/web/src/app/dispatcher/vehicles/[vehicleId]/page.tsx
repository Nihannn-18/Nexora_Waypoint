import type { Metadata } from 'next';
import { Suspense } from 'react';
import { VehicleDetailScreen } from './vehicle-detail-screen';

export const metadata: Metadata = { title: 'Vehicle' };

export default function VehicleDetailPage({
  params,
}: {
  params: Promise<{ vehicleId: string }>;
}) {
  return (
    <Suspense fallback={null}>
      <VehicleDetailScreen params={params} />
    </Suspense>
  );
}
