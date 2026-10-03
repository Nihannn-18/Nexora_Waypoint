'use client';

import Link from 'next/link';
import { useRouter, useSearchParams } from 'next/navigation';
import { useCallback, useMemo, useState } from 'react';
import { WaypointApiError } from '@waypoint/api-client';
import { Mono, StatusBadge } from '@waypoint/ui';
import {
  CONSTRAINT_CATALOG,
  DESIGN_RULE_GROUPS,
  PRIORITISATION_POLICY,
  type ConfirmAllocationResponse,
  type DesignRuleId,
  type PlanningJob,
} from '@waypoint/shared-types';
import { api } from '../../../lib/api';
import { formatDateTime, formatDay, plural } from '../../../lib/format';
import { useApiQuery } from '../../../lib/use-api-query';
import {
  EmptyState,
  ErrorState,
  LoadingState,
  describeApiError,
} from '../../../components/states';
import { useDispatcherScope } from '../_components/dispatcher-context';
import { fetchAllOrders, indexBy, loadOutlets } from '../_components/data';
import { ConfirmationDialog } from '../_components/confirmation-dialog';
import {
  ButtonLink,
  Card,
  PageHeader,
  SectionHeading,
  buttonClass,
} from '../_components/ui';
import { DeferralPanel } from './deferral-panel';
import { TripCard } from './trip-card';
import {
  buildConfirmRequest,
  buildPlanView,
  ordersMissingReason,
  type DeferralDecision,
  type PlanView,
} from './plan-model';

/**
 * D-03 / DG-A / D-04 / D-05 — plan a delivery day.
 *
 * 1. Run planning for the depot and day (POST /allocations/suggest → 202).
 * 2. Review the proposal: trips by vehicle, and the orders it defers and why.
 * 3. Give every deferral a reason (Confirm stays disabled until then).
 * 4. Confirm — the only step that creates routes, legs and allocations.
 *
 * The job id lives in the URL (?job=), so a reload keeps the proposal.
 */
export function PlanScreen() {
  const params = useSearchParams();
  const jobId = params.get('job');
  return jobId ? <PlanReview jobId={jobId} /> : <PlanStart />;
}

const STEPS = [
  'Run planning',
  'Review proposal',
  'Give deferral reasons',
  'Confirm plan',
];

function Stepper({ current }: { current: number }) {
  return (
    <ol className="mb-5 flex flex-wrap gap-2" aria-label="Planning steps">
      {STEPS.map((label, i) => {
        const state = i < current ? 'done' : i === current ? 'current' : 'todo';
        return (
          <li
            key={label}
            aria-current={state === 'current' ? 'step' : undefined}
            className={`flex items-center gap-2 rounded-pill px-3 py-1 text-xs font-medium ring-1 ${
              state === 'current'
                ? 'bg-action text-card ring-action'
                : state === 'done'
                  ? 'bg-success/10 text-success ring-success/30'
                  : 'bg-card text-ink-muted ring-ink/15'
            }`}
          >
            <span aria-hidden="true">{state === 'done' ? '✓' : i + 1}</span>
            {label}
          </li>
        );
      })}
    </ol>
  );
}

/* -------------------------------------------------------------------------- */
/* Step 1 — run planning                                                       */
/* -------------------------------------------------------------------------- */

/** Poll a job until it leaves QUEUED/RUNNING. The run is fast; cap the wait. */
async function waitForJob(jobId: string): Promise<PlanningJob> {
  for (let attempt = 0; attempt < 60; attempt++) {
    const job = await api.getPlanningJob(jobId);
    if (job.status === 'COMPLETED' || job.status === 'FAILED') return job;
    await new Promise((r) => setTimeout(r, 1000));
  }
  throw new Error('The planning job is still running after a minute.');
}

