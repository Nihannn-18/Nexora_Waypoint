'use client';

import Link from 'next/link';
import { use, useState } from 'react';
import { useSearchParams } from 'next/navigation';
import { Mono } from '@waypoint/ui';
import type { Outlet } from '@waypoint/shared-types';
import { api } from '../../../../lib/api';
import { humanize } from '../../../../lib/format';
import { useApiQuery } from '../../../../lib/use-api-query';
import { ErrorState, LoadingState } from '../../../../components/states';
import { BrandChip, Fact, PageHeader, Card, SectionHeading } from '../../_components/ui';
import { IdentityFact, SavedNotice } from '../../_components/master-data';
import { useDispatcherScope } from '../../_components/dispatcher-context';
import { OutletForm } from '../_components/outlet-form';
import { ManagerAssignment } from './manager-assignment';

/**
 * Dispatcher · Outlet detail and edit. The identity is immutable and shown as
 * such; the operating fields are editable and revalidated server-side.
 */
export function OutletDetailScreen({
  params,
}: {
  params: Promise<{ outletId: string }>;
}) {
  const { outletId } = use(params);
  const search = useSearchParams();
  const saved = search.get('saved') === '1';
  const { depots } = useDispatcherScope();
  const [editing, setEditing] = useState(false);

  const outlet = useApiQuery(`master-outlet:${outletId}`, () =>
    api.getOutlet(outletId),
  );

  if (outlet.loading && !outlet.data) {
    return <LoadingState label="Loading outlet…" />;
  }
  if (outlet.error && !outlet.data) {
    return <ErrorState error={outlet.error} onRetry={outlet.reload} />;
  }
  const o = outlet.data as Outlet;
  if (!o) {
    return (
      <ErrorState
        error={new Error('This outlet was not found.')}
        onRetry={outlet.reload}
      />
    );
  }

  if (editing && depots) {
    return (
      <>
        <PageHeader
          title={`Edit ${o.outletId}`}
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
        <OutletForm outlet={o} depots={depots} />
      </>
    );
  }

  return (
    <>
      <div className="mb-2">
        <Link href="/dispatcher/outlets" className="text-sm font-medium text-link">
          ← All outlets
        </Link>
      </div>
      <PageHeader
        title={o.name}
        description={
          <>
            <Mono>{o.outletId}</Mono> · {o.district}
          </>
        }
        actions={
          <button
            type="button"
            onClick={() => setEditing(true)}
            className="tap-target inline-flex items-center gap-2 rounded-control bg-action px-4 text-sm font-semibold text-card hover:bg-action/90"
          >
            Edit outlet
          </button>
        }
      />

      {saved && <div className="mb-4"><SavedNotice>Outlet saved.</SavedNotice></div>}

      <SectionHeading>Outlet</SectionHeading>
      <Card>
        <dl className="grid grid-cols-2 gap-4 sm:grid-cols-3">
          <IdentityFact label="Outlet ID" id={o.outletId} />
          <Fact label="Brand">
            <BrandChip brand={o.brand} />
          </Fact>
          <Fact label="District">{o.district}</Fact>
          <Fact label="Home depot">
            <Mono>{o.depotId}</Mono>
          </Fact>
          <Fact label="Dock type">{humanize(o.dockType)}</Fact>
          <Fact label="Access">{humanize(o.parkingConstraint)}</Fact>
          <Fact label="Delivery window">
            <Mono>
              {o.windowOpenTime}–{o.windowCloseTime}
            </Mono>
          </Fact>
          <Fact label="Mall window">
            {o.mallWindow ? (
              <Mono>
                {o.mallWindow.open}–{o.mallWindow.close}
              </Mono>
            ) : (
              <span className="text-ink-muted">None</span>
            )}
          </Fact>
        </dl>
      </Card>

      <p className="mt-4 max-w-3xl text-xs text-ink-muted">
        An outlet is never deleted: orders, routes and delivery records reference
        its id. Changes affect future planning only; already-confirmed routes are
        not rewritten.
      </p>

      <div className="mt-6">
        <ManagerAssignment outletId={o.outletId} />
      </div>
    </>
  );
}
