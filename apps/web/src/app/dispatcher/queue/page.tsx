import type { Metadata } from 'next';
import { Suspense } from 'react';
import { LoadingState } from '../../../components/states';
import { OrdersScreen } from './orders-screen';

export const metadata: Metadata = { title: 'Orders' };

export default function OrdersPage() {
  return (
    <Suspense fallback={<LoadingState />}>
      <OrdersScreen />
    </Suspense>
  );
}