function PlanStart() {
  const router = useRouter();
  const { depot, deliveryDate } = useDispatcherScope();
  const ready = depot && deliveryDate;
  const scopeKey = ready ? `${depot.depotId}:${deliveryDate}` : null;

  const eligible = useApiQuery(scopeKey && `eligible:${scopeKey}`, () =>
    api.listOrders({
      deliveryDate,
      depotId: depot?.depotId,
      status: 'CONFIRMED',
      limit: 1,
    }),
  );
  const routes = useApiQuery(scopeKey && `routes:${scopeKey}`, () =>
    api.listRoutes({ date: deliveryDate as string, depotId: depot?.depotId }),
  );

  const [running, setRunning] = useState(false);
  const [runError, setRunError] = useState<unknown>(null);

  const run = async () => {
    if (!depot || !deliveryDate) return;
    setRunning(true);
    setRunError(null);
    try {
      const { jobId } = await api.suggestPlan({
        planningDate: deliveryDate,
        depotId: depot.depotId,
      });
      await waitForJob(jobId);
      router.replace(`/dispatcher/plan?job=${encodeURIComponent(jobId)}`);
    } catch (error) {
      setRunError(error);
      setRunning(false);
    }
  };

  const eligibleCount = eligible.data?.total;
  const confirmedTrips = routes.data?.length ?? 0;

  return (
    <>
      <PageHeader
        screenId="D-03"
        title="Plan & allocate"
        description="The engine proposes trips that pass every hard rule and names the rule behind each deferral. Nothing is allocated until you confirm."
      />
      <Stepper current={0} />
      {!ready ? (
        <LoadingState label="Loading depots and the API clock…" />
      ) : (
        <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_320px]">
          <Card>
            <h2 className="text-base font-semibold text-ink">
              {depot.name} · {formatDay(deliveryDate)}
            </h2>
            {eligible.error ? (
              <div className="mt-3">
                <ErrorState
                  error={eligible.error}
                  onRetry={eligible.reload}
                  compact
                />
              </div>
            ) : (
              <p className="mt-1 text-sm text-ink-muted">
                {eligibleCount === undefined
                  ? 'Counting confirmed orders…'
                  : eligibleCount === 0
                    ? 'No confirmed orders are waiting for this day, so there is nothing to plan.'
                    : `${plural(eligibleCount, 'confirmed order')} waiting for this day will be considered. Orders still PLACED are not planned until the store confirms them.`}
              </p>
            )}

            {confirmedTrips > 0 && (
              <p className="mt-3 rounded-control bg-success/10 p-3 text-sm text-ink">
                <span className="font-semibold text-success">
                  ✓ Already confirmed:
                </span>{' '}
                {plural(confirmedTrips, 'trip')} exist for this day. A new run
                only plans orders that are still waiting.{' '}
                <Link
                  href="/dispatcher/tracker"
                  className="font-medium text-link hover:underline"
                >
                  View routes
                </Link>
              </p>
            )}

            {runError ? (
              <div className="mt-3">
                <ErrorState error={runError} compact />
              </div>
            ) : null}

            <div className="mt-4 flex flex-wrap items-center gap-3">
              <button
                type="button"
                className={buttonClass.primary}
                onClick={run}
                disabled={running || !eligibleCount}
                aria-busy={running}
              >
                {running ? 'Planning…' : 'Run planning'}
              </button>
              <span className="text-xs text-ink-muted">
                Writes a proposal only — routes are created when you confirm.
              </span>
            </div>
          </Card>
          <HowTheEngineChose />
        </div>
      )}
    </>
  );
}

/* -------------------------------------------------------------------------- */
/* Steps 2–4 — review, reasons, confirm                                        */
/* -------------------------------------------------------------------------- */

type ConfirmState =
  | { kind: 'idle' }
  | { kind: 'dialog' }
  | { kind: 'submitting' }
  | { kind: 'error'; error: unknown }
  | { kind: 'done'; result: ConfirmAllocationResponse };

