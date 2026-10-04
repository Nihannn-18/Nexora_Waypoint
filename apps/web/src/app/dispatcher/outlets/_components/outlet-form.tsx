'use client';

import { useState, type FormEvent } from 'react';
import { useRouter } from 'next/navigation';
import type { Depot, Outlet, OutletWriteRequest } from '@waypoint/shared-types';
import { api } from '../../../../lib/api';
import { Card, SectionHeading } from '../../_components/ui';
import {
  DOCK_TYPES,
  Field,
  FormActions,
  PARKING_CONSTRAINTS,
  inputClass,
} from '../../_components/master-data';
import { applyFieldErrors, readableMasterError } from '../../vehicles/_components/vehicle-form';

const BRANDS = ['FRESH', 'STYLE', 'TECH'] as const;

interface Draft {
  name: string;
  brand: string;
  district: string;
  depotId: string;
  dockType: string;
  parkingConstraint: string;
  windowOpenTime: string;
  windowCloseTime: string;
  mallWindowOpen: string;
  mallWindowClose: string;
}

function draftFrom(outlet: Outlet | undefined, depots: readonly Depot[]): Draft {
  return {
    name: outlet?.name ?? '',
    brand: outlet?.brand ?? 'FRESH',
    district: outlet?.district ?? '',
    depotId: outlet?.depotId ?? depots[0]?.depotId ?? '',
    dockType: outlet?.dockType ?? 'STREET',
    parkingConstraint: outlet?.parkingConstraint ?? 'NORMAL',
    windowOpenTime: outlet?.windowOpenTime ?? '05:00',
    windowCloseTime: outlet?.windowCloseTime ?? '08:00',
    mallWindowOpen: outlet?.mallWindow?.open ?? '',
    mallWindowClose: outlet?.mallWindow?.close ?? '',
  };
}

const TIME = /^([01]\d|2[0-3]):[0-5]\d$/;

/**
 * The outlet create/edit form. Outlets are planning inputs, so brand, district,
 * depot, dock type, parking constraint and windows are all validated server-side.
 * A MALL_DOCK outlet must carry a mall access window; a non-mall outlet must not.
 */
