import type { AuditRecord } from '@waypoint/shared-types';
import { auditActionLabel, auditFacts, auditSentence } from './audit-model';

const load: AuditRecord = {
  id: '1',
  actor: 'seed-loader',
  action: 'LOAD_RECORDED',
  entityType: 'LOAD_ITEM',
  entityId: 'OI1',
  outletId: 'OUT027',
  result: 'SUCCESS',
  detail: { routeId: 'R-123', loaded: 59, damaged: 1, missing: 2 },
  occurredAt: '2026-09-26T03:40:00+05:30',
};

describe('audit presentation', () => {
  it('names actions in words, never as raw codes', () => {
    expect(auditActionLabel('DELIVERY_RECORDED')).toBe('Delivery recorded');
    expect(auditActionLabel('SOMETHING_NEW')).toBe('Something new');
  });

  it('summarises a shortfall load in one sentence', () => {
    expect(auditSentence(load)).toBe(
      'Loader recorded a line at OUT027 with 2 missing and 1 damaged.',
    );
  });

  it('turns detail into labelled facts, linking a route id', () => {
    const facts = auditFacts(load);
    expect(facts).toContainEqual({
      label: 'Missing',
      value: '2',
      href: undefined,
    });
    expect(facts).toContainEqual({
      label: 'Route',
      value: 'R-123',
      href: '/dispatcher/tracker/R-123',
    });
  });

  it('shows enum values in words', () => {
    const delivery: AuditRecord = {
      ...load,
      action: 'DELIVERY_RECORDED',
      detail: { outcome: 'FAILED', orderStatus: 'FAILED' },
    };
    expect(auditSentence(delivery)).toBe(
      'Driver recorded the stop at OUT027 as failed.',
    );
    expect(auditFacts(delivery)).toContainEqual({
      label: 'Outcome',
      value: 'Failed',
      href: undefined,
    });
  });
});
