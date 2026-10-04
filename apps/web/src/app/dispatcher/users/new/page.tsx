import type { Metadata } from 'next';
import { NewUserScreen } from './new-user-screen';

export const metadata: Metadata = { title: 'Create user' };

export default function NewUserPage() {
  return <NewUserScreen />;
}
