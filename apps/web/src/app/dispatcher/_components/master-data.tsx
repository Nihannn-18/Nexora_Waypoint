'use client';

import { useState, type ReactNode } from 'react';
import { Mono } from '@waypoint/ui';
import { WaypointApiError } from '@waypoint/api-client';
import type {
  DockType,
  ParkingConstraint,
  VehicleStatus,
  VehicleTempClass,
  VehicleType,
} from '@waypoint/shared-types';
import { formatKg, formatLitres, formatM3, humanize } from '../../../lib/format';
import { buttonClass, Card, Fact } from './ui';

/**
 * Shared Dispatcher master-data presentation. Vehicles and outlets are planning
 * inputs, so the screens label identity as immutable and never offer a delete —
 * a vehicle that should stop being used is put out of service through its
 * availability, not removed.
 */

export const VEHICLE_TYPES: readonly VehicleType[] = ['TRUCK', 'VAN'];
export const VEHICLE_TEMP_CLASSES: readonly VehicleTempClass[] = [
  'REEFER',
  'AMBIENT',
];
export const DOCK_TYPES: readonly DockType[] = [
  'REAR_DOCK',
  'STREET',
  'MALL_BAY',
];
export const PARKING_CONSTRAINTS: readonly ParkingConstraint[] = [
  'NORMAL',
  'VAN_ONLY',
  'MALL_DOCK',
];

/** Neutral tone mapping: the word is always shown, colour is never the signal. */
export function vehicleStatusTone(
  status: VehicleStatus | string,
): 'success' | 'warning' | 'error' | 'neutral' {
  switch (status) {
    case 'AVAILABLE':
      return 'success';
    case 'IN_WORKSHOP':
      return 'warning';
    case 'BROKEN_DOWN':
    case 'UNAVAILABLE':
      return 'error';
    default:
      return 'neutral';
  }
}

/** A labelled form field with an inline error and a required marker. */
export function Field({
  label,
  htmlFor,
  required = false,
  error,
  hint,
  children,
}: {
  label: string;
  htmlFor: string;
  required?: boolean;
  error?: string;
  hint?: string;
  children: ReactNode;
}) {
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={htmlFor} className="text-sm font-medium text-ink">
        {label}
        {required && (
          <span aria-hidden="true" className="ml-0.5 text-error">
            *
          </span>
        )}
      </label>
      {children}
      {hint && !error && <p className="text-xs text-ink-muted">{hint}</p>}
      {error && (
        <p role="alert" className="text-xs text-error-strong">
          {error}
        </p>
      )}
    </div>
  );
}

/** A text/select input styled to the dispatcher control tokens. */
export function inputClass(hasError = false): string {
  return `tap-target w-full rounded-control bg-card px-3 text-sm text-ink ring-1 ${
    hasError ? 'ring-error' : 'ring-ink/15'
  } focus-visible:ring-2 focus-visible:ring-brand`;
}

/**
 * The form action row: submit is disabled while submitting, and there is an
 * explicit cancel. A destructive or ambiguous change asks for confirmation via
 * the caller; create/update themselves are not destructive.
 */
export function FormActions({
  submitting,
  submitLabel,
  onCancel,
}: {
  submitting: boolean;
  submitLabel: string;
  onCancel: () => void;
}) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <button
        type="submit"
        disabled={submitting}
        aria-busy={submitting}
        className={buttonClass.primary}
      >
        {submitting ? 'Saving…' : submitLabel}
      </button>
      <button
        type="button"
        onClick={onCancel}
        disabled={submitting}
        className={buttonClass.secondary}
      >
        Cancel
      </button>
    </div>
  );
}

/** A success banner shown after a create/update persists. */
export function SavedNotice({ children }: { children: ReactNode }) {
  return (
    <Card className="border border-success bg-success-bg">
      <p className="flex items-center gap-2 text-sm text-ink">
        <span aria-hidden="true" className="text-success">
          ✓
        </span>
        {children}
      </p>
    </Card>
  );
}

/** Identity rows shared by both detail screens; identity is immutable. */
export function IdentityFact({ label, id }: { label: string; id: string }) {
  return (
    <Fact label={label}>
      <Mono className="font-semibold">{id}</Mono>
      <span className="ml-2 text-xs text-ink-muted">(immutable)</span>
    </Fact>
  );
}

/** Small helper for the vehicle summary line used in lists. */
export function vehicleSummary(v: {
  type: VehicleType;
  tempClass: VehicleTempClass;
}): string {
  return `${v.tempClass === 'REEFER' ? 'Refrigerated' : 'Dry-box'} ${humanize(
    v.type,
  ).toLowerCase()}`;
}

export const KM_PER_L = (n: number) => `${n} km/L`;
export { formatKg, formatLitres, formatM3 };

/**
 * Maps an assignment error to a sentence. A 409 here is a genuine operational
 * conflict (a driver/vehicle is already committed for the date), not a duplicate
 * identifier, so it gets its own wording rather than the master-data default.
 */
export function readableAssignmentError(error: unknown): string {
  if (error instanceof WaypointApiError) {
    if (error.isOffline) {
      return 'No connection to the server. Try again — nothing has been saved.';
    }
    switch (error.status) {
      case 400:
      case 422:
        return (
          error.fieldErrors?.[0]?.message ??
          'The assignment was rejected. Check the values and try again.'
        );
      case 403:
        return 'Only a dispatcher may change assignments.';
      case 409:
        return 'That driver or vehicle is already assigned for this date. Remove the existing assignment first.';
      case 404:
        return 'That record was not found.';
      default:
        return 'Something went wrong on our side. Try again in a moment.';
    }
  }
  return 'Something went wrong. Try again in a moment.';
}

/**
 * A tiny reducer-friendly hook: tracks submit state and the first server field
 * error per field so forms can mark errors in place without discarding them.
 */
export function useSubmitState() {
  const [submitting, setSubmitting] = useState(false);
  return { submitting, setSubmitting };
}