export function OutletForm({
  outlet,
  depots,
}: {
  outlet?: Outlet;
  depots: readonly Depot[];
}) {
  const router = useRouter();
  const editing = outlet !== undefined;
  const [draft, setDraft] = useState<Draft>(() => draftFrom(outlet, depots));
  const [submitting, setSubmitting] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState<string | null>(null);

  const set = (key: keyof Draft) => (value: string) => {
    setDraft((d) => ({ ...d, [key]: value }));
    setErrors((e) => ({ ...e, [key]: '' }));
  };

  const isMall = draft.parkingConstraint === 'MALL_DOCK';

  function localErrors(): Record<string, string> {
    const e: Record<string, string> = {};
    if (!draft.name.trim()) e.name = 'Name is required';
    if (!draft.district.trim()) e.district = 'District is required';
    if (!draft.depotId) e.depotId = 'Depot is required';
    if (!TIME.test(draft.windowOpenTime))
      e.windowOpenTime = 'Use HH:MM (24-hour)';
    if (!TIME.test(draft.windowCloseTime))
      e.windowCloseTime = 'Use HH:MM (24-hour)';
    if (TIME.test(draft.windowOpenTime) && TIME.test(draft.windowCloseTime) && draft.windowCloseTime <= draft.windowOpenTime)
      e.windowCloseTime = 'Close must be after open';
    if (isMall) {
      if (!TIME.test(draft.mallWindowOpen)) e.mallWindowOpen = 'Use HH:MM (24-hour)';
      if (!TIME.test(draft.mallWindowClose)) e.mallWindowClose = 'Use HH:MM (24-hour)';
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

    const body: OutletWriteRequest = {
      name: draft.name.trim(),
      brand: draft.brand as OutletWriteRequest['brand'],
      district: draft.district.trim(),
      depotId: draft.depotId,
      dockType: draft.dockType as OutletWriteRequest['dockType'],
      parkingConstraint:
        draft.parkingConstraint as OutletWriteRequest['parkingConstraint'],
      windowOpenTime: draft.windowOpenTime,
      windowCloseTime: draft.windowCloseTime,
      ...(isMall
        ? {
            mallWindowOpen: draft.mallWindowOpen,
            mallWindowClose: draft.mallWindowClose,
          }
        : {}),
    };

    try {
      const saved = editing
        ? await api.updateOutlet(outlet.outletId, body)
        : await api.createOutlet(body);
      router.push(`/dispatcher/outlets/${saved.outletId}?saved=1`);
    } catch (err) {
      setFormError(readableMasterError(err, 'outlet'));
      applyFieldErrors(err, setErrors);
      setSubmitting(false);
    }
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-4">
      <SectionHeading>
        {editing ? `Edit ${outlet.outletId}` : 'New outlet'}
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
          <Field label="Outlet name" htmlFor="name" required error={errors.name}>
            <input
              id="name"
              type="text"
              value={draft.name}
              onChange={(e) => set('name')(e.target.value)}
              className={inputClass(Boolean(errors.name))}
            />
          </Field>

          <Field label="Brand" htmlFor="brand" required error={errors.brand}>
            <select
              id="brand"
              value={draft.brand}
              onChange={(e) => set('brand')(e.target.value)}
              className={inputClass(Boolean(errors.brand))}
            >
              {BRANDS.map((b) => (
                <option key={b} value={b}>
                  {b}
                </option>
              ))}
            </select>
          </Field>

          <Field
            label="District"
            htmlFor="district"
            required
            error={errors.district}
          >
            <input
              id="district"
              type="text"
              value={draft.district}
              onChange={(e) => set('district')(e.target.value)}
              className={inputClass(Boolean(errors.district))}
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

          <Field label="Dock type" htmlFor="dockType" required error={errors.dockType}>
            <select
              id="dockType"
              value={draft.dockType}
              onChange={(e) => set('dockType')(e.target.value)}
              className={inputClass(Boolean(errors.dockType))}
            >
              {DOCK_TYPES.map((d) => (
                <option key={d} value={d}>
                  {d}
                </option>
              ))}
            </select>
          </Field>

          <Field
            label="Access constraint"
            htmlFor="parkingConstraint"
            required
            error={errors.parkingConstraint}
            hint="VAN_ONLY means only a van may serve it."
          >
            <select
              id="parkingConstraint"
              value={draft.parkingConstraint}
              onChange={(e) => set('parkingConstraint')(e.target.value)}
              className={inputClass(Boolean(errors.parkingConstraint))}
            >
              {PARKING_CONSTRAINTS.map((p) => (
                <option key={p} value={p}>
                  {p}
                </option>
              ))}
            </select>
          </Field>

          <Field
            label="Window opens"
            htmlFor="windowOpenTime"
            required
            error={errors.windowOpenTime}
            hint="24-hour HH:MM"
          >
            <input
              id="windowOpenTime"
              type="time"
              value={draft.windowOpenTime}
              onChange={(e) => set('windowOpenTime')(e.target.value)}
              className={inputClass(Boolean(errors.windowOpenTime))}
            />
          </Field>

          <Field
            label="Window closes"
            htmlFor="windowCloseTime"
            required
            error={errors.windowCloseTime}
            hint="24-hour HH:MM"
          >
            <input
              id="windowCloseTime"
              type="time"
              value={draft.windowCloseTime}
              onChange={(e) => set('windowCloseTime')(e.target.value)}
              className={inputClass(Boolean(errors.windowCloseTime))}
            />
          </Field>

          {isMall && (
            <>
              <Field
                label="Mall access opens"
                htmlFor="mallWindowOpen"
                required
                error={errors.mallWindowOpen}
              >
                <input
                  id="mallWindowOpen"
                  type="time"
                  value={draft.mallWindowOpen}
                  onChange={(e) => set('mallWindowOpen')(e.target.value)}
                  className={inputClass(Boolean(errors.mallWindowOpen))}
                />
              </Field>
              <Field
                label="Mall access closes"
                htmlFor="mallWindowClose"
                required
                error={errors.mallWindowClose}
              >
                <input
                  id="mallWindowClose"
                  type="time"
                  value={draft.mallWindowClose}
                  onChange={(e) => set('mallWindowClose')(e.target.value)}
                  className={inputClass(Boolean(errors.mallWindowClose))}
                />
              </Field>
            </>
          )}
        </div>

        <FormActions
          submitting={submitting}
          submitLabel={editing ? 'Save changes' : 'Create outlet'}
          onCancel={() => router.push('/dispatcher/outlets')}
        />
      </Card>
    </form>
  );
}
