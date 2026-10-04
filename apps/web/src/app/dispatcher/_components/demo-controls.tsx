'use client';

import { useCallback, useState } from 'react';
import {
  DEMO_STAGES,
  DEMO_STAGE_LABEL,
  type DemoStage,
} from '@waypoint/shared-types';
import { api } from '../../../lib/api';
import { describeApiError } from '../../../components/states';
import { ConfirmationDialog } from './confirmation-dialog';
import { useDispatcherScope } from './dispatcher-context';

function isDemoStage(value: string): value is DemoStage {
  return (DEMO_STAGES as readonly string[]).includes(value);
}

/**
 * Judge-walkthrough controls (CLAUDE.md §6, "The demo clock"). Rendered only
 * when the API reports demo mode; the endpoints themselves do not exist
 * otherwise. Jumping moves the API clock for every role, so the countdowns
 * re-sync from the response straight away. Reset wipes the operational data,
 * so it takes a deliberate confirmation step and then reloads the page so no
 * screen keeps showing rows that no longer exist.
 */
export function DemoControls({
  onReset = () => window.location.reload(),
}: {
  /** Called once a reset commits. Defaults to a full page reload. */
  onReset?: () => void;
}) {
  const { meta, reloadMeta } = useDispatcherScope();
  const [jumping, setJumping] = useState(false);
  const [jumpError, setJumpError] = useState<unknown>(null);
  const [lastStage, setLastStage] = useState<DemoStage | null>(null);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [resetting, setResetting] = useState(false);
  const [resetError, setResetError] = useState<unknown>(null);

  const jump = useCallback(
    async (stage: DemoStage) => {
      setJumping(true);
      setJumpError(null);
      try {
        await api.setDemoClock({ stage });
        setLastStage(stage);
        reloadMeta();
      } catch (err) {
        setJumpError(err);
      } finally {
        setJumping(false);
      }
    },
    [reloadMeta],
  );

  const reset = useCallback(async () => {
    setResetting(true);
    setResetError(null);
    try {
      await api.resetDemo();
      setConfirmOpen(false);
      onReset();
    } catch (err) {
      setResetError(err);
    } finally {
      setResetting(false);
    }
  }, [onReset]);

  const closeConfirm = useCallback(() => {
    setConfirmOpen(false);
    setResetError(null);
  }, []);

  if (!meta?.demoMode) return null;

  return (
    <div className="flex flex-wrap items-center gap-2 text-sm">
      <label className="flex items-center gap-2">
        <span className="text-ink-muted">Jump to</span>
        <select
          className="h-9 rounded-control bg-page px-2 text-sm font-medium text-ink ring-1 ring-ink/15"
          value=""
          disabled={jumping || resetting}
          aria-busy={jumping}
          onChange={(e) => {
            if (isDemoStage(e.target.value)) void jump(e.target.value);
          }}
        >
          <option value="">
            {jumping
              ? 'Moving clock…'
              : lastStage
                ? DEMO_STAGE_LABEL[lastStage]
                : 'Demo stage…'}
          </option>
          {DEMO_STAGES.map((s) => (
            <option key={s} value={s}>
              {DEMO_STAGE_LABEL[s]}
            </option>
          ))}
        </select>
      </label>
      <button
        type="button"
        className="tap-target rounded-control px-2 text-sm text-ink-muted ring-1 ring-ink/15 hover:bg-page hover:text-ink disabled:cursor-not-allowed"
        disabled={jumping || resetting}
        onClick={() => setConfirmOpen(true)}
      >
        Reset demo
      </button>
      {jumpError ? (
        <span role="alert" className="text-xs text-error">
          {describeApiError(jumpError).title}. The clock did not move.
        </span>
      ) : null}

      <ConfirmationDialog
        open={confirmOpen}
        eyebrow="Demo mode"
        title="Reset the demo day?"
        confirmLabel="Reset demo data"
        busyLabel="Resetting…"
        busy={resetting}
        onConfirm={() => void reset()}
        onCancel={closeConfirm}
      >
        <div className="flex flex-col gap-3">
          <p>
            Every order, plan, route, load count, delivery, deferral,
            notification and driver assignment is removed, and the 85 seeded
            orders for Sat 26 Sep return to <strong>Confirmed</strong> with the
            demo driver back on VEH014. The clock goes back to Fri 15:40.
          </p>
          <p className="text-ink-muted">
            Kept: accounts and sign-ins, the outlet, fleet and calendar
            reference data, and the audit trail — the reset itself is recorded
            there.
          </p>
          {resetError ? (
            <p role="alert" className="text-error">
              {describeApiError(resetError).title}. Nothing was changed.
            </p>
          ) : null}
        </div>
      </ConfirmationDialog>
    </div>
  );
}
