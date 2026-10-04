'use client';

import Link from 'next/link';
import { use, useState } from 'react';
import type { LoadingLine, RouteLoading } from '@waypoint/api-client';
import { Mono } from '@waypoint/ui';
import { api } from '../../../../lib/api';
import {
  Card,
  Chip,
  EmptyState,
  ErrorState,
  Eyebrow,
  LoadingState,
  Meter,
  buttonClass,
} from '../../_components/ui';
import { FlagSheet, LineRow } from '../../_components/picking';
import type { Drafts, LineDraft } from '../../_lib/loading';
import {
  DOCK_LABEL,
  TEMP_LABEL,
  draftFor,
  fillPercent,
  progress,
  stopsInLoadOrder,
  submission,
} from '../../_lib/loading';
import { readableError, useRouteLoading } from '../../_lib/use-loading';

/**
 * L-02 · The picking list for one route.
 *
 * Stops are shown in LOAD order — the last drop first, so it goes deepest into
 * the vehicle and the first drop ends up by the door. The order never changes
 * under the loader's hands once the screen is open.
 *
 * Readiness is whatever the server says after a save. The screen never decides
 * on its own that a route is ready.
 */
export default function PickingListPage({
  params,
}: {
  params: Promise<{ routeId: string }>;
}) {
  const { routeId } = use(params);
  const state = useRouteLoading(routeId);

  if (state.status === 'loading') {
    return <LoadingState label="Loading the picking list…" />;
  }
  if (state.status === 'error') {
    return (
      <ErrorState
        title="Couldn't open this route"
        message={state.message}
        onRetry={state.reload}
      />
    );
  }
  return (
    <PickingList
      route={state.data}
      onSaved={state.replace}
      onReload={state.reload}
    />
  );
}

