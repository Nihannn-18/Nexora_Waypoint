'use client';

import Link from 'next/link';
import { useMemo, useState } from 'react';
import { Mono, StatusBadge } from '@waypoint/ui';
import type { ManagedUser, Role } from '@waypoint/shared-types';
import { api } from '../../../lib/api';
import { useApiQuery } from '../../../lib/use-api-query';
import {
  EmptyState,
  ErrorState,
  LoadingState,
} from '../../../components/states';
import { PageHeader } from '../_components/ui';
import {
  accountScope,
  accountStatusLabel,
  accountStatusTone,
  roleLabel,
  roleTone,
} from '../_components/user-admin';

type RoleFilter = 'ALL' | 'DRIVER' | 'LOADER' | 'STORE_MANAGER';
type ActiveFilter = 'ALL' | 'ACTIVE' | 'INACTIVE';

const ROLE_FILTERS: { key: RoleFilter; label: string }[] = [
  { key: 'ALL', label: 'All' },
  { key: 'DRIVER', label: 'Drivers' },
  { key: 'LOADER', label: 'Loaders' },
  { key: 'STORE_MANAGER', label: 'Store managers' },
];

const ACTIVE_FILTERS: { key: ActiveFilter; label: string }[] = [
  { key: 'ALL', label: 'Any status' },
  { key: 'ACTIVE', label: 'Active' },
  { key: 'INACTIVE', label: 'Deactivated' },
];

/**
 * Dispatcher · Users. Operational accounts only: drivers, loaders and store
 * managers. The dispatcher's own account is not listed here — it is not created
 * or edited through this surface, and a dispatcher may not create another.
 */
export function UsersScreen() {
  const [role, setRole] = useState<RoleFilter>('ALL');
  const [active, setActive] = useState<ActiveFilter>('ALL');
  const [search, setSearch] = useState('');

  const users = useApiQuery('managed-users', () => api.listUsers());

  const all = users.data ?? [];
  const term = search.trim().toLowerCase();
  const shown = useMemo(
    () =>
      all
        .filter((u) => role === 'ALL' || u.role === (role as Role))
        .filter((u) =>
          active === 'ALL'
            ? true
            : active === 'ACTIVE'
              ? u.active
              : !u.active,
        )
        .filter(
          (u) =>
            !term ||
            u.displayName.toLowerCase().includes(term) ||
            u.email.toLowerCase().includes(term) ||
            (u.outletId ?? '').toLowerCase().includes(term) ||
            (u.depotId ?? '').toLowerCase().includes(term),
        ),
    [all, role, active, term],
  );

  return (
    <>
      <PageHeader
        title="Users"
        description={
          <>
            Operational accounts for drivers, loaders and store managers.{' '}
            <Mono>{all.length}</Mono> accounts. A dispatcher account is not
            created here.
          </>
        }
        actions={
          <Link
            href="/dispatcher/users/new"
            className="tap-target inline-flex items-center gap-2 rounded-control bg-action px-4 text-sm font-semibold text-card hover:bg-action/90"
          >
            + Create user
          </Link>
        }
      />

      <div className="mb-3 flex flex-wrap items-center gap-2">
        <div role="group" aria-label="Filter by role" className="flex flex-wrap gap-1.5">
          {ROLE_FILTERS.map((f) => (
            <button
              key={f.key}
              type="button"
              aria-pressed={role === f.key}
              onClick={() => setRole(f.key)}
              className={`h-9 rounded-control px-3 text-xs font-medium ring-1 ${
                role === f.key
                  ? 'bg-action text-card ring-action'
                  : 'bg-card text-ink ring-ink/15 hover:bg-page'
              }`}
            >
              {f.label}
            </button>
          ))}
        </div>

        <label className="flex items-center gap-2 text-sm">
          <span className="sr-only">Filter by status</span>
          <select
            value={active}
            onChange={(e) => setActive(e.target.value as ActiveFilter)}
            className="h-9 rounded-control bg-card px-2 text-sm text-ink ring-1 ring-ink/15"
          >
            {ACTIVE_FILTERS.map((f) => (
              <option key={f.key} value={f.key}>
                {f.label}
              </option>
            ))}
          </select>
        </label>

        <label className="ml-auto flex items-center gap-2 text-sm">
          <span className="sr-only">Search users</span>
          <input
            type="search"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search name, email or assignment…"
            className="tap-target w-64 rounded-control bg-card px-3 text-sm text-ink ring-1 ring-ink/15 focus-visible:ring-2 focus-visible:ring-brand"
          />
        </label>
      </div>

      {users.loading && !all.length ? (
        <LoadingState label="Loading users…" />
      ) : users.error ? (
        <ErrorState error={users.error} onRetry={users.reload} />
      ) : shown.length === 0 ? (
        <EmptyState title="No accounts match">
          Adjust the filter or search, or create an account.
        </EmptyState>
      ) : (
        <div className="overflow-x-auto rounded-card bg-card ring-1 ring-ink/10">
          <table className="w-full min-w-[860px] text-left text-sm">
            <thead className="bg-brand/5 text-[0.6875rem] uppercase tracking-wide text-ink-muted">
              <tr>
                <th className="px-4 py-2.5 font-semibold">Name</th>
                <th className="px-4 py-2.5 font-semibold">Email</th>
                <th className="px-4 py-2.5 font-semibold">Role</th>
                <th className="px-4 py-2.5 font-semibold">Depot / Outlet</th>
                <th className="px-4 py-2.5 font-semibold">Status</th>
                <th className="px-4 py-2.5 text-right font-semibold">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-ink/5">
              {shown.map((u) => (
                <UserRow key={u.userId} user={u} />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}

function UserRow({ user }: { user: ManagedUser }) {
  return (
    <tr className="hover:bg-page">
      <td className="px-4 py-2.5">
        <Link
          href={`/dispatcher/users/${user.userId}`}
          className="font-medium text-link hover:underline"
        >
          {user.displayName || user.email}
        </Link>
      </td>
      <td className="px-4 py-2.5">
        <Mono>{user.email}</Mono>
      </td>
      <td className="px-4 py-2.5">
        <StatusBadge tone={roleTone(user.role)} glyph={false}>
          {roleLabel(user.role)}
        </StatusBadge>
      </td>
      <td className="px-4 py-2.5">{accountScope(user)}</td>
      <td className="px-4 py-2.5">
        <StatusBadge tone={accountStatusTone(user.active)}>
          {accountStatusLabel(user.active)}
        </StatusBadge>
      </td>
      <td className="px-4 py-2.5 text-right">
        <Link
          href={`/dispatcher/users/${user.userId}`}
          className="text-link hover:underline"
        >
          View
        </Link>
      </td>
    </tr>
  );
}