function PlanReview({ jobId }: { jobId: string }) {
  const router = useRouter();
  const { depots } = useDispatcherScope();

  const results = useApiQuery(`results:${jobId}`, () =>
    api.getPlanningResults(jobId),
  );
  const job = results.data?.job;
  const enrichKey =
    job?.status === 'COMPLETED' ? `${job.depotId}:${job.planningDate}` : null;

  const reference = useApiQuery(
    enrichKey && `plan-ref:${enrichKey}`,
    async () => {
      const [orders, vehicles, outlets] = await Promise.all([
        fetchAllOrders({
          deliveryDate: job?.planningDate,
          depotId: job?.depotId,
        }),
        api.getVehicles({ depotId: job?.depotId, date: job?.planningDate }),
        loadOutlets(job?.depotId),
      ]);
      return {
        orders: indexBy(orders, 'orderId'),
        vehicles: indexBy(vehicles, 'vehicleId'),
        outlets: indexBy(outlets, 'outletId'),
        availableVehicles: vehicles.filter((v) => v.status === 'AVAILABLE')
          .length,
      };
    },
  );

  const view: PlanView | undefined = useMemo(
    () =>
      results.data && reference.data
        ? buildPlanView(
            results.data.proposals,
            reference.data.orders,
            reference.data.outlets,
            reference.data.vehicles,
          )
        : undefined,
    [results.data, reference.data],
  );

  const [decisions, setDecisions] = useState<
    ReadonlyMap<string, DeferralDecision>
  >(() => new Map());
  const decide = useCallback(
    (updates: ReadonlyMap<string, DeferralDecision>) => {
      setDecisions((prev) => new Map([...prev, ...updates]));
    },
    [],
  );

  const [confirm, setConfirm] = useState<ConfirmState>({ kind: 'idle' });

  if (results.loading)
    return <LoadingState label="Loading the planning proposal…" />;
  if (results.error || !results.data || !job) {
    return (
      <>
        <PageHeader screenId="D-03" title="Plan & allocate" />
        <ErrorState error={results.error} onRetry={results.reload} />
        <div className="mt-4">
          <ButtonLink href="/dispatcher/plan">
            Start a new planning run
          </ButtonLink>
        </div>
      </>
    );
  }

  const depotName =
    depots?.find((d) => d.depotId === job.depotId)?.name ?? 'Depot';
  const header = (
    <PageHeader
      screenId="D-03"
      title={`Plan for ${formatDay(job.planningDate)} · ${depotName}`}
      description={
        <>
          Proposal <Mono>{job.jobId}</Mono>, planned{' '}
          {formatDateTime(job.createdAt)}. A proposal is never an allocation:
          routes are created only when you confirm.
        </>
      }
      actions={
        <button
          type="button"
          className={buttonClass.secondary}
          onClick={() => router.replace('/dispatcher/plan')}
          disabled={confirm.kind === 'submitting'}
        >
          Start a new run
        </button>
      }
    />
  );

  if (job.status === 'FAILED') {
    return (
      <>
        {header}
        <ErrorState
          error={new Error(job.errorMessage ?? 'The planning run failed.')}
          onRetry={() => router.replace('/dispatcher/plan')}
        />
      </>
    );
  }
  if (job.status !== 'COMPLETED') {
    return (
      <>
        {header}
        <LoadingState label={`Planning is ${job.status.toLowerCase()}…`} />
        <button
          type="button"
          className={`${buttonClass.secondary} mt-3`}
          onClick={results.reload}
        >
          Check again
        </button>
      </>
    );
  }
  if (reference.error) {
    return (
      <>
        {header}
        <ErrorState error={reference.error} onRetry={reference.reload} />
      </>
    );
  }
  if (!view || !reference.data) {
    return (
      <>
        {header}
        <LoadingState label="Loading orders, outlets and vehicles for this proposal…" />
      </>
    );
  }

  if (view.ordersTotal === 0) {
    return (
      <>
        {header}
        <EmptyState
          title="This run had no orders to plan"
          action={
            <ButtonLink href="/dispatcher/queue">Review orders</ButtonLink>
          }
        >
          Only CONFIRMED orders for the day are planned. Check that stores have
          confirmed their orders, then run planning again.
        </EmptyState>
      </>
    );
  }

  const missing = ordersMissingReason(view.deferred, decisions);
  const done = confirm.kind === 'done';
  // Every order moved on since planning: this proposal has been confirmed.
  const alreadyConfirmed =
    !done && view.changedSincePlanning.length === view.ordersTotal;
  const stale =
    !done && !alreadyConfirmed && view.changedSincePlanning.length > 0;
  const locked = done || alreadyConfirmed;
  const step = locked ? 4 : missing.length > 0 ? 2 : 3;

  const submit = async () => {
    setConfirm({ kind: 'submitting' });
    try {
      const result = await api.confirmAllocation(
        buildConfirmRequest(job.jobId, view, decisions),
      );
      setConfirm({ kind: 'done', result });
      reference.reload();
    } catch (error) {
      setConfirm({ kind: 'error', error });
      reference.reload();
    }
  };

  const served = view.ordersTotal - view.deferred.length;
  const topConstraint = view.deferralsByConstraint[0]?.code;

  return (
    <>
      {header}
      <Stepper current={step} />

      <dl className="mb-4 flex flex-wrap gap-2">
        {[
          ['Orders planned', view.ordersTotal],
          ['Proposed to serve', served],
          ['Proposed to defer', view.deferred.length],
          ['Trips', view.trips.length],
          [
            'Vehicles used',
            `${view.vehiclesUsed} / ${reference.data.availableVehicles} available`,
          ],
        ].map(([label, value]) => (
          <div
            key={label}
            className="rounded-control bg-card px-3 py-1.5 text-sm ring-1 ring-ink/10"
          >
            <dt className="inline text-ink-muted">{label}: </dt>
            <dd className="inline font-mono font-semibold text-ink">{value}</dd>
          </div>
        ))}
      </dl>

      {done && (
        <ConfirmedBanner result={confirm.result} date={job.planningDate} />
      )}
      {alreadyConfirmed && (
        <div
          role="status"
          className="mb-4 rounded-card bg-success/10 p-4 text-sm ring-1 ring-success/30"
        >
          <p className="font-semibold text-success">
            ✓ This plan has already been confirmed
          </p>
          <p className="mt-1 text-ink">
            Every order in it has moved on from CONFIRMED, so it cannot be sent
            twice.{' '}
            <Link
              href="/dispatcher/tracker"
              className="font-medium text-link hover:underline"
            >
              View routes
            </Link>{' '}
            ·{' '}
            <Link
              href="/dispatcher/deferrals"
              className="font-medium text-link hover:underline"
            >
              View deferrals
            </Link>
          </p>
        </div>
      )}
      {stale && (
        <div
          role="alert"
          className="mb-4 rounded-card bg-warning/10 p-4 text-sm ring-1 ring-warning/30"
        >
          <p className="font-semibold text-ink">
            ! {plural(view.changedSincePlanning.length, 'order')} changed since
            this proposal was made
          </p>
          <p className="mt-1 text-ink-muted">
            They were allocated, deferred or otherwise updated after planning,
            so the API would reject this confirmation. Run planning again to
            plan from current data.
          </p>
        </div>
      )}

      {view.deferred.length > 0 && !locked && (
        <div className="mb-4 rounded-card bg-warning/10 p-4 ring-1 ring-warning/30">
          <p className="font-semibold text-ink">
            <span aria-hidden="true" className="text-warning">
              ▲{' '}
            </span>
            {view.deferred.length} of {view.ordersTotal} orders can’t be served
            on {formatDay(job.planningDate)}
            {topConstraint && (
              <>
                {' '}
                — {CONSTRAINT_CATALOG[topConstraint].label.toLowerCase()} is the
                main limit
              </>
            )}
          </p>
          <p className="mt-1 text-sm text-ink-muted">
            Each deferral names the rule that blocked it. Give each one a reason
            the store will read, then confirm.{' '}
            <a
              href="#deferrals"
              className="font-medium text-link hover:underline"
            >
              Go to deferrals
            </a>
          </p>
        </div>
      )}

      <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_320px]">
        <div className="min-w-0 space-y-6">
          <section aria-labelledby="trips-heading">
            <SectionHeading
              aside={
                <span className="text-xs text-ink-muted">
                  Sorted by vehicle
                </span>
              }
            >
              <span id="trips-heading">
                Proposed trips ({view.trips.length})
              </span>
            </SectionHeading>
            {view.trips.length === 0 ? (
              <EmptyState title="No trip could be formed">
                Every order was deferred. The rule panel shows which constraints
                left no feasible slot.
              </EmptyState>
            ) : (
              <TripList trips={view.trips} />
            )}
          </section>

          <section
            id="deferrals"
            aria-labelledby="deferrals-heading"
            className="scroll-mt-24"
          >
            <SectionHeading
              aside={
                view.deferred.length > 0 && !locked ? (
                  missing.length > 0 ? (
                    <StatusBadge tone="warning">
                      {missing.length} still need a reason
                    </StatusBadge>
                  ) : (
                    <StatusBadge tone="success">
                      Every deferral has a reason
                    </StatusBadge>
                  )
                ) : undefined
              }
            >
              <span id="deferrals-heading">
                Proposed deferrals ({view.deferred.length})
              </span>
            </SectionHeading>
            {view.deferred.length === 0 ? (
              <EmptyState title="Every order fits">
                The engine found a feasible trip for every confirmed order on
                this day.
              </EmptyState>
            ) : (
              <DeferralPanel
                deferred={view.deferred}
                decisions={decisions}
                onDecide={decide}
                readOnly={locked || confirm.kind === 'submitting'}
              />
            )}
          </section>
        </div>

        <aside className="space-y-4 xl:sticky xl:top-24 xl:self-start">
          <RulePanel view={view} />
          <HowTheEngineChose />
        </aside>
      </div>

      {!locked && (
        <div className="sticky bottom-0 z-10 -mx-4 mt-6 border-t border-ink/10 bg-card/95 px-4 py-3 backdrop-blur lg:-mx-6 lg:px-6">
          {confirm.kind === 'error' && (
            <div className="mb-3">
              <ConfirmError error={confirm.error} />
            </div>
          )}
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-sm text-ink">
              {missing.length > 0 ? (
                <>
                  <span className="font-semibold text-warning">!</span>{' '}
                  {plural(missing.length, 'deferral')} still need a reason
                  before you can confirm.
                </>
              ) : stale ? (
                'This proposal is out of date. Run planning again before confirming.'
              ) : (
                <>
                  Ready: {plural(view.trips.length, 'route')},{' '}
                  {plural(served, 'order')} allocated,{' '}
                  {plural(view.deferred.length, 'order')} deferred with reasons.
                </>
              )}
            </p>
            <button
              type="button"
              className={buttonClass.primary}
              disabled={
                missing.length > 0 || stale || confirm.kind === 'submitting'
              }
              onClick={() => setConfirm({ kind: 'dialog' })}
            >
              Review & confirm plan
            </button>
          </div>
        </div>
      )}

      <ConfirmationDialog
        open={confirm.kind === 'dialog' || confirm.kind === 'submitting'}
        eyebrow="Confirm allocation"
        title={`Confirm the plan for ${formatDay(job.planningDate)}?`}
        confirmLabel={`Confirm ${plural(view.trips.length, 'route')}`}
        busyLabel="Confirming…"
        busy={confirm.kind === 'submitting'}
        onConfirm={submit}
        onCancel={() => setConfirm({ kind: 'idle' })}
      >
        <ConfirmSummary view={view} depotName={depotName} />
      </ConfirmationDialog>
    </>
  );
}

