import type { Metadata } from 'next';
import { FleetScreen } from './fleet-screen';

export const metadata: Metadata = { title: 'Fleet' };

export default function FleetPage() {
  return <FleetScreen />;
}
