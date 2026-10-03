import type { DriverRoute } from '@waypoint/api-client';
import { resolveSelectedRoute } from './route-selection';

const route = (routeId: string, vehicleId: string): DriverRoute => ({
  routeId,
  routeDate: '2026-09-26',
  vehicleId,
  tripNo: 1,
  brand: 'FRESH',
  district: 'Colombo',
  status: 'DISPATCHED',
  stops: [],
});

describe('resolveSelectedRoute', () => {
  const routes = [route('r-veh001', 'VEH001'), route('r-veh014', 'VEH014')];

  it('uses the remembered choice, not the first route', () => {
    expect(resolveSelectedRoute(routes, 'r-veh014')?.vehicleId).toBe('VEH014');
  });

  it('never silently picks the first route when several exist', () => {
    expect(resolveSelectedRoute(routes, undefined)).toBeUndefined();
  });

  it('auto-selects only when the depot has exactly one route', () => {
    const single = [route('r-only', 'VEH014')];
    expect(resolveSelectedRoute(single, undefined)?.vehicleId).toBe('VEH014');
  });

  it('falls back to a single route when a stale choice no longer exists', () => {
    const single = [route('r-only', 'VEH014')];
    expect(resolveSelectedRoute(single, 'r-gone')?.vehicleId).toBe('VEH014');
  });

  it('ignores a stale choice while several routes exist', () => {
    expect(resolveSelectedRoute(routes, 'r-gone')).toBeUndefined();
  });
});
