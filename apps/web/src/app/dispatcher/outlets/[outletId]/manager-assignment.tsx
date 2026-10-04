'use client';

import { useEffect, useState } from 'react';
import { api } from '../../../../lib/api';
import { useApiQuery } from '../../../../lib/use-api-query';
import { Card, SectionHeading, buttonClass } from '../../_components/ui';
import {
  Field,
  SavedNotice,
  inputClass,
  readableAssignmentError,
} from '../../_components/master-data';

/**
 * Dispatcher · Outlet store-manager assignment. app_user.outlet_id is the one
 * authoritative link, so this reassigns an existing STORE_MANAGER rather than
 * creating a second relationship table. Assigning a manager to an outlet
 * releases whoever held it before, in the same transaction.
 */
export function ManagerAssignment({ outletId }: { outletId: string }) {
  const [selected, setSelected] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState<string | null>(null);
  const [confirmRemove, setConfirmRemove] = useState(false);

  const managers = useApiQuery('assign-managers', () =>
    api.listStoreManagers(),
  );
  const assignment = useApiQuery(`outlet-manager:${outletId}`, () =>
    api.getOutletManager(outletId),
  );
  const current = assignment.data ?? null;

  useEffect(() => {
    setSelected((s) => (current ? current.userId : s));
  }, [current?.userId]);

  async function assign() {
    if (!selected) {
      setError('Choose a store manager first.');
      return;
    }
    setBusy(true);
    setError(null);
    setSaved(null);
    try {
      await api.assignManager(outletId, { userId: selected });
      setSaved('Store manager assigned.');
      assignment.reload();
      void managers.reload();
    } catch (err) {
      setError(readableAssignmentError(err));
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    setBusy(true);
    setError(null);
    setSaved(null);
    try {
      await api.unassignManager(outletId);
      setSaved('Store manager unassigned.');
      setConfirmRemove(false);
      assignment.reload();
      void managers.reload();
    } catch (err) {
      setError(readableAssignmentError(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <SectionHeading>Store manager</SectionHeading>
      <Card className="flex flex-col gap-4">
        {saved && <SavedNotice>{saved}</SavedNotice>}
        {error && (
          <p role="alert" className="text-sm text-error-strong">
            {error}
          </p>
        )}

        <Field
          label="Assigned store manager"
          htmlFor="outlet-manager"
          required
          hint="Only active store managers are listed. Assigning releases the previous manager."
        >
          <select
            id="outlet-manager"
            value={selected}
            onChange={(e) => setSelected(e.target.value)}
            className={inputClass()}
            disabled={managers.loading}
          >
            <option value="">— none selected —</option>
            {(managers.data ?? []).map((m) => (
              <option key={m.userId} value={m.userId}>
                {m.name} · {m.email}
                {m.outletId && m.outletId !== outletId
                  ? ` (currently ${m.outletId})`
                  : ''}
              </option>
            ))}
          </select>
        </Field>

        <div className="rounded-control bg-page px-3 py-2 text-sm">
          {assignment.loading ? (
            <span className="text-ink-muted">Checking the assignment…</span>
          ) : current ? (
            <span className="text-ink">
              <strong className="font-semibold">{current.name}</strong>{' '}
              <span className="text-ink-muted">{current.email}</span> manages
              this outlet.
            </span>
          ) : (
            <span className="text-ink-muted">
              No store manager is assigned to this outlet.
            </span>
          )}
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <button
            type="button"
            onClick={assign}
            disabled={busy || !selected}
            aria-busy={busy}
            className={buttonClass.primary}
          >
            {busy ? 'Saving…' : current ? 'Change manager' : 'Assign manager'}
          </button>
          {current &&
            (confirmRemove ? (
              <>
                <button
                  type="button"
                  onClick={remove}
                  disabled={busy}
                  className="tap-target inline-flex items-center justify-center gap-2 rounded-control bg-error px-4 text-sm font-semibold text-card hover:bg-error/90 disabled:opacity-50"
                >
                  Confirm removal
                </button>
                <button
                  type="button"
                  onClick={() => setConfirmRemove(false)}
                  disabled={busy}
                  className={buttonClass.secondary}
                >
                  Keep assignment
                </button>
              </>
            ) : (
              <button
                type="button"
                onClick={() => setConfirmRemove(true)}
                disabled={busy}
                className={buttonClass.secondary}
              >
                Remove assignment
              </button>
            ))}
        </div>
      </Card>
    </>
  );
}
