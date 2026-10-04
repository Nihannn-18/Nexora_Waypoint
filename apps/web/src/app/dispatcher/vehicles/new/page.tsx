import type { Metadata } from 'next';
import { NewVehicleScreen } from './new-vehicle-screen';

export const metadata: Metadata = { title: 'Add vehicle' };

export default function NewVehiclePage() {
  return <NewVehicleScreen />;
}
