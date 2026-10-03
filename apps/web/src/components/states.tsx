import type { ReactNode } from 'react';
import { WaypointApiError } from '@waypoint/api-client';

/**
 * Loading, empty and error states shared by the role workspaces. Every list
 * has an empty state that explains itself, and every failure says what
 * happened and what to do next (style guide microcopy rule).
 */

export function LoadingState({ label = 'Loading…' }: { label?: string }) {
  return (
    <div
      role="status"
      aria-live="polite"
      className="flex items-center gap-3 rounded-card bg-card p-5 text-sm text-ink-muted ring-1 ring-ink/10"
    >
      <span
        aria-hidden="true"
        className="size-4 shrink-0 animate-spin rounded-pill border-2 border-ink/15 border-t-brand"
      />
      {label}
    </div>
  );
}

export function EmptyState({
  title,
  children,
  action,
}: {
  title: string;
  children?: ReactNode;
  action?: ReactNode;
}) {
  return (
    <div className="rounded-card border border-dashed border-ink/20 bg-card p-6 text-center">
      <p className="font-medium text-ink">{title}</p>
      {children && (
        <div className="mx-auto mt-1 max-w-prose text-sm text-ink-muted">
          {children}
        </div>
      )}
      {action && <div className="mt-4 flex justify-center">{action}</div>}
    </div>
  );
}

export interface ErrorDescription {
  readonly title: string;
  readonly detail: string;
  /** Worth offering a retry: the same request could succeed later. */
  readonly retryable: boolean;
}

/**
 * Turns any thrown value into operator-facing copy. Only the server's own
 * message is echoed; no stack or raw body is shown.
 */
export function describeApiError(error: unknown): ErrorDescription {
  if (!(error instanceof WaypointApiError)) {
    return {
      title: 'Something went wrong in this page',
      detail: 'Reload the page. If it happens again, tell the platform team.',
      retryable: true,
    };
  }
  if (error.isOffline) {
    return {
      title: 'Can’t reach the Waypoint API',
      detail:
        'The request never reached the server, so nothing was changed. Check the connection and try again.',
      retryable: true,
    };
  }
  switch (error.status) {
    case 401:
      return {
        title: 'Your session has ended',
        detail: 'Sign in again to continue.',
        retryable: false,
      };
    case 403:
      return {
        title: 'This needs a dispatcher account',
        detail:
          'The API refused the request for your role. Sign in with the dispatcher account to plan and confirm deliveries.',
        retryable: false,
      };
    case 404:
      return {
        title: 'Not found',
        detail: error.message,
        retryable: false,
      };
    case 409:
      return {
        title: 'Someone changed this first',
        detail: `${error.message}. Reload to see the current state before trying again.`,
        retryable: true,
      };
    case 400:
    case 422:
      return {
        title: 'The API rejected this request',
        detail:
          error.fieldErrors && error.fieldErrors.length > 0
            ? error.fieldErrors
                .map((f) => `${f.field}: ${f.message}`)
                .join(' · ')
            : error.message,
        retryable: false,
      };
    default:
      if (/authentication is not configured/i.test(error.message)) {
        return {
          title: 'Sign-in isn’t connected to the API yet',
          detail:
            'The API cannot verify your session, so it refuses every request rather than guess who you are. This clears once the sign-in bridge is deployed.',
          retryable: true,
        };
      }
      return {
        title: 'The API hit a problem',
        detail: `${error.message} (HTTP ${error.status}). Try again in a moment.`,
        retryable: true,
      };
  }
}

export function ErrorState({
  error,
  onRetry,
  compact = false,
}: {
  error: unknown;
  onRetry?: () => void;
  compact?: boolean;
}) {
  const { title, detail, retryable } = describeApiError(error);
  return (
    <div
      role="alert"
      className={`flex flex-wrap items-start gap-3 rounded-card bg-error/5 ring-1 ring-error/25 ${compact ? 'p-3' : 'p-5'}`}
    >
      <span
        aria-hidden="true"
        className="mt-0.5 grid size-6 shrink-0 place-items-center rounded-chip bg-error/10 text-sm font-bold text-error"
      >
        !
      </span>
      <div className="min-w-0 flex-1">
        <p className="font-semibold text-ink">{title}</p>
        <p className="mt-0.5 text-sm text-ink-muted">{detail}</p>
      </div>
      {onRetry && retryable && (
        <button
          type="button"
          onClick={onRetry}
          className="tap-target rounded-control bg-card px-4 text-sm font-medium text-ink ring-1 ring-ink/15 hover:bg-page"
        >
          Try again
        </button>
      )}
    </div>
  );
}
