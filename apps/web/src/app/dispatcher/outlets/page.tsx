import type { Metadata } from 'next';
import { OutletsScreen } from './outlets-screen';

export const metadata: Metadata = { title: 'Outlets' };

export default function OutletsPage() {
  return <OutletsScreen />;
}
