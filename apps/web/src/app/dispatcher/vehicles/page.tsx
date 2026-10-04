import type { Metadata } from 'next';
import { VehiclesScreen } from './vehicles-screen';

export const metadata: Metadata = { title: 'Vehicles' };

export default function VehiclesPage() {
  return <VehiclesScreen />;
}
