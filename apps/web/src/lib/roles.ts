import type { Role } from '@waypoint/shared-types';

/**
 * Where each role lands after signing in, and the device the design assumes.
 *
 * The seeded demo accounts are the ones printed on the sign-in card (G-01) so a
 * judge never has to hunt for credentials. Passwords live in the seed data and
 * the README — not here, because this file ships to the browser.
 */
export interface RoleRoute {
  readonly role: Role;
  readonly label: string;
  /** Who does this job at Waypoint, from the Designathon personas. */
  readonly persona: string;
  readonly href: string;
  readonly email: string;
  /** The viewport the screens for this role were designed at. */
  readonly surface: 'desktop' | 'tablet' | 'phone';
  readonly summary: string;
}

export const ROLE_ROUTES: readonly RoleRoute[] = [
  {
    role: 'DISPATCHER',
    label: 'Dispatcher',
    persona: 'Priyantha W.',
    href: '/dispatcher',
    email: 'priyantha.w@waypoint.lk',
    surface: 'desktop',
    summary: 'Close the queue, build the plan, explain every deferral.',
  },
  {
    role: 'LOADER',
    label: 'Loader',
    persona: 'Nadeesha P.',
    href: '/loader',
    email: 'nadeesha.p@waypoint.lk',
    surface: 'tablet',
    summary: 'Load in stop order and flag what is missing before departure.',
  },
  {
    role: 'DRIVER',
    label: 'Driver',
    persona: 'Kasun P.',
    href: '/driver',
    email: 'kasun.p@waypoint.lk',
    surface: 'phone',
    summary: 'Work the run sheet and record outcomes, with or without signal.',
  },
  {
    role: 'STORE_MANAGER',
    label: 'Store manager',
    persona: 'Ishara S.',
    href: '/store',
    email: 'ishara.s@waypoint.lk',
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
