import type { Metadata } from 'next';
import { RoutesScreen } from './routes-screen';

export const metadata: Metadata = { title: 'Routes & trips' };

export default function RoutesPage() {
  return <RoutesScreen />;
}
