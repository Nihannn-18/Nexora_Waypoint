import type { Role } from '@waypoint/shared-types';

/**
 * Where each role lands after signing in, and the device the design assumes.
 *
 * No account identity lives here: this file ships to the browser, and the
 * seeded demo accounts are documented in the README, not in the UI.
 */
export interface RoleRoute {
  readonly role: Role;
  readonly label: string;
  readonly href: string;
  /** The viewport the screens for this role were designed at. */
  readonly surface: 'desktop' | 'tablet' | 'phone';
  readonly summary: string;
}

export const ROLE_ROUTES: readonly RoleRoute[] = [
  {
    role: 'DISPATCHER',
    label: 'Dispatcher',
    href: '/dispatcher',
    surface: 'desktop',
    summary: 'Close the queue, build the plan, explain every deferral.',
  },
  {
    role: 'LOADER',
    label: 'Loader',
    href: '/loader',
    surface: 'tablet',
    summary: 'Load in stop order and flag what is missing before departure.',
  },
  {
    role: 'DRIVER',
    label: 'Driver',
    href: '/driver',
    surface: 'phone',
    summary: 'Work the run sheet and record outcomes, with or without signal.',
  },
  {
    role: 'STORE_MANAGER',
    label: 'Store manager',
    href: '/store',
    surface: 'desktop',
    summary: 'Order before cutoff, track arrival, confirm receipt.',
  },
] as const;

const BY_ROLE = new Map(ROLE_ROUTES.map((r) => [r.role, r]));

export function routeForRole(role: Role): RoleRoute {
  const found = BY_ROLE.get(role);
  if (!found) {
    // Unreachable while Role and ROLE_ROUTES agree; throwing beats a silent
    // redirect to the wrong workspace.
    throw new Error(`No route configured for role ${role}`);
  }
  return found;
}
