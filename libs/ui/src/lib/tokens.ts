/**
 * Design tokens as TypeScript values.
 *
 * The CSS custom properties in `apps/web/src/app/global.css` are the source of
 * truth for anything rendered with Tailwind classes. These constants exist for
 * the places CSS variables cannot reach — canvas, chart libraries, generated
 * SVG, `<meta name="theme-color">`. Keep the two in step.
 */

export const COLORS = {
  page: '#F2F4F6',
  card: '#FFFFFF',
  ink: '#191C1E',
  inkMuted: '#505F76',
  brand: '#019EFF',
  link: '#1E40AF',
  action: '#0F172A',
  success: '#28794B',
  warning: '#D97706',
  error: '#BA1A1A',
  /** Offline is neutral, never red — see DG-B in the design. */
  offline: '#505F76',
} as const;

export const BRAND_COLORS = {
  FRESH: '#28794B',
  STYLE: '#7C3AED',
  TECH: '#0F766E',
} as const;

/** Viewport widths the design was drawn at. */
export const BREAKPOINTS = {
  phone: 402,
  tablet: 1024,
  desktop: 1440,
} as const;

export const LAYOUT = {
  sidebarWidth: 252,
  topBarHeight: 60,
  /** Minimum touch target; 56 for Driver primary actions. */
  tapTarget: 48,
  tapTargetDriver: 56,
} as const;

/** The live trip tracker polls at this interval (D-06). */
export const LIVE_REFRESH_MS = 30_000;
