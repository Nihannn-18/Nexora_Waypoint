'use client';

import { useState, type FormEvent } from 'react';
import { useRouter } from 'next/navigation';
import type { Depot, Vehicle, VehicleWriteRequest } from '@waypoint/shared-types';
import { WaypointApiError } from '@waypoint/api-client';
import { api } from '../../../../lib/api';
import { Card, SectionHeading } from '../../_components/ui';
import {
  Field,
  FormActions,
  VEHICLE_TEMP_CLASSES,
  VEHICLE_TYPES,
  inputClass,
} from '../../_components/master-data';

interface Draft {
  type: string;
  tempClass: string;
  weightCapKg: string;
  volumeCapM3: string;
  fuelType: string;
  kmPerL: string;
  weeklyFuelQuotaL: string;
  depotId: string;
}

function draftFrom(vehicle: Vehicle | undefined, depots: readonly Depot[]): Draft {
  return {
    type: vehicle?.type ?? 'TRUCK',
    tempClass: vehicle?.tempClass ?? 'AMBIENT',
    weightCapKg: vehicle ? String(vehicle.weightCapKg) : '',
    volumeCapM3: vehicle ? String(vehicle.volumeCapM3) : '',
    fuelType: vehicle?.fuelType ?? 'diesel',
    kmPerL: vehicle ? String(vehicle.kmPerL) : '',
    weeklyFuelQuotaL: vehicle ? String(vehicle.weeklyFuelQuotaL) : '',
    depotId: vehicle?.depotId ?? depots[0]?.depotId ?? '',
  };
}

/**
 * The vehicle create/edit form. Only mutable operating fields are editable; the
 * identity is server-generated on create and immutable on edit. Numeric fields
 * are validated in the browser for a fast signal, and the server revalidates
 * authoritatively — a server field error is shown against the offending field.
 */
