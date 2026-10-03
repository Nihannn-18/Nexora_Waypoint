/**
 * @jest-environment node
 */
import { WaypointApiError } from '@waypoint/api-client';
import { loadError } from './use-outbox';

const err = (status: number, isOffline = false) =>
  new WaypointApiError('x', { status, isOffline });

it('names the real cause of a failed driver read — never a fake success', () => {
  expect(loadError(err(0, true), 'saved offline?', 'this stop')).toBe(
    'saved offline?',
  );
  expect(loadError(err(401), '', 'this stop')).toMatch(/Sign in again/);
  expect(loadError(err(403), '', 'this stop')).toMatch(/your depot/);
  expect(loadError(err(404), '', 'this stop')).toMatch(/your depot/);
  expect(loadError(err(500), '', 'this stop')).toMatch(/Try again/);
});
