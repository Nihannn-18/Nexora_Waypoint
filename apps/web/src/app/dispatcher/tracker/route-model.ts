import {
  timeBudgetForBrand,
  type Route,
  type Vehicle,
} from '@waypoint/shared-types';

/**
 * Read-side figures for a confirmed route. Distance and the time breakdown are
 * the server's (official formula); fuel is the documented
 * `fuel_litres = route_distance_km / km_per_l`, computed for display only.
 */
export interface RouteFigures {
  readonly stopsTotal: number;
  readonly stopsDelivered: number;
  readonly stopsFailed: number;
  readonly fuelLitres: number | undefined;
  /** The vehicle's minutes that day in this route's budget pool. */
  readonly vehicleDayMinutes: number;
  readonly budgetMinutes: number;
}

export function routeFigures(
  route: Route,
  vehicle: Vehicle | undefined,
  sameDayRoutes: readonly Route[],
): RouteFigures {
  const legs = route.legs ?? [];
  const fresh = route.brand === 'FRESH';
  const vehicleDayMinutes = sameDayRoutes
    .filter(
      (r) => r.vehicleId === route.vehicleId && (r.brand === 'FRESH') === fresh,
    )
    .reduce((sum, r) => sum + r.totalTripMin, 0);
  return {
    stopsTotal: legs.length,
    stopsDelivered: legs.filter((l) => l.status === 'DELIVERED').length,
    stopsFailed: legs.filter((l) => l.status === 'FAILED').length,
    fuelLitres:
      vehicle && vehicle.kmPerL > 0
        ? route.distanceKm / vehicle.kmPerL
        : undefined,
    vehicleDayMinutes: vehicleDayMinutes || route.totalTripMin,
    budgetMinutes: timeBudgetForBrand(route.brand),
  };
}
