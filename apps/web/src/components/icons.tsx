import type { ReactNode, SVGProps } from 'react';

/**
 * A handful of 16px stroke icons. Decorative only: every icon sits next to a
 * text label, so each is aria-hidden and carries no meaning on its own.
 */
function Icon({
  children,
  ...props
}: SVGProps<SVGSVGElement> & { children: ReactNode }) {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
      {...props}
    >
      {children}
    </svg>
  );
}

type P = SVGProps<SVGSVGElement>;

export const GridIcon = (p: P) => (
  <Icon {...p}>
    <rect x="3" y="3" width="7" height="7" rx="1" />
    <rect x="14" y="3" width="7" height="7" rx="1" />
    <rect x="3" y="14" width="7" height="7" rx="1" />
    <rect x="14" y="14" width="7" height="7" rx="1" />
  </Icon>
);

export const BoxIcon = (p: P) => (
  <Icon {...p}>
    <path d="M3 7l9-4 9 4-9 4-9-4z" />
    <path d="M3 7v10l9 4 9-4V7" />
    <path d="M12 11v10" />
  </Icon>
);

export const RouteIcon = (p: P) => (
  <Icon {...p}>
    <circle cx="6" cy="19" r="2" />
    <circle cx="18" cy="5" r="2" />
    <path d="M8 19h7a3 3 0 000-6H9a3 3 0 010-6h7" />
  </Icon>
);

export const TruckIcon = (p: P) => (
  <Icon {...p}>
    <path d="M2 6h11v10H2z" />
    <path d="M13 9h4l4 4v3h-8" />
    <circle cx="6" cy="18" r="2" />
    <circle cx="17" cy="18" r="2" />
  </Icon>
);

export const ClockIcon = (p: P) => (
  <Icon {...p}>
    <circle cx="12" cy="12" r="9" />
    <path d="M12 7v5l3 2" />
  </Icon>
);

export const BellIcon = (p: P) => (
  <Icon {...p}>
    <path d="M6 16V11a6 6 0 1112 0v5l2 2H4l2-2z" />
    <path d="M10 20a2 2 0 004 0" />
  </Icon>
);

export const ListIcon = (p: P) => (
  <Icon {...p}>
    <path d="M9 6h12M9 12h12M9 18h12" />
    <path d="M4 6h.01M4 12h.01M4 18h.01" />
  </Icon>
);

export const CalendarIcon = (p: P) => (
  <Icon {...p}>
    <rect x="3" y="5" width="18" height="16" rx="2" />
    <path d="M3 10h18M8 3v4M16 3v4" />
  </Icon>
);

export const ArrowRightIcon = (p: P) => (
  <Icon {...p}>
    <path d="M5 12h14M13 6l6 6-6 6" />
  </Icon>
);

export const DepotIcon = (p: P) => (
  <Icon {...p}>
    <path d="M3 21V9l9-6 9 6v12" />
    <path d="M9 21v-6h6v6" />
  </Icon>
);

export const UsersIcon = (p: P) => (
  <Icon {...p}>
    <circle cx="9" cy="8" r="3" />
    <path d="M3 20a6 6 0 0112 0" />
    <path d="M16 6a3 3 0 010 5.5" />
    <path d="M18 20a6 6 0 00-3-5.2" />
  </Icon>
);

export const AssignmentIcon = (p: P) => (
  <Icon {...p}>
    <path d="M9 12h6" />
    <path d="M10 8H8a4 4 0 000 8h2" />
    <path d="M14 16h2a4 4 0 000-8h-2" />
  </Icon>
);
