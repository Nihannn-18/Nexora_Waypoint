import type { Metadata } from 'next';
import { AuditScreen } from './audit-screen';

export const metadata: Metadata = { title: 'Audit trail' };

export default function AuditPage() {
  return <AuditScreen />;
}
