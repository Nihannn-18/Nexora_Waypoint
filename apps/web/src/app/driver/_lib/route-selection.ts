import type { DriverRoute } from '@waypoint/api-client';

/**
 * Resolves which route the cockpit shows.
 *
 * When the Dispatcher has assigned the driver to a vehicle for the run date,
 * the server already narrows `GET /driver/routes` to that vehicle, so the list
 * is either one trip (auto-selected) or the driver's two trips. An explicit,
 * remembered choice still wins, and with more than one candidate and no choice
 * we return `undefined` so the UI asks the driver — never the first unfinished
 * route, which could belong to another vehicle.
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
