import type { Metadata } from 'next';
import { AssignmentsScreen } from './assignments-screen';

export const metadata: Metadata = { title: 'Assignments' };

export default function AssignmentsPage() {
  return <AssignmentsScreen />;
}
