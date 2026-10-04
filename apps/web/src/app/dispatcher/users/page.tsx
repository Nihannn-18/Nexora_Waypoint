import type { Metadata } from 'next';
import { UsersScreen } from './users-screen';

export const metadata: Metadata = { title: 'Users' };

export default function UsersPage() {
  return <UsersScreen />;
}
