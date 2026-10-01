import type { ReactNode } from 'react';

/**
 * Identifiers and times are set in Cousine with tabular figures so columns of
 * OUT001 / VEH014 / 07:34 align down a table and a dispatcher can scan them.
 */
export function Mono({
  children,
  className = '',
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <span className={`tabular text-[0.9375em] ${className}`}>{children}</span>
  );
}

/**
 * A 24-hour time. `<time>` carries the machine-readable value so screen readers
 * and copy-paste both get something sensible.
 */
export function ClockTimeText({
  value,
  className = '',
}: {
  value: string;
  className?: string;
}) {
  return (
    <time dateTime={value} className={`tabular ${className}`}>
      {value}
    </time>
  );
}

/** A signed minute delta: `+12 min` late, `−4 min` early. */
export function MinuteDelta({ minutes }: { minutes: number }) {
  if (minutes === 0) {
    return <span className="tabular text-ink-muted">on time</span>;
  }

  const late = minutes > 0;
  // U+2212 minus sign, not a hyphen — it aligns with digits.
  const text = `${late ? '+' : '−'}${Math.abs(minutes)} min`;

  return (
    <span className={`tabular ${late ? 'text-warning' : 'text-success'}`}>
      {text}
      <span className="sr-only">
        {late ? ' behind plan' : ' ahead of plan'}
      </span>
    </span>
  );
}