function TripList({ trips }: { trips: PlanView['trips'] }) {
  const [brand, setBrand] = useState<'ALL' | 'FRESH' | 'STYLE' | 'TECH'>('ALL');
  const shown =
    brand === 'ALL' ? trips : trips.filter((t) => t.brand === brand);
  const count = (b: string) => trips.filter((t) => t.brand === b).length;
  return (
    <>
      <div
        role="group"
        aria-label="Filter trips by brand"
        className="mb-3 flex flex-wrap gap-1.5"
      >
        {(['ALL', 'FRESH', 'STYLE', 'TECH'] as const).map((b) => (
          <button
            key={b}
            type="button"
            aria-pressed={brand === b}
            onClick={() => setBrand(b)}
            className={`h-8 rounded-control px-3 text-xs font-medium ring-1 ${
              brand === b
                ? 'bg-action text-card ring-action'
                : 'bg-card text-ink ring-ink/15 hover:bg-page'
            }`}
          >
            {b === 'ALL'
              ? `All (${trips.length})`
              : `${b.charAt(0)}${b.slice(1).toLowerCase()} (${count(b)})`}
          </button>
        ))}
      </div>
      <div className="space-y-3">
        {shown.map((t) => (
          <TripCard key={t.key} trip={t} />
        ))}
      </div>
    </>
  );
}

