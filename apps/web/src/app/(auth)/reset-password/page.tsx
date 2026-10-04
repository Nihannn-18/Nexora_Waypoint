import type { Metadata } from 'next';
import { Suspense } from 'react';
import ResetPasswordClient from './reset-password-client';

export const metadata: Metadata = { title: 'Reset password' };

export default function ResetPasswordPage() {
  // useSearchParams needs a Suspense boundary during prerendering.
  return (
    <Suspense fallback={null}>
      <ResetPasswordClient />
    </Suspense>
  );
}