function PickingList({
  route,
  onSaved,
  onReload,
}: {
  route: RouteLoading;
  onSaved: (next: RouteLoading) => void;
  onReload: () => void;
}) {
  const [drafts, setDrafts] = useState<Drafts>(new Map());
  const [flagging, setFlagging] = useState<LoadingLine | null>(null);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saved, setSaved] = useState<string | null>(null);
  const [uploading, setUploading] = useState(false);
  const [uploadError, setUploadError] = useState<string | null>(null);

  const stops = stopsInLoadOrder(route.lines);
  const counted = progress(route.lines, drafts);
  const { items, error: blockingError } = submission(route.lines, drafts);

  const setDraft = (orderItemId: string, next: LineDraft) => {
    setSaved(null);
    setDrafts((prev) => new Map(prev).set(orderItemId, next));
  };

  /**
   * The media flow is the existing one: ask the server for a slot (it mints a
   * key scoped to this order line), PUT the bytes, then carry the returned
   * fileRef on the line. The photo is only referenced once the upload actually
   * succeeded — a failed upload never looks attached.
   */
  const uploadPhoto = async (line: LoadingLine, file: File) => {
    setUploading(true);
    setUploadError(null);
    try {
      const slot = await api.createShortfallUpload(line.orderItemId, file.type);
      const response = await fetch(slot.uploadUrl, {
        method: 'PUT',
        headers: { 'Content-Type': file.type, ...(slot.headers ?? {}) },
        body: file,
      });
      if (!response.ok) throw new Error('upload failed');
      const current = draftFor(line, drafts);
      setDraft(line.orderItemId, { ...current, photoRef: slot.fileRef });
    } catch (e) {
      setUploadError(readableError(e, 'The photo'));
    } finally {
      setUploading(false);
    }
  };

  /**
   * Save sends only changed lines. On failure the drafts are kept, so a loader
   * never loses counts they walked the bay to collect.
   */
  const save = async () => {
    if (blockingError) return;
    setSaving(true);
    setSaveError(null);
    try {
      const next = await api.recordShortfall(route.routeId, {
        items: items.map((line) => {
          const d = draftFor(line, drafts);
          return {
            orderItemId: line.orderItemId,
            loadedQty: d.loadedQty,
            damagedQty: d.damagedQty,
            missingQty: d.missingQty,
            ...(d.photoRef ? { photoRef: d.photoRef } : {}),
          };
        }),
      });
      onSaved(next);
      setDrafts(new Map());
      setSaved(
        next.routeReady
          ? 'Counts saved. The route is ready to leave.'
          : 'Counts saved. Some lines are still outstanding.',
      );
    } catch (e) {
      setSaveError(readableError(e, 'This route'));
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="flex flex-col gap-3 pb-28">
      <header className="flex flex-col gap-2 px-1 pt-2">
        <Link href="/loader" className="text-sm font-medium text-link">
          ← All trips
        </Link>
        <div className="flex items-start justify-between gap-2">
          <div>
            <Eyebrow>
              Trip {route.tripNo} · {route.brand} · {route.district}
            </Eyebrow>
            <h1 className="text-xl font-semibold text-ink">
              <Mono>{route.vehicleId}</Mono>
            </h1>
          </div>
          {route.routeReady ? (
            <Chip tone="success" glyph="✓">
              Ready
            </Chip>
          ) : (
            <Chip tone="neutral" glyph="○">
              In progress
            </Chip>
          )}
        </div>
      </header>

      <Card className="flex-row gap-4">
        <Meter
          label="Payload"
          value={`${fillPercent(route.loadedWeightKg, route.weightCapKg)}%`}
          detail={`${(route.loadedWeightKg / 1000).toFixed(2)} / ${(
            route.weightCapKg / 1000
          ).toFixed(2)} t`}
          percent={fillPercent(route.loadedWeightKg, route.weightCapKg)}
        />
        <Meter
          label="Volume"
          value={`${fillPercent(route.loadedVolumeM3, route.volumeCapM3)}%`}
          detail={`${route.loadedVolumeM3.toFixed(1)} / ${route.volumeCapM3.toFixed(
            1,
          )} m³`}
          percent={fillPercent(route.loadedVolumeM3, route.volumeCapM3)}
        />
        <Meter
          label="Lines"
          value={`${counted.complete}/${counted.total}`}
          detail={`${route.stops} ${route.stops === 1 ? 'stop' : 'stops'}`}
          percent={
            counted.total > 0
              ? Math.round((counted.complete / counted.total) * 100)
              : 0
          }
          tone={counted.shortfallQty > 0 ? 'warning' : 'normal'}
        />
      </Card>

      <p className="px-1 text-xs text-ink-muted">
        Loaded in reverse stop order — the last drop goes in first, nearest the
        cab.
      </p>

      {stops.length === 0 ? (
        <EmptyState title="Nothing on this route">
          This route has no order lines to load. Ask the dispatcher to check the
          plan.
        </EmptyState>
      ) : (
        <ol className="flex flex-col gap-3">
          {stops.map((stop, index) => (
            <li key={stop.seq}>
              <Card as="article">
                <div className="flex items-start justify-between gap-2">
                  <div className="flex flex-col gap-0.5">
                    <Eyebrow>
                      Load {index + 1} of {stops.length} · Stop {stop.seq}
                      {index === 0 && ' · deepest'}
                    </Eyebrow>
                    <p className="text-base font-semibold text-ink">
                      <Mono>{stop.outletId}</Mono> · {stop.outletName}
                    </p>
                    <p className="text-xs text-ink-muted">
                      {DOCK_LABEL[stop.dockType]} ·{' '}
                      {TEMP_LABEL[stop.tempRequirement]} ·{' '}
                      {stop.orderNumbers.map((n) => (
                        <Mono key={n}>{n}</Mono>
                      ))}
                    </p>
                  </div>
                  <Chip
                    tone={
                      stop.tempRequirement === 'AMBIENT' ? 'neutral' : 'info'
                    }
                    glyph={stop.tempRequirement === 'AMBIENT' ? '▢' : '❄'}
                  >
                    {TEMP_LABEL[stop.tempRequirement]}
                  </Chip>
                </div>

                <div className="flex flex-col gap-2">
                  {stop.lines.map((line) => (
                    <LineRow
                      key={line.orderItemId}
                      line={line}
                      draft={draftFor(line, drafts)}
                      touched={drafts.has(line.orderItemId)}
                      onChange={(next) => setDraft(line.orderItemId, next)}
                      onFlag={() => {
                        setUploadError(null);
                        setFlagging(line);
                      }}
                    />
                  ))}
                </div>
              </Card>
            </li>
          ))}
        </ol>
      )}

      {saveError && (
        <ErrorState
          title="Couldn't save your counts"
          message={`${saveError} Your counts are still here — try again.`}
          onRetry={onReload}
        />
      )}

      {/* Sticky action bar: the loader's hands are full, so the primary action
          never scrolls away. */}
      <div className="fixed inset-x-0 bottom-16 z-10 border-t border-line bg-card/95 p-3 backdrop-blur">
        <div className="mx-auto flex max-w-5xl flex-col gap-2">
          {saved && (
            <p className="text-sm font-medium text-success" role="status">
              {saved}
            </p>
          )}
          {!saved && blockingError && items.length > 0 && (
            <p className="text-xs text-ink-muted">{blockingError}</p>
          )}
          <button
            type="button"
            className={buttonClass('ink')}
            disabled={saving || Boolean(blockingError)}
            onClick={save}
          >
            {saving
              ? 'Saving…'
              : items.length > 0
                ? `Save ${items.length} ${items.length === 1 ? 'line' : 'lines'}`
                : 'Nothing to save'}
          </button>
        </div>
      </div>

      {flagging && (
        <FlagSheet
          line={flagging}
          draft={draftFor(flagging, drafts)}
          busy={uploading}
          uploadError={uploadError}
          onUploadPhoto={(file) => void uploadPhoto(flagging, file)}
          onRemovePhoto={() =>
            setDraft(flagging.orderItemId, {
              ...draftFor(flagging, drafts),
              photoRef: undefined,
            })
          }
          onCancel={() => setFlagging(null)}
          onApply={(next) => {
            setDraft(flagging.orderItemId, next);
            setFlagging(null);
          }}
        />
      )}
    </div>
  );
}
