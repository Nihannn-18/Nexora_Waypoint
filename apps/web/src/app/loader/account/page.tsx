'use client';

import { useRouter } from 'next/navigation';
import { useEffect, useState } from 'react';
import type { AuthenticatedUser } from '@waypoint/shared-types';
import { Mono } from '@waypoint/ui';
import { api, tokenStore } from '../../../lib/api';
import {
  Card,
  ErrorState,
  Eyebrow,
  LoadingState,
  buttonClass,
} from '../_components/ui';
import { readableError } from '../_lib/use-loading';

/**
 * Who is signed in on this dock tablet, and how to hand it over.
 *
 * The tablet is shared between shifts, which is why signing out matters enough
 * to have a screen: the token lives in sessionStorage and the next loader must
 * be able to clear it deliberately rather than by closing the browser.
 */
export default function LoaderAccountPage() {
  const router = useRouter();
  const [user, setUser] = useState<AuthenticatedUser | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let live = true;
    api
      .me()
      .then((u) => live && setUser(u))
      .catch((e) => live && setError(readableError(e, 'Your account')));
    return () => {
      live = false;
    };
  }, []);

  const signOut = async () => {
    try {
      await api.logout();
    } catch {
      // The session is being abandoned either way; clearing the local token is
      // what actually ends it on this device.
    }
    tokenStore.clear();
    router.push('/signin');
  };

  if (error) {
    return <ErrorState title="Couldn't load your account" message={error} />;
  }
  if (!user) {
    return <LoadingState label="Loading your account…" />;
  }

  return (
    <div className="flex flex-col gap-3">
      <header className="flex flex-col gap-1 px-1 pt-2">
        <Eyebrow>Account</Eyebrow>
        <h1 className="text-xl font-semibold text-ink">{user.name}</h1>
      </header>

      <Card>
        <Row label="Role" value="Loader" />
        <Row label="Email" value={user.email} />
        <Row label="Depot" value={user.depotId ?? 'Not assigned'} mono />
        <p className="text-xs text-ink-muted">
          Your depot decides which routes you can load. The server applies it to
          every request, so another depot&rsquo;s dock is never visible here.
        </p>
      </Card>

      <button type="button" onClick={signOut} className={buttonClass('outline')}>
        Sign out
      </button>
    </div>
  );
}

function Row({
  label,
  value,
  mono = false,
}: {
  label: string;
  value: string;
  mono?: boolean;
}) {
  return (
    <div className="flex items-baseline justify-between gap-3 border-b border-line pb-2 last:border-0 last:pb-0">
      <span className="text-sm text-ink-muted">{label}</span>
      <span className="text-sm font-medium text-ink">
        {mono ? <Mono>{value}</Mono> : value}
      </span>
    </div>
  );
}