/** The D-03 rule panel: every rule group with its E-0x id and what it blocked. */
function RulePanel({ view }: { view: PlanView }) {
  const blockedByGroup = new Map<DesignRuleId, number>();
  for (const { code, count } of view.deferralsByConstraint) {
    if (!code) continue;
    const group = CONSTRAINT_CATALOG[code].designRuleId;
    blockedByGroup.set(group, (blockedByGroup.get(group) ?? 0) + count);
  }
  return (
    <Card>
      <h2 className="text-sm font-semibold text-ink">Hard rules</h2>
      <p className="mt-0.5 text-xs text-ink-muted">
        Every proposed trip passes all of these. On confirm the API re-checks
        each order’s status, the vehicle’s depot and the one-brand-one-district
        rule inside the transaction that writes the routes.
      </p>
      <ul className="mt-3 space-y-2">
        {(Object.keys(DESIGN_RULE_GROUPS) as DesignRuleId[]).map((id) => {
          const blocked = blockedByGroup.get(id) ?? 0;
          return (
            <li key={id} className="flex items-start gap-2 text-sm">
              <Mono className="mt-0.5 rounded-chip bg-ink/[0.06] px-1 text-xs font-semibold text-ink">
                {id}
              </Mono>
              <span className="min-w-0 flex-1 text-ink">
                {DESIGN_RULE_GROUPS[id]}
              </span>
              {blocked > 0 ? (
                <StatusBadge tone="warning">{blocked} deferred</StatusBadge>
              ) : (
                <StatusBadge tone="success">Clear</StatusBadge>
              )}
            </li>
          );
        })}
      </ul>
    </Card>
  );
}

