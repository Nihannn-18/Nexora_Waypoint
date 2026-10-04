'use client';

import { useState, type FormEvent } from 'react';
import { useRouter } from 'next/navigation';
import type {
  Depot,
  ManagedUser,
  Outlet,
  UpdateUserRequest,
} from '@waypoint/shared-types';
import { WaypointApiError } from '@waypoint/api-client';
import { api } from '../../../../lib/api';
import { Card, SectionHeading } from '../../_components/ui';
import {
  Field,
  FormActions,
  inputClass,
} from '../../_components/master-data';
import { applyFieldErrors } from '../../vehicles/_components/vehicle-form';
import {
  CREATABLE_ROLES,
  roleLabel,
  type CreatableRole,
} from '../../_components/user-admin';

interface Draft {
  displayName: string;
  email: string;
  role: CreatableRole;
  depotId: string;
  outletId: string;
  password: string;
  confirmPassword: string;
}

function draftFrom(user: ManagedUser | undefined, depots: readonly Depot[]): Draft {
  return {
    displayName: user?.displayName ?? '',
    email: user?.email ?? '',
    role: (user?.role as CreatableRole | undefined) ?? 'DRIVER',
    depotId: user?.depotId ?? depots[0]?.depotId ?? '',
    outletId: user?.outletId ?? '',
    password: '',
    confirmPassword: '',
  };
}

/**
 * The account create/edit form. Role, identity and password rules are all
 * enforced server-side; the browser only gives a fast first signal. On create,
 * the role decides which assignment field is shown: a driver/loader needs a
 * depot, a store manager needs an outlet. The other field is never sent, so a
 * stale value cannot leak into the wrong role's account.
 */
