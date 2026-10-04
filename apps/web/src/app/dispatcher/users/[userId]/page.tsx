import type { Metadata } from 'next';
import { Suspense } from 'react';
import { UserDetailScreen } from './user-detail-screen';

export const metadata: Metadata = { title: 'User' };

export default function UserDetailPage({
  params,
}: {
  params: Promise<{ userId: string }>;
}) {
  return (
    <Suspense fallback={null}>
      <UserDetailScreen params={params} />
    </Suspense>
  );
}