/** "How the engine chose" — the documented fairness order, applied after feasibility. */
function HowTheEngineChose() {
  return (
    <Card>
      <h2 className="text-sm font-semibold text-ink">How the engine chose</h2>
      <p className="mt-0.5 text-xs text-ink-muted">
        Feasibility first. When there is not room for everyone, orders are
        ranked in this order:
      </p>
      <ol className="mt-2 space-y-1.5 text-sm text-ink">
        {PRIORITISATION_POLICY.map((p) => (
          <li key={p.key} className="flex gap-2">
            <Mono className="text-xs text-ink-muted">{p.rank}.</Mono>
            <span>{p.rule}</span>
          </li>
        ))}
      </ol>
    </Card>
  );
}

function ConfirmSummary({
  view,
  depotName,
}: {
  view: PlanView;
  depotName: string;
}) {
  const served = view.ordersTotal - view.deferred.length;
  return (
    <div className="space-y-3">
      <p>
        This creates <strong>{plural(view.trips.length, 'route')}</strong> at{' '}
        {depotName}, allocates <strong>{plural(served, 'order')}</strong> and
        records <strong>{plural(view.deferred.length, 'deferral')}</strong> with
        your reasons — in one transaction. Loaders can start from these routes
        as soon as it succeeds.
      </p>
      <p className="rounded-control bg-warning/10 p-2.5 text-ink">
        <span className="font-semibold">
          ! This cannot be undone from this screen.
        </span>{' '}
        A second confirmation of the same plan is refused, never duplicated.
      </p>
      <div>
        <p className="text-xs font-semibold uppercase tracking-wide text-ink-muted">
          Vehicle / trip assignments
        </p>
        <ul className="mt-1 max-h-48 space-y-0.5 overflow-y-auto font-mono text-xs">
          {view.trips.map((t) => (
            <li key={t.key}>
              {t.vehicleId} · trip {t.tripNo} · {t.brand ?? '—'} ·{' '}
              {t.district ?? '—'} · {plural(t.stops.length, 'stop')}
            </li>
          ))}
        </ul>
      </div>
      {view.deferralsByConstraint.length > 0 && (
        <div>
          <p className="text-xs font-semibold uppercase tracking-wide text-ink-muted">
            Deferrals by rule
          </p>
          <ul className="mt-1 space-y-0.5 text-xs">
            {view.deferralsByConstraint.map(({ code, count }) => (
              <li key={code ?? 'none'}>
                {code
                  ? `${CONSTRAINT_CATALOG[code].designRuleId} ${CONSTRAINT_CATALOG[code].label}`
                  : 'No rule named'}
                : {count}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}

function ConfirmedBanner({
  result,
  date,
}: {
  result: ConfirmAllocationResponse;
  date: string;
}) {
  return (
    <div
      role="status"
      className="mb-4 rounded-card bg-success/10 p-4 ring-1 ring-success/30"
    >
      <p className="font-semibold text-success">
        ✓ Plan confirmed for {formatDay(date)}
      </p>
      <p className="mt-1 text-sm text-ink">
        {plural(result.routeIds.length, 'route')} created,{' '}
        {plural(result.allocatedOrders.length, 'order')} allocated,{' '}
        {plural(result.deferredOrders.length, 'order')} deferred and logged with
        your reasons. Loaders now see these routes; the deferral log holds each
        reason.
      </p>
      <div className="mt-3 flex flex-wrap gap-2">
        <ButtonLink href="/dispatcher/tracker" variant="primary">
          View routes
        </ButtonLink>
        <ButtonLink href="/dispatcher/deferrals">View deferral log</ButtonLink>
      </div>
    </div>
  );
}

function ConfirmError({ error }: { error: unknown }) {
  if (error instanceof WaypointApiError && error.status === 409) {
    return (
      <div
        role="alert"
        className="rounded-card bg-error/5 p-3 text-sm ring-1 ring-error/25"
      >
        <p className="font-semibold text-ink">
          Not confirmed — the plan conflicts with current data
        </p>
        <p className="mt-0.5 text-ink-muted">
          {error.message}. It may already have been confirmed, or an order in it
          was allocated since planning. Nothing was written. Run planning again
          to plan from current data.
        </p>
      </div>
    );
  }
  const { title, detail } = describeApiError(error);
  return (
    <div
      role="alert"
      className="rounded-card bg-error/5 p-3 text-sm ring-1 ring-error/25"
    >
      <p className="font-semibold text-ink">
        Not confirmed — {title.toLowerCase()}
      </p>
      <p className="mt-0.5 text-ink-muted">{detail} Nothing was written.</p>
    </div>
  );
}
