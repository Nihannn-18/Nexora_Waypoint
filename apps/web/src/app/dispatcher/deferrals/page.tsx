import type { Metadata } from 'next';
import { DeferralsScreen } from './deferrals-screen';

export const metadata: Metadata = { title: 'Deferral log' };

export default function DeferralsPage() {
  return <DeferralsScreen />;
}
