'use client';

import { useEffect, useState } from 'react';
import { Mono, StatusBadge } from '@waypoint/ui';
import type {
  Depot,
  Driver,
  Loader,
  Outlet,
  StoreManagerOption,
  Vehicle,
  VehicleAssignment,
} from '@waypoint/shared-types';
import { api } from '../../../lib/api';
import { useApiQuery } from '../../../lib/use-api-query';
import {
  EmptyState,
  ErrorState,
  LoadingState,
} from '../../../components/states';
import {
  Card,
  PageHeader,
  SectionHeading,
  buttonClass,
} from '../_components/ui';
import { Field, SavedNotice, inputClass, readableAssignmentError } from '../_components/master-data';
import { useDispatcherScope } from '../_components/dispatcher-context';

type Tab = 'drivers' | 'managers' | 'loaders';

const TABS: { key: Tab; label: string }[] = [
  { key: 'drivers', label: 'Driver → Vehicle' },
  { key: 'managers', label: 'Store Manager → Outlet' },
  { key: 'loaders', label: 'Loader → Depot' },
];

/**
 * Dispatcher · Assignments. One screen for the three operational links the
 * planner needs before it can build a run: which driver is on which vehicle for
 * the delivery day, which store manager owns which outlet, and which depot each
 * loader works in.
 *
 * Every mutation is server-validated (role, depot compatibility, one
 * driver/vehicle per date, loader depot existence) and Dispatcher-only; the
 * server rejects anything the UI might miss.
 */
export function AssignmentsScreen() {
  const [tab, setTab] = useState<Tab>('drivers');
  return (
    <>
      <PageHeader
        title="Assignments"
        description="Operational links for the delivery day: drivers to vehicles, store managers to outlets, and loaders to depots. Every change is validated and audited server-side."
      />

      <div role="tablist" aria-label="Assignment type" className="mb-4 flex flex-wrap gap-1.5">
        {TABS.map((t) => (
          <button
            key={t.key}
            type="button"
            role="tab"
            aria-selected={tab === t.key}
            aria-controls={`panel-${t.key}`}
            id={`tab-${t.key}`}
            onClick={() => setTab(t.key)}
            className={`h-10 rounded-control px-3 text-sm font-medium ring-1 ${
              tab === t.key
                ? 'bg-action text-card ring-action'
                : 'bg-card text-ink ring-ink/15 hover:bg-page'
            }`}
          >
            {t.label}
          </button>
        ))}
      </div>

      <div
        role="tabpanel"
        id={`panel-${tab}`}
        aria-labelledby={`tab-${tab}`}
        className="flex flex-col gap-6"
      >
        {tab === 'drivers' && <DriverVehiclePanel />}
        {tab === 'managers' && <ManagerOutletPanel />}
        {tab === 'loaders' && <LoaderDepotPanel />}
      </div>
    </>
  );
}

/* -------------------------------------------------------------------------- */
/* Driver → Vehicle                                                           */
/* -------------------------------------------------------------------------- */