export function UserForm({
  user,
  depots,
  outlets,
}: {
  user?: ManagedUser;
  depots: readonly Depot[];
  outlets: readonly Outlet[];
}) {
  const router = useRouter();
  const editing = user !== undefined;
  const [draft, setDraft] = useState<Draft>(() => draftFrom(user, depots));
  const [submitting, setSubmitting] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState<string | null>(null);
  const [showPassword, setShowPassword] = useState(false);

  const set = (key: keyof Draft) => (value: string) => {
    setDraft((d) => ({ ...d, [key]: value }));
    setErrors((e) => ({ ...e, [key]: '' }));
  };

  function localErrors(): Record<string, string> {
    const e: Record<string, string> = {};
    if (!draft.displayName.trim()) e.displayName = 'Name is required';
    if (!editing && !draft.email.trim()) e.email = 'Email is required';
    if (draft.role === 'STORE_MANAGER') {
      if (!draft.outletId) e.outletId = 'Outlet is required for a store manager';
    } else if (!draft.depotId) {
      e.depotId = 'Depot is required for this role';
    }
    if (!editing) {
      if (draft.password.length < 8)
        e.password = 'Password must be at least 8 characters';
      if (draft.password !== draft.confirmPassword)
        e.confirmPassword = 'Passwords do not match';
    }
    return e;
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const local = localErrors();
    if (Object.keys(local).length > 0) {
      setErrors(local);
      setFormError('Check the highlighted fields.');
      return;
    }
    setSubmitting(true);
    setFormError(null);
    setErrors({});

    try {
      if (editing) {
        // Only permitted fields; the depot is derived from the outlet for a
        // store manager server-side.
        const body: UpdateUserRequest =
          draft.role === 'STORE_MANAGER'
            ? { displayName: draft.displayName.trim(), outletId: draft.outletId }
            : {
                displayName: draft.displayName.trim(),
                depotId: draft.depotId,
              };
        const saved = await api.updateUser(user.userId, body);
        router.push(`/dispatcher/users/${saved.userId}?saved=1`);
        return;
      }

      const saved = await api.createUser({
        email: draft.email.trim(),
        displayName: draft.displayName.trim(),
        role: draft.role,
        // Send only the role-appropriate assignment field.
        ...(draft.role === 'STORE_MANAGER'
          ? { outletId: draft.outletId }
          : { depotId: draft.depotId }),
        initialPassword: draft.password,
      });
      router.push(`/dispatcher/users/${saved.userId}?saved=1`);
    } catch (err) {
      setFormError(readableUserError(err));
      applyFieldErrors(err, setErrors);
      setSubmitting(false);
    }
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-4">
      <SectionHeading>
        {editing ? `Edit ${user.displayName || user.email}` : 'Create user'}
      </SectionHeading>

      {formError && (
        <Card className="border border-error bg-error/10">
          <p role="alert" className="text-sm text-ink">
            <strong className="font-semibold">Could not save.</strong>{' '}
            {formError}
          </p>
        </Card>
      )}

      <Card className="flex flex-col gap-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <Field
            label="Name"
            htmlFor="displayName"
            required
            error={errors.displayName}
          >
            <input
              id="displayName"
              type="text"
              value={draft.displayName}
              onChange={(e) => set('displayName')(e.target.value)}
              className={inputClass(Boolean(errors.displayName))}
            />
          </Field>

          <Field label="Email" htmlFor="email" required error={errors.email}>
            <input
              id="email"
              type="email"
              value={draft.email}
              disabled={editing}
              onChange={(e) => set('email')(e.target.value)}
              className={inputClass(Boolean(errors.email))}
            />
          </Field>

          <Field
            label="Role"
            htmlFor="role"
            required
            error={errors.role}
            hint={
              editing
                ? 'A role cannot be changed after creation.'
                : 'Operational roles only. A dispatcher account is not created here.'
            }
          >
            <select
              id="role"
              value={draft.role}
              disabled={editing}
              onChange={(e) => set('role')(e.target.value)}
              className={inputClass(Boolean(errors.role))}
            >
              {CREATABLE_ROLES.map((r) => (
                <option key={r} value={r}>
                  {roleLabel(r)}
                </option>
              ))}
            </select>
          </Field>

          {/* Role-specific assignment: a store manager gets an outlet; a
              driver/loader gets a depot. The other is never rendered. */}
          {draft.role === 'STORE_MANAGER' ? (
            <Field
              label="Outlet"
              htmlFor="outletId"
              required
              error={errors.outletId}
              hint="The store manager is scoped to this outlet and its depot."
            >
              <select
                id="outletId"
                value={draft.outletId}
                onChange={(e) => set('outletId')(e.target.value)}
                className={inputClass(Boolean(errors.outletId))}
              >
                <option value="">Select an outlet…</option>
                {outlets.map((o) => (
                  <option key={o.outletId} value={o.outletId}>
                    {o.outletId} · {o.name}
                  </option>
                ))}
              </select>
            </Field>
          ) : (
            <Field
              label="Depot"
              htmlFor="depotId"
              required
              error={errors.depotId}
              hint="The driver/loader is scoped to this depot."
            >
              <select
                id="depotId"
                value={draft.depotId}
                onChange={(e) => set('depotId')(e.target.value)}
                className={inputClass(Boolean(errors.depotId))}
              >
                {depots.map((d) => (
                  <option key={d.depotId} value={d.depotId}>
                    {d.name}
                  </option>
                ))}
              </select>
            </Field>
          )}

          {!editing && (
            <>
              <Field
                label="Initial password"
                htmlFor="password"
                required
                error={errors.password}
                hint="At least 8 characters. The user should reset it after first sign-in."
              >
                <div className="flex items-center gap-2">
                  <input
                    id="password"
                    type={showPassword ? 'text' : 'password'}
                    autoComplete="new-password"
                    value={draft.password}
                    onChange={(e) => set('password')(e.target.value)}
                    className={inputClass(Boolean(errors.password))}
                  />
                  <button
                    type="button"
                    aria-pressed={showPassword}
                    onClick={() => setShowPassword((v) => !v)}
                    className="tap-target shrink-0 rounded-control bg-card px-3 text-sm font-medium text-ink ring-1 ring-ink/15 hover:bg-page"
                  >
                    {showPassword ? 'Hide' : 'Show'}
                  </button>
                </div>
              </Field>

              <Field
                label="Confirm password"
                htmlFor="confirmPassword"
                required
                error={errors.confirmPassword}
              >
                <input
                  id="confirmPassword"
                  type={showPassword ? 'text' : 'password'}
                  autoComplete="new-password"
                  value={draft.confirmPassword}
                  onChange={(e) => set('confirmPassword')(e.target.value)}
                  className={inputClass(Boolean(errors.confirmPassword))}
                />
              </Field>
            </>
          )}
        </div>

        <FormActions
          submitting={submitting}
          submitLabel={editing ? 'Save changes' : 'Create account'}
          onCancel={() => router.push('/dispatcher/users')}
        />
      </Card>
    </form>
  );
}

/**
 * Maps an account-management error to a sentence. A 409 here is a duplicate
 * email, which is the one conflict this form produces.
 */
export function readableUserError(error: unknown): string {
  if (error instanceof WaypointApiError) {
    if (error.isOffline) {
      return 'No connection to the server. Try again — nothing has been saved.';
    }
    switch (error.status) {
      case 400:
      case 422:
        return (
          error.fieldErrors?.[0]?.message ??
          'The account was rejected. Check the values and try again.'
        );
      case 403:
        return 'Only a dispatcher may manage accounts.';
      case 409:
        return 'An account with that email already exists.';
      case 404:
        return 'That account was not found.';
      default:
        return 'Something went wrong on our side. Try again in a moment.';
    }
  }
  return 'Something went wrong. Try again in a moment.';
}
