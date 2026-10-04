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
import { useDispatcherScope } from '../../_components/dispatcher-context';

/**
 * Dispatcher · Vehicle driver assignment. The operational link the schema was
 * missing: which driver is on this vehicle for an operating date. The date
 * defaults to the dispatcher's current delivery day. The server validates depot
 * compatibility and the one-driver/one-vehicle-per-date rules; a conflict is
 * shown in plain language rather than retried.
 */
export function DriverAssignment({
  vehicleId,
  depotId,
}: {
  vehicleId: string;
  depotId: string;
}) {
  const { deliveryDate } = useDispatcherScope();
  const [date, setDate] = useState('');
  const [selected, setSelected] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState<string | null>(null);
  const [confirmRemove, setConfirmRemove] = useState(false);

  useEffect(() => {
    setDate((d) => d || deliveryDate || '');
  }, [deliveryDate]);

  const drivers = useApiQuery(`assign-drivers:${depotId}`, () =>
    api.listDrivers({ depotId }),
  );
  const assignment = useApiQuery(
    date ? `vehicle-assignment:${vehicleId}:${date}` : null,
    () => api.getVehicleAssignment(vehicleId, date),
  );
  const current = assignment.data ?? null;

  useEffect(() => {
    setSelected((s) => (current ? current.driverId : s));
  }, [current?.driverId]);

  async function assign() {
    if (!date) {
      setError('Choose an operating date first.');
      return;
    }
    if (!selected) {
      setError('Choose a driver first.');
      return;
    }
    setBusy(true);
    setError(null);
    setSaved(null);
    try {
      await api.assignDriver(vehicleId, { driverId: selected, date });
      setSaved('Driver assigned.');
      assignment.reload();
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
      await api.unassignDriver(vehicleId, date);
      setSaved('Driver unassigned.');
      setConfirmRemove(false);
      assignment.reload();
    } catch (err) {
      setError(readableAssignmentError(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <SectionHeading>Driver assignment</SectionHeading>
      <Card className="flex flex-col gap-4">
        {saved && <SavedNotice>{saved}</SavedNotice>}
        {error && (
          <p role="alert" className="text-sm text-error-strong">
            {error}
          </p>
        )}

        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Operating date" htmlFor="assignment-date" required>
            <input
              id="assignment-date"
              type="date"
              value={date}
              onChange={(e) => {
                setDate(e.target.value);
                setSaved(null);
                setError(null);
              }}
              className={inputClass()}
            />
          </Field>

          <Field
            label="Assigned driver"
            htmlFor="assignment-driver"
            required
            hint="Only active drivers in this vehicle's depot are listed."
          >
            <select
              id="assignment-driver"
              value={selected}
              onChange={(e) => setSelected(e.target.value)}
              className={inputClass()}
              disabled={drivers.loading}
            >
              <option value="">— none selected —</option>
              {(drivers.data ?? []).map((d) => (
                <option key={d.userId} value={d.userId}>
                  {d.name} · {d.email}
                </option>
              ))}
            </select>
          </Field>
        </div>

        <div className="rounded-control bg-page px-3 py-2 text-sm">
          {assignment.loading ? (
            <span className="text-ink-muted">Checking the assignment…</span>
          ) : current ? (
            <span className="text-ink">
              <strong className="font-semibold">{current.driverName}</strong>{' '}
              <span className="text-ink-muted">{current.driverEmail}</span> is
              assigned for <span className="font-mono">{current.date}</span>.
            </span>
          ) : (
            <span className="text-ink-muted">
              No driver is assigned for {date || 'this date'}.
            </span>
          )}
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <button
            type="button"
            onClick={assign}
            disabled={busy || !date || !selected}
            aria-busy={busy}
            className={buttonClass.primary}
          >
            {busy ? 'Saving…' : current ? 'Change driver' : 'Assign driver'}
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
