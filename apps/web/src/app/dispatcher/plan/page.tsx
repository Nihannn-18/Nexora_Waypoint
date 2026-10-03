import type { Metadata } from 'next';
import { Suspense } from 'react';
import { LoadingState } from '../../../components/states';
import { PlanScreen } from './plan-screen';

export const metadata: Metadata = { title: 'Plan & allocate' };

export default function PlanPage() {
  return (
    <Suspense fallback={<LoadingState />}>
      <PlanScreen />
    </Suspense>
  );
}
