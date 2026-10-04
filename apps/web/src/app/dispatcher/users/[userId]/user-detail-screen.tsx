'use client';

import Link from 'next/link';
import { use, useState } from 'react';
import { useSearchParams } from 'next/navigation';
import { Mono, StatusBadge } from '@waypoint/ui';
import type { ManagedUser } from '@waypoint/shared-types';
import { api } from '../../../../lib/api';
import { formatInstantDay } from '../../../../lib/format';
import { useApiQuery } from '../../../../lib/use-api-query';
import { ErrorState, LoadingState } from '../../../../components/states';
import { Card, Fact, PageHeader, SectionHeading } from '../../_components/ui';
import {
  PasswordPlaceholder,
  accountScope,
  accountStatusLabel,
  accountStatusTone,
  roleLabel,
  roleTone,
} from '../../_components/user-admin';
import { SavedNotice } from '../../_components/master-data';
import { ConfirmationDialog } from '../../_components/confirmation-dialog';
import { UserForm } from '../_components/user-form';

/**
 * Dispatcher · User detail. Shows the account's profile and assignment, offers
 * edit of the permitted fields, and deactivate/reactivate. The credential is
 * never retrievable: the password row is a fixed mask pointing at the
 * self-service reset flow.
 */
export function UserDetailScreen({
  params,
}: {
  params: Promise<{ userId: string }>;
}) {
  const { userId } = use(params);
  const search = useSearchParams();
  const saved = search.get('saved') === '1';

  const [editing, setEditing] = useState(false);
  const [confirm, setConfirm] = useState(false);
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);

  const user = useApiQuery(`managed-user:${userId}`, () => api.getUser(userId));
  const depots = useApiQuery('depots', () => api.getDepots());
  const outlets = useApiQuery('all-outlets', () => api.getOutlets());

  if (user.loading && !user.data) {
    return <LoadingState label="Loading account…" />;
  }
  if (user.error && !user.data) {
    return <ErrorState error={user.error} onRetry={user.reload} />;
  }
  const u = user.data as ManagedUser;
  if (!u) {
    return (
      <ErrorState
        error={new Error('This account was not found.')}
        onRetry={user.reload}
      />
    );
  }

  if (editing && depots.data && outlets.data) {
    return (
      <>
        <PageHeader
          title={`Edit ${u.displayName || u.email}`}
          description="Role and password are not editable here; role is immutable and passwords change only through the reset flow."
          actions={
            <button
              type="button"
              onClick={() => setEditing(false)}
              className="tap-target rounded-control bg-card px-4 text-sm font-medium text-ink ring-1 ring-ink/15 hover:bg-page"
            >
              Cancel
            </button>
          }
        />
        <UserForm user={u} depots={depots.data} outlets={outlets.data} />
      </>
    );
  }

  async function toggleActive() {
    setBusy(true);
    setActionError(null);
    try {
      if (u.active) {
        await api.deactivateUser(u.userId);
      } else {
        await api.activateUser(u.userId);
      }
      setConfirm(false);
      user.reload();
    } catch {
      setActionError(
        'The change could not be saved. Check the connection and try again.',
      );
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <div className="mb-2">
        <Link href="/dispatcher/users" className="text-sm font-medium text-link">
          ← All users
        </Link>
      </div>
      <PageHeader
        title={u.displayName || u.email}
        description={roleLabel(u.role)}
        actions={
          <div className="flex flex-wrap gap-2">
            <button
              type="button"
              onClick={() => setEditing(true)}
              className="tap-target rounded-control bg-action px-4 text-sm font-semibold text-card hover:bg-action/90"
            >
              Edit
            </button>
            {u.active ? (
              <button
                type="button"
                onClick={() => setConfirm(true)}
                className="tap-target rounded-control bg-card px-4 text-sm font-medium text-error ring-1 ring-error/30 hover:bg-error/5"
              >
                Deactivate
              </button>
            ) : (
              <button
                type="button"
                onClick={() => setConfirm(true)}
                className="tap-target rounded-control bg-card px-4 text-sm font-medium text-ink ring-1 ring-ink/15 hover:bg-page"
              >
                Reactivate
              </button>
            )}
          </div>
        }
      />

      {saved && (
        <div className="mb-4">
          <SavedNotice>Account saved.</SavedNotice>
        </div>
      )}

      {actionError && (
        <p role="alert" className="mb-4 rounded-control bg-error/10 p-3 text-sm text-ink">
          {actionError}
        </p>
      )}

      <SectionHeading>Account</SectionHeading>
      <Card>
        <dl className="grid grid-cols-2 gap-4 sm:grid-cols-3">
          <Fact label="Name">{u.displayName || '—'}</Fact>
          <Fact label="Email">
            <Mono>{u.email}</Mono>
          </Fact>
          <Fact label="Role">
            <StatusBadge tone={roleTone(u.role)} glyph={false}>
              {roleLabel(u.role)}
            </StatusBadge>
          </Fact>
          <Fact label={u.role === 'STORE_MANAGER' ? 'Outlet' : 'Depot'}>
            {accountScope(u)}
          </Fact>
          <Fact label="Status">
            <StatusBadge tone={accountStatusTone(u.active)}>
              {accountStatusLabel(u.active)}
            </StatusBadge>
          </Fact>
          <Fact label="Created">
            <Mono>{u.createdAt ? formatInstantDay(u.createdAt) : '—'}</Mono>
          </Fact>
          <PasswordPlaceholder />
        </dl>
      </Card>

      <p className="mt-4 max-w-3xl text-xs text-ink-muted">
        Accounts are never deleted: routes, assignments, audit entries and
        delivery history reference them. Deactivating ends the account's
        sessions so a signed-in device cannot keep using it; the record and its
        history remain.
      </p>

      <ConfirmationDialog
        open={confirm}
        title={u.active ? 'Deactivate this account?' : 'Reactivate this account?'}
        eyebrow="Account access"
        confirmLabel={u.active ? 'Yes, deactivate' : 'Yes, reactivate'}
        busyLabel="Saving…"
        busy={busy}
        onConfirm={() => void toggleActive()}
        onCancel={() => setConfirm(false)}
      >
        {u.active ? (
          <p>
            <strong className="font-semibold">
              {u.displayName || u.email}
            </strong>{' '}
            will not be able to sign in, and any signed-in session on their
            device is ended. Their history — routes, deliveries and audit —
            stays intact, and the account can be reactivated later.
          </p>
        ) : (
          <p>
            <strong className="font-semibold">
              {u.displayName || u.email}
            </strong>{' '}
            will be able to sign in again with their existing password.
          </p>
        )}
      </ConfirmationDialog>
    </>
  );
}
