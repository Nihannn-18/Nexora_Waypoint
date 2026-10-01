import { Mono } from '@waypoint/ui';

/**
 * A placeholder that names the screen it stands in for, so the route tree is
 * navigable before any screen is built and nobody has to guess which design
 * frame a route corresponds to. Delete each one as its screen lands.
 */
export function ScreenStub({
  screenId,
  title,
  purpose,
  endpoints = [],
  designNotes = [],
}: {
  screenId: string;
  title: string;
  purpose: string;
  endpoints?: readonly string[];
  designNotes?: readonly string[];
}) {
  return (
    <section className="rounded-card bg-card p-5 ring-1 ring-ink/10">
      <div className="flex items-baseline gap-2">
        <Mono className="rounded-chip bg-brand/10 px-1.5 py-0.5 text-xs text-link">
          {screenId}
        </Mono>
        <h1 className="text-lg font-semibold text-ink">{title}</h1>
      </div>

      <p className="mt-2 max-w-prose text-sm text-ink-muted">{purpose}</p>

      {endpoints.length > 0 && (
        <div className="mt-4">
          <h2 className="text-xs font-semibold uppercase tracking-wide text-ink-muted">
            Endpoints
          </h2>
          <ul className="mt-1.5 space-y-1">
            {endpoints.map((e) => (
              <li key={e}>
                <Mono className="text-xs text-ink">{e}</Mono>
              </li>
            ))}
          </ul>
        </div>
      )}

      {designNotes.length > 0 && (
        <div className="mt-4">
          <h2 className="text-xs font-semibold uppercase tracking-wide text-ink-muted">
            Must not be lost in implementation
          </h2>
          <ul className="mt-1.5 list-disc space-y-1 pl-5 text-sm text-ink-muted">
            {designNotes.map((n) => (
              <li key={n}>{n}</li>
            ))}
          </ul>
        </div>
      )}
    </section>
  );
}
