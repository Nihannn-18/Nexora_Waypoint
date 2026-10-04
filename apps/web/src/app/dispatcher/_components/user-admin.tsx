'use client';

import type { ReactNode } from 'react';
import { Mono, type StatusTone } from '@waypoint/ui';
import {
  CREATABLE_ROLES,
  type CreatableRole,
  type ManagedUser,
  type Role,
} from '@waypoint/shared-types';
import { Fact } from './ui';

/**
 * Shared Dispatcher account-management presentation. An account's credential is
 * never shown: the detail screen shows a masked placeholder and a pointer to the
 * self-service reset flow, never a "view password" affordance.
 */

/** Human-readable label for a role. */
export function roleLabel(role: Role | string): string {
  switch (role) {
    case 'DISPATCHER':
      return 'Dispatcher';
    case 'DRIVER':
      return 'Driver';
    case 'LOADER':
      return 'Loader';
    case 'STORE_MANAGER':
      return 'Store manager';
    default:
      return String(role);
  }
}

/** Neutral tone for a role chip; the word is always shown alongside. */
export function roleTone(role: Role | string): StatusTone {
  switch (role) {
    case 'DISPATCHER':
      return 'info';
    case 'DRIVER':
      return 'neutral';
    case 'LOADER':
      return 'neutral';
    case 'STORE_MANAGER':
      return 'neutral';
    default:
      return 'neutral';
  }
}

/** Active/inactive tone. The word carries the meaning, not the colour. */
export function accountStatusTone(active: boolean): StatusTone {
  return active ? 'success' : 'warning';
}

export function accountStatusLabel(active: boolean): string {
  return active ? 'Active' : 'Deactivated';
}

/**
 * The scope line for an account: a store manager shows its outlet, a
 * driver/loader shows its depot. A dispatcher has neither.
 */
export function accountScope(
  account: Pick<ManagedUser, 'role' | 'depotId' | 'outletId'>,
): ReactNode {
  if (account.role === 'STORE_MANAGER') {
    return account.outletId ? (
      <Mono>{account.outletId}</Mono>
    ) : (
      <span className="text-ink-muted">No outlet</span>
    );
  }
  if (account.role === 'DRIVER' || account.role === 'LOADER') {
    return account.depotId ? (
      <Mono>{account.depotId}</Mono>
    ) : (
      <span className="text-ink-muted">No depot</span>
    );
  }
  return <span className="text-ink-muted">Both depots</span>;
}

/** The roles a dispatcher may pick in the create form, in a stable order. */
export { CREATABLE_ROLES };
export type { CreatableRole };

/**
 * The credential placeholder shown on an account detail screen. It is a fixed
 * mask, not a value: there is no endpoint that returns a password or hash, and
 * no "reveal" control.
 */
export function PasswordPlaceholder() {
  return (
    <Fact label="Password">
      <span className="flex flex-wrap items-center gap-2">
        <Mono className="tracking-[0.2em]">••••••••</Mono>
        <span className="text-xs text-ink-muted">
          Reset via Forgot Password
        </span>
      </span>
    </Fact>
  );
}