export function VehicleForm({
  vehicle,
  depots,
}: {
  vehicle?: Vehicle;
  depots: readonly Depot[];
}) {
  const router = useRouter();
  const editing = vehicle !== undefined;
  const [draft, setDraft] = useState<Draft>(() => draftFrom(vehicle, depots));
  const [submitting, setSubmitting] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState<string | null>(null);

  const set = (key: keyof Draft) => (value: string) => {
    setDraft((d) => ({ ...d, [key]: value }));
    setErrors((e) => ({ ...e, [key]: '' }));
  };

  function localErrors(): Record<string, string> {
    const e: Record<string, string> = {};
    const positive = (key: keyof Draft, label: string) => {
      const n = Number(draft[key]);
      if (!Number.isFinite(n) || n <= 0) e[key] = `${label} must be greater than zero`;
    };
    positive('weightCapKg', 'Weight capacity');
    positive('volumeCapM3', 'Volume capacity');
    positive('kmPerL', 'Fuel efficiency');
    const quota = Number(draft.weeklyFuelQuotaL);
    if (!Number.isFinite(quota) || quota < 0)
      e.weeklyFuelQuotaL = 'Weekly quota must not be negative';
    if (!draft.fuelType.trim()) e.fuelType = 'Fuel type is required';
    if (!draft.depotId) e.depotId = 'Depot is required';
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

    const body: VehicleWriteRequest = {
      type: draft.type as VehicleWriteRequest['type'],
      tempClass: draft.tempClass as VehicleWriteRequest['tempClass'],
      weightCapKg: Number(draft.weightCapKg),
      volumeCapM3: Number(draft.volumeCapM3),
      fuelType: draft.fuelType.trim(),
      kmPerL: Number(draft.kmPerL),
      weeklyFuelQuotaL: Number(draft.weeklyFuelQuotaL),
      depotId: draft.depotId,
    };

    try {
      const saved = editing
        ? await api.updateVehicle(vehicle.vehicleId, body)
        : await api.createVehicle(body);
      router.push(`/dispatcher/vehicles/${saved.vehicleId}?saved=1`);
    } catch (err) {
      setFormError(readableMasterError(err, 'vehicle'));
      applyFieldErrors(err, setErrors);
      setSubmitting(false);
    }
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-4">
      <SectionHeading>
        {editing ? `Edit ${vehicle.vehicleId}` : 'New vehicle'}
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
          <Field label="Vehicle type" htmlFor="type" required error={errors.type}>
            <select
              id="type"
              value={draft.type}
              onChange={(e) => set('type')(e.target.value)}
              className={inputClass(Boolean(errors.type))}
            >
              {VEHICLE_TYPES.map((t) => (
                <option key={t} value={t}>
                  {t === 'TRUCK' ? 'Truck' : 'Van'}
                </option>
              ))}
            </select>
          </Field>

          <Field
            label="Temperature capability"
            htmlFor="tempClass"
            required
            error={errors.tempClass}
            hint="A reefer may also carry ambient goods."
          >
            <select
              id="tempClass"
              value={draft.tempClass}
              onChange={(e) => set('tempClass')(e.target.value)}
              className={inputClass(Boolean(errors.tempClass))}
            >
              {VEHICLE_TEMP_CLASSES.map((t) => (
                <option key={t} value={t}>
                  {t === 'REEFER' ? 'Refrigerated (reefer)' : 'Ambient (dry-box)'}
                </option>
              ))}
            </select>
          </Field>

          <Field
            label="Weight capacity (kg)"
            htmlFor="weightCapKg"
            required
            error={errors.weightCapKg}
          >
            <input
              id="weightCapKg"
              type="number"
              min="0.01"
              step="0.01"
              value={draft.weightCapKg}
              onChange={(e) => set('weightCapKg')(e.target.value)}
              className={inputClass(Boolean(errors.weightCapKg))}
            />
          </Field>

          <Field
            label="Volume capacity (m³)"
            htmlFor="volumeCapM3"
            required
            error={errors.volumeCapM3}
          >
            <input
              id="volumeCapM3"
              type="number"
              min="0.01"
              step="0.01"
              value={draft.volumeCapM3}
              onChange={(e) => set('volumeCapM3')(e.target.value)}
              className={inputClass(Boolean(errors.volumeCapM3))}
            />
          </Field>

          <Field label="Fuel type" htmlFor="fuelType" required error={errors.fuelType}>
            <input
              id="fuelType"
              type="text"
              value={draft.fuelType}
              onChange={(e) => set('fuelType')(e.target.value)}
              className={inputClass(Boolean(errors.fuelType))}
            />
          </Field>

          <Field
            label="Fuel efficiency (km/L)"
            htmlFor="kmPerL"
            required
            error={errors.kmPerL}
          >
            <input
              id="kmPerL"
              type="number"
              min="0.01"
              step="0.1"
              value={draft.kmPerL}
              onChange={(e) => set('kmPerL')(e.target.value)}
              className={inputClass(Boolean(errors.kmPerL))}
            />
          </Field>

          <Field
            label="Weekly fuel quota (L)"
            htmlFor="weeklyFuelQuotaL"
            required
            error={errors.weeklyFuelQuotaL}
          >
            <input
              id="weeklyFuelQuotaL"
              type="number"
              min="0"
              step="0.01"
              value={draft.weeklyFuelQuotaL}
              onChange={(e) => set('weeklyFuelQuotaL')(e.target.value)}
              className={inputClass(Boolean(errors.weeklyFuelQuotaL))}
            />
          </Field>

          <Field label="Home depot" htmlFor="depotId" required error={errors.depotId}>
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
        </div>

        <FormActions
          submitting={submitting}
          submitLabel={editing ? 'Save changes' : 'Create vehicle'}
          onCancel={() => router.push('/dispatcher/vehicles')}
        />
      </Card>
    </form>
  );
}

/** Maps a server error to a sentence; a 400 field error is applied in place. */
export function readableMasterError(error: unknown, subject: string): string {
  if (error instanceof WaypointApiError) {
    if (error.isOffline) {
      return 'No connection to the server. Try again — nothing has been saved.';
    }
    switch (error.status) {
      case 400:
      case 422:
        return (
          error.fieldErrors?.[0]?.message ??
          `The ${subject} was rejected. Check the values and try again.`
        );
      case 403:
        return 'Only a dispatcher may change master data.';
      case 409:
        return 'That identifier is already in use.';
      case 404:
        return `That ${subject} was not found.`;
      default:
        return 'Something went wrong on our side. Try again in a moment.';
    }
  }
  return 'Something went wrong. Try again in a moment.';
}

/** Copies server field errors into the form's error map, marking them inline. */
export function applyFieldErrors(
  error: unknown,
  setErrors: (updater: (e: Record<string, string>) => Record<string, string>) => void,
): void {
  if (error instanceof WaypointApiError && error.fieldErrors) {
    const next: Record<string, string> = {};
    for (const fe of error.fieldErrors) next[fe.field] = fe.message;
    setErrors((e) => ({ ...e, ...next }));
  }
}
