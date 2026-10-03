import type { DriverRoute } from '@waypoint/api-client';

/**
 * Resolves which route the cockpit shows.
 *
 * The schema links no driver to a vehicle, so the run cannot be derived
 * server-side. An explicit, remembered choice therefore wins. Failing that, a
 * single depot route is unambiguous and may be auto-selected. With more than
 * one candidate and no choice we return `undefined` so the UI asks the driver
 * — never the first unfinished route, which could belong to another vehicle.
 */
export function resolveSelectedRoute(
  routes: readonly DriverRoute[],
  storedRouteId: string | undefined,
): DriverRoute | undefined {
  if (storedRouteId) {
    const stored = routes.find((r) => r.routeId === storedRouteId);
    if (stored) return stored;
  }
  return routes.length === 1 ? routes[0] : undefined;
}
