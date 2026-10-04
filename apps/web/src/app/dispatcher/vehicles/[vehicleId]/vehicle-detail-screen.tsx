'use client';

import Link from 'next/link';
import { use, useState } from 'react';
import { useSearchParams } from 'next/navigation';
import { Mono, StatusBadge } from '@waypoint/ui';
import type { Vehicle } from '@waypoint/shared-types';
import { api } from '../../../../lib/api';
import { formatKg, formatLitres, formatM3, humanize } from '../../../../lib/format';
import { useApiQuery } from '../../../../lib/use-api-query';
import { ErrorState, LoadingState } from '../../../../components/states';
import { PageHeader } from '../../_components/ui';
import { Fact, Card, SectionHeading } from '../../_components/ui';
import {
  IdentityFact,
  SavedNotice,
  vehicleStatusTone,
  vehicleSummary,
} from '../../_components/master-data';
import { useDispatcherScope } from '../../_components/dispatcher-context';
import { VehicleForm } from '../_components/vehicle-form';
import { DriverAssignment } from './driver-assignment';

/**
 * Dispatcher · Vehicle detail and edit. The identity is immutable and shown as
 * such; only the mutable operating fields are editable. Saving returns here with
 * a success notice.
 */
export function VehicleDetailScreen({
  params,
}: {
  params: Promise<{ vehicleId: string }>;
}) {
  const { vehicleId } = use(params);
  const search = useSearchParams();
  const saved = search.get('saved') === '1';
  const { depots } = useDispatcherScope();
  const [editing, setEditing] = useState(false);

  const vehicle = useApiQuery(`master-vehicle:${vehicleId}`, () =>
    api.getVehicle(vehicleId),
  );

  if (vehicle.loading && !vehicle.data) {
    return <LoadingState label="Loading vehicle…" />;
  }
  if (vehicle.error && !vehicle.data) {
    return <ErrorState error={vehicle.error} onRetry={vehicle.reload} />;
  }
  const v = vehicle.data as Vehicle;
  if (!v) {
    return (
      <ErrorState
        error={new Error('This vehicle was not found.')}
        onRetry={vehicle.reload}
      />
    );
  }

  if (editing && depots) {
    return (
      <>
        <PageHeader
          title={`Edit ${v.vehicleId}`}
          description="Identity is immutable; the operating fields below can change."
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
        <VehicleForm vehicle={v} depots={depots} />
      </>
    );
  }

  return (
    <>
      <div className="mb-2">
        <Link href="/dispatcher/vehicles" className="text-sm font-medium text-link">
          ← All vehicles
        </Link>
      </div>
      <PageHeader
        title={v.vehicleId}
        description={vehicleSummary(v)}
        actions={
          <button
            type="button"
            onClick={() => setEditing(true)}
            className="tap-target inline-flex items-center gap-2 rounded-control bg-action px-4 text-sm font-semibold text-card hover:bg-action/90"
          >
            Edit vehicle
          </button>
        }
      />

      {saved && <div className="mb-4"><SavedNotice>Vehicle saved.</SavedNotice></div>}

      <SectionHeading>Vehicle</SectionHeading>
      <Card>
        <dl className="grid grid-cols-2 gap-4 sm:grid-cols-3">
          <IdentityFact label="Vehicle ID" id={v.vehicleId} />
          <Fact label="Status">
            <StatusBadge tone={vehicleStatusTone(v.status)}>
              {humanize(v.status)}
            </StatusBadge>
          </Fact>
          <Fact label="Capability">{vehicleSummary(v)}</Fact>
          <Fact label="Weight capacity">
            <Mono>{formatKg(v.weightCapKg)}</Mono>
          </Fact>
          <Fact label="Volume capacity">
            <Mono>{formatM3(v.volumeCapM3)}</Mono>
          </Fact>
          <Fact label="Home depot">
            <Mono>{v.depotId}</Mono>
          </Fact>
          <Fact label="Fuel type">{v.fuelType}</Fact>
          <Fact label="Fuel efficiency">
            <Mono>{v.kmPerL}</Mono> km/L
          </Fact>
          <Fact label="Weekly fuel quota">
            <Mono>{formatLitres(v.weeklyFuelQuotaL)}</Mono>
          </Fact>
        </dl>
      </Card>

      <p className="mt-4 max-w-3xl text-xs text-ink-muted">
        A vehicle is never deleted: routes and orders reference its id for
        history. To take it out of service, its availability is changed, not the
        record removed.
      </p>

      <div className="mt-6">
        <DriverAssignment vehicleId={v.vehicleId} depotId={v.depotId} />
      </div>
    </>
  );
}