function DriverVehiclePanel() {
  const { deliveryDate, depot, depots } = useDispatcherScope();
  const [date, setDate] = useState('');
  useEffect(() => {
    setDate((d) => d || deliveryDate || '');
  }, [deliveryDate]);

  const [selectedDriver, setSelectedDriver] = useState('');
  const [selectedVehicle, setSelectedVehicle] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState<string | null>(null);

  const vehicles = useApiQuery(
    depot ? `assign-vehicles:${depot.depotId}` : null,
    () => api.getVehicles({ depotId: depot?.depotId }),
  );
  const drivers = useApiQuery(
    depot ? `assign-drivers:${depot.depotId}` : null,
    () => api.listDrivers({ depotId: depot?.depotId }),
  );
  const assignments = useApiQuery(
    date ? `assign-list:${depot?.depotId ?? ''}:${date}` : null,
    () =>
      api.listVehicleAssignments(date).then((rows) =>
        depot ? rows.filter((r) => r.depotId === depot.depotId) : rows,
      ),
  );

  const rows = assignments.data ?? [];

  async function assign() {
    if (!date) return setError('Choose an operating date first.');
    if (!selectedVehicle) return setError('Choose a vehicle.');
    if (!selectedDriver) return setError('Choose a driver.');
    setBusy(true);
    setError(null);
    setSaved(null);
    try {
      await api.assignDriver(selectedVehicle, { driverId: selectedDriver, date });
      setSaved('Driver assigned.');
      setSelectedDriver('');
      setSelectedVehicle('');
      assignments.reload();
    } catch (err) {
      setError(readableAssignmentError(err));
    } finally {
      setBusy(false);
    }
  }

  async function remove(row: VehicleAssignment) {
    setBusy(true);
    setError(null);
    setSaved(null);
    try {
      await api.unassignDriver(row.vehicleId, row.date);
      setSaved('Driver unassigned.');
      assignments.reload();
    } catch (err) {
      setError(readableAssignmentError(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <Card className="flex flex-col gap-4">
        <SectionHeading>Assign a driver to a vehicle</SectionHeading>
        {saved && <SavedNotice>{saved}</SavedNotice>}
        {error && (
          <p role="alert" className="text-sm text-error-strong">
            {error}
          </p>
        )}
        <div className="grid gap-4 sm:grid-cols-3">
          <Field label="Operating date" htmlFor="dv-date" required>
            <input
              id="dv-date"
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
          <Field label="Vehicle" htmlFor="dv-vehicle" required hint="This depot's fleet.">
            <select
              id="dv-vehicle"
              value={selectedVehicle}
              onChange={(e) => setSelectedVehicle(e.target.value)}
              className={inputClass()}
              disabled={vehicles.loading}
            >
              <option value="">— select a vehicle —</option>
              {(vehicles.data ?? []).map((v: Vehicle) => (
                <option key={v.vehicleId} value={v.vehicleId}>
                  {v.vehicleId} · {v.tempClass === 'REEFER' ? 'Refrigerated' : 'Dry-box'}{' '}
                  {v.type === 'VAN' ? 'van' : 'truck'}
                </option>
              ))}
            </select>
          </Field>
          <Field
            label="Driver"
            htmlFor="dv-driver"
            required
            hint="Only active drivers in this vehicle's depot."
          >
            <select
              id="dv-driver"
              value={selectedDriver}
              onChange={(e) => setSelectedDriver(e.target.value)}
              className={inputClass()}
              disabled={drivers.loading}
            >
              <option value="">— select a driver —</option>
              {(drivers.data ?? []).map((d: Driver) => (
                <option key={d.userId} value={d.userId}>
                  {d.name} · {d.email}
                </option>
              ))}
            </select>
          </Field>
        </div>
        <div>
          <button
            type="button"
            onClick={assign}
            disabled={busy || !date || !selectedVehicle || !selectedDriver}
            aria-busy={busy}
            className={buttonClass.primary}
          >
            {busy ? 'Saving…' : 'Assign / change'}
          </button>
        </div>
      </Card>

      <section>
        <SectionHeading>
          Driver assignments · {date || 'no date'} · {depot?.name ?? 'all depots'}
        </SectionHeading>
        {assignments.loading && !rows.length ? (
          <LoadingState label="Loading assignments…" />
        ) : assignments.error ? (
          <ErrorState error={assignments.error} onRetry={assignments.reload} />
        ) : rows.length === 0 ? (
          <EmptyState title="No drivers assigned for this date">
            Assign a driver to a vehicle above to cover the day's run.
          </EmptyState>
        ) : (
          <div className="overflow-x-auto rounded-card bg-card ring-1 ring-ink/10">
            <table className="w-full min-w-[760px] text-left text-sm">
              <thead className="bg-brand/5 text-[0.6875rem] uppercase tracking-wide text-ink-muted">
                <tr>
                  <th className="px-4 py-2.5 font-semibold">Driver</th>
                  <th className="px-4 py-2.5 font-semibold">Depot</th>
                  <th className="px-4 py-2.5 font-semibold">Date</th>
                  <th className="px-4 py-2.5 font-semibold">Vehicle</th>
                  <th className="px-4 py-2.5 text-right font-semibold">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-ink/5">
                {rows.map((r) => (
                  <tr key={`${r.vehicleId}-${r.date}`} className="hover:bg-page">
                    <td className="px-4 py-2.5">
                      {r.driverName} <span className="text-ink-muted">{r.driverEmail}</span>
                    </td>
                    <td className="px-4 py-2.5">
                      <Mono>{depotName(depots ?? [], r.depotId)}</Mono>
                    </td>
                    <td className="px-4 py-2.5">
                      <Mono>{r.date}</Mono>
                    </td>
                    <td className="px-4 py-2.5">
                      <Mono className="font-semibold">{r.vehicleId}</Mono>
                    </td>
                    <td className="px-4 py-2.5 text-right">
                      <button
                        type="button"
                        onClick={() => void remove(r)}
                        disabled={busy}
                        className="text-error hover:underline disabled:opacity-50"
                      >
                        Remove
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </>
  );
}

/* -------------------------------------------------------------------------- */
/* Store Manager → Outlet                                                     */
/* -------------------------------------------------------------------------- */

function ManagerOutletPanel() {
  const managers = useApiQuery('assign-managers', () => api.listStoreManagers());
  const outlets = useApiQuery('assign-outlets', () => api.getOutlets());
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState<string | null>(null);

  async function setOutlet(manager: StoreManagerOption, outletId: string) {
    if (!outletId) return;
    setBusy(true);
    setError(null);
    setSaved(null);
    try {
      await api.assignManager(outletId, { userId: manager.userId });
      setSaved(`${manager.name || manager.email} assigned to ${outletId}.`);
      managers.reload();
    } catch (err) {
      setError(readableAssignmentError(err));
    } finally {
      setBusy(false);
    }
  }

  async function remove(manager: StoreManagerOption) {
    if (!manager.outletId) return;
    setBusy(true);
    setError(null);
    setSaved(null);
    try {
      await api.unassignManager(manager.outletId);
      setSaved(`${manager.name || manager.email} unassigned.`);
      managers.reload();
    } catch (err) {
      setError(readableAssignmentError(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card className="flex flex-col gap-4">
      <SectionHeading>Store manager → outlet</SectionHeading>
      {saved && <SavedNotice>{saved}</SavedNotice>}
      {error && (
        <p role="alert" className="text-sm text-error-strong">
          {error}
        </p>
      )}

      {managers.loading && !managers.data ? (
        <LoadingState label="Loading store managers…" />
      ) : managers.error ? (
        <ErrorState error={managers.error} onRetry={managers.reload} />
      ) : (managers.data ?? []).length === 0 ? (
        <EmptyState title="No store managers">
          Create a store manager account under Users first.
        </EmptyState>
      ) : (
        <div className="overflow-x-auto rounded-card ring-1 ring-ink/10">
          <table className="w-full min-w-[720px] text-left text-sm">
            <thead className="bg-brand/5 text-[0.6875rem] uppercase tracking-wide text-ink-muted">
              <tr>
                <th className="px-4 py-2.5 font-semibold">Manager</th>
                <th className="px-4 py-2.5 font-semibold">Outlet</th>
                <th className="px-4 py-2.5 font-semibold">Depot</th>
                <th className="px-4 py-2.5 text-right font-semibold">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-ink/5">
              {(managers.data ?? []).map((m) => (
                <tr key={m.userId} className="hover:bg-page">
                  <td className="px-4 py-2.5">
                    {m.name || m.email} <span className="text-ink-muted">{m.email}</span>
                  </td>
                  <td className="px-4 py-2.5">
                    <select
                      aria-label={`Outlet for ${m.name || m.email}`}
                      value={m.outletId}
                      disabled={busy}
                      onChange={(e) => void setOutlet(m, e.target.value)}
                      className="h-9 rounded-control bg-card px-2 text-sm text-ink ring-1 ring-ink/15"
                    >
                      <option value="">— unassigned —</option>
                      {(outlets.data ?? []).map((o: Outlet) => (
                        <option key={o.outletId} value={o.outletId}>
                          {o.outletId} · {o.name}
                        </option>
                      ))}
                    </select>
                  </td>
                  <td className="px-4 py-2.5">
                    {m.depotId ? (
                      <Mono>{m.depotId}</Mono>
                    ) : (
                      <span className="text-ink-muted">—</span>
                    )}
                  </td>
                  <td className="px-4 py-2.5 text-right">
                    <button
                      type="button"
                      onClick={() => void remove(m)}
                      disabled={busy || !m.outletId}
                      className="text-error hover:underline disabled:opacity-50"
                    >
                      Remove
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <p className="text-xs text-ink-muted">
        A store manager's depot is derived from the assigned outlet; it cannot be
        set independently. Assigning an outlet releases whoever held it before.
      </p>
    </Card>
  );
}

/* -------------------------------------------------------------------------- */
/* Loader → Depot                                                             */
/* -------------------------------------------------------------------------- */

function LoaderDepotPanel() {
  const loaders = useApiQuery('assign-loaders', () => api.listLoaders());
  const depots = useApiQuery('depots', () => api.getDepots());
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState<string | null>(null);

  async function setDepot(loader: Loader, depotId: string) {
    if (!depotId) return;
    setBusy(true);
    setError(null);
    setSaved(null);
    try {
      await api.assignLoader(loader.userId, { depotId });
      setSaved(`${loader.name || loader.email} assigned to a depot.`);
      loaders.reload();
    } catch (err) {
      setError(readableAssignmentError(err));
    } finally {
      setBusy(false);
    }
  }

  async function remove(loader: Loader) {
    if (!loader.depotId) return;
    setBusy(true);
    setError(null);
    setSaved(null);
    try {
      await api.unassignLoader(loader.userId);
      setSaved(`${loader.name || loader.email} unassigned.`);
      loaders.reload();
    } catch (err) {
      setError(readableAssignmentError(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card className="flex flex-col gap-4">
      <SectionHeading>Loader → depot</SectionHeading>
      {saved && <SavedNotice>{saved}</SavedNotice>}
      {error && (
        <p role="alert" className="text-sm text-error-strong">
          {error}
        </p>
      )}

      {loaders.loading && !loaders.data ? (
        <LoadingState label="Loading loaders…" />
      ) : loaders.error ? (
        <ErrorState error={loaders.error} onRetry={loaders.reload} />
      ) : (loaders.data ?? []).length === 0 ? (
        <EmptyState title="No loaders">
          Create a loader account under Users first.
        </EmptyState>
      ) : (
        <div className="overflow-x-auto rounded-card ring-1 ring-ink/10">
          <table className="w-full min-w-[640px] text-left text-sm">
            <thead className="bg-brand/5 text-[0.6875rem] uppercase tracking-wide text-ink-muted">
              <tr>
                <th className="px-4 py-2.5 font-semibold">Loader</th>
                <th className="px-4 py-2.5 font-semibold">Depot</th>
                <th className="px-4 py-2.5 text-right font-semibold">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-ink/5">
              {(loaders.data ?? []).map((l) => (
                <tr key={l.userId} className="hover:bg-page">
                  <td className="px-4 py-2.5">
                    {l.name || l.email} <span className="text-ink-muted">{l.email}</span>
                  </td>
                  <td className="px-4 py-2.5">
                    <select
                      aria-label={`Depot for ${l.name || l.email}`}
                      value={l.depotId}
                      disabled={busy}
                      onChange={(e) => void setDepot(l, e.target.value)}
                      className="h-9 rounded-control bg-card px-2 text-sm text-ink ring-1 ring-ink/15"
                    >
                      <option value="">— unassigned —</option>
                      {(depots.data ?? []).map((d: Depot) => (
                        <option key={d.depotId} value={d.depotId}>
                          {d.name}
                        </option>
                      ))}
                    </select>
                  </td>
                  <td className="px-4 py-2.5 text-right">
                    <button
                      type="button"
                      onClick={() => void remove(l)}
                      disabled={busy || !l.depotId}
                      className="text-error hover:underline disabled:opacity-50"
                    >
                      Remove
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <p className="text-xs text-ink-muted">
        A loader's depot decides which depot's loading work they see. A loader
        cannot change their own depot.
      </p>
    </Card>
  );
}

function depotName(depots: readonly Depot[], depotId: string): string {
  return depots.find((d) => d.depotId === depotId)?.name ?? depotId;
}
