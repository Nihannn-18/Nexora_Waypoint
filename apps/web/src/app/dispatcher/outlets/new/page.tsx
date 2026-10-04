import type { Metadata } from 'next';
import { NewOutletScreen } from './new-outlet-screen';

export const metadata: Metadata = { title: 'Add outlet' };

export default function NewOutletPage() {
  return <NewOutletScreen />;
}
