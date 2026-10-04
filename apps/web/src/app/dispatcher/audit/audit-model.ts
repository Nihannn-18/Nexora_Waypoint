import type { AuditRecord } from '@waypoint/shared-types';
import { humanize } from '../../../lib/format';

/**
 * Turns an immutable audit record into sentences and facts a dispatcher can
 * read. Raw JSON is never the primary view; unknown keys still appear, as
 * plain "Label: value" pairs, so nothing recorded is hidden.
 */

const ACTION_LABEL: Record<string, string> = {
  ORDER_CREATED: 'Order placed',
  ORDER_CONFIRMED: 'Order confirmed',
  PLANNING_PROPOSED: 'Plan proposed',
  ROUTE_CONFIRMED: 'Route confirmed',
  ALLOCATION_DECIDED: 'Allocation decided',
  LOAD_RECORDED: 'Load recorded',
  SHORTFALL_RECORDED: 'Shortfall recorded',
  DELIVERY_RECORDED: 'Delivery recorded',
  SYNC_PROCESSED: 'Offline sync processed',
};

export function auditActionLabel(action: string): string {
  return ACTION_LABEL[action] ?? humanize(action);
}

const ENTITY_LABEL: Record<string, string> = {
  ORDER: 'Order',
  PLANNING_JOB: 'Planning job',
  ROUTE: 'Route',
  ALLOCATION: 'Allocation',
  LOAD_ITEM: 'Order line',
  DELIVERY_EVENT: 'Delivery event',
  SYNC_BATCH: 'Sync batch',
};

export function auditEntityLabel(entityType: string): string {
  return ENTITY_LABEL[entityType] ?? humanize(entityType);
}

export interface AuditFact {
  readonly label: string;
  readonly value: string;
  /** Set when the value is an id the dispatcher can open. */
  readonly href?: string;
}

const KEY_LABEL: Record<string, string> = {
  outcome: 'Outcome',
  orderStatus: 'Order now',
  clientEventId: 'Device event',
  routeId: 'Route',
  loaded: 'Loaded',
  damaged: 'Damaged',
  missing: 'Missing',
};

/** Keys whose values are enum codes, shown in words. */
const ENUM_KEYS = new Set(['outcome', 'orderStatus']);

function show(value: unknown): string {
  if (value === null || value === undefined) return '—';
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'boolean')
    return String(value);
  // A nested structure: summarise rather than dump.
  if (Array.isArray(value))
    return `${value.length} item${value.length === 1 ? '' : 's'}`;
  return Object.entries(value as Record<string, unknown>)
    .map(([k, v]) => `${humanize(k)} ${show(v)}`)
    .join(', ');
}

export function auditFacts(record: AuditRecord): AuditFact[] {
  return Object.entries(record.detail ?? {}).map(([key, value]) => {
    const text = show(value);
    return {
      label:
        KEY_LABEL[key] ?? humanize(key.replace(/([a-z])([A-Z])/g, '$1_$2')),
      value:
        ENUM_KEYS.has(key) && typeof value === 'string'
          ? humanize(value)
          : text,
      href:
        key === 'routeId' && typeof value === 'string'
          ? `/dispatcher/tracker/${value}`
          : undefined,
    };
  });
}

/** One plain sentence for the timeline row. */
export function auditSentence(record: AuditRecord): string {
  const d = record.detail ?? {};
  const where = record.outletId ? ` at ${record.outletId}` : '';
  switch (record.action) {
    case 'DELIVERY_RECORDED': {
      const outcome =
        typeof d['outcome'] === 'string'
          ? humanize(d['outcome']).toLowerCase()
          : 'recorded';
      return `Driver recorded the stop${where} as ${outcome}.`;
    }
    case 'LOAD_RECORDED': {
      const missing = Number(d['missing'] ?? 0);
      const damaged = Number(d['damaged'] ?? 0);
      const short = missing + damaged;
      return short > 0
        ? `Loader recorded a line${where} with ${missing} missing and ${damaged} damaged.`
        : `Loader recorded a line${where} as fully loaded.`;
    }
    default:
      return `${auditActionLabel(record.action)}${where}.`;
  }
}
