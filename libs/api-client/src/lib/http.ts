/**
 * The HTTP layer every role screen shares.
 *
 * Two things it deliberately does that a bare `fetch` does not:
 *
 *  1. It distinguishes *offline* from *failed*. The Driver runs on patchy hill-
 *     country coverage, so a request that never reached the server must be
 *     queued and retried, while a 422 from the server must be shown to the user.
 *     `WaypointApiError.isOffline` is what the outbox checks.
 *
 *  2. It surfaces constraint violations as structured data rather than a string,
 *     so the dispatcher's rule panel can highlight the specific blocking rule.
 */

import type { ConstraintResult } from '@waypoint/shared-types';

export interface FieldError {
  readonly field: string;
  readonly message: string;
}

export interface ApiErrorBody {
  readonly message?: string;
  readonly code?: string;
  /** Present on 400 VALIDATION_FAILED: one entry per invalid field. */
  readonly fieldErrors?: readonly FieldError[];
  /** Present on 422 from the allocation endpoints. */
  readonly constraintResults?: readonly ConstraintResult[];
}

export class WaypointApiError extends Error {
  readonly status: number;
  readonly code?: string;
  readonly fieldErrors?: readonly FieldError[];
  readonly constraintResults?: readonly ConstraintResult[];
  /**
   * True when the request never reached the server — DNS failure, dropped
   * connection, navigator offline. Callers retry these; they do not retry 4xx.
   */
  readonly isOffline: boolean;

  constructor(
    message: string,
    options: {
      status: number;
      code?: string;
      fieldErrors?: readonly FieldError[];
      constraintResults?: readonly ConstraintResult[];
      isOffline?: boolean;
    },
  ) {
    super(message);
    this.name = 'WaypointApiError';
    this.status = options.status;
    this.code = options.code;
    this.fieldErrors = options.fieldErrors;
    this.constraintResults = options.constraintResults;
    this.isOffline = options.isOffline ?? false;
  }

  /** A violation the user can act on, as opposed to a server fault. */
  get isConstraintViolation(): boolean {
    return this.status === 422 && (this.constraintResults?.length ?? 0) > 0;
  }
}

export interface HttpClientOptions {
  /** Base URL including the version prefix, e.g. `http://localhost:8080/api/v1`. */
  readonly baseUrl: string;
  /** Called before each request; return null when not signed in. */
  readonly getToken?: () => string | null;
  /** Invoked on 401 so the UI can clear the session and return to G-01. */
  readonly onUnauthorized?: () => void;
  readonly fetchImpl?: typeof fetch;
  readonly timeoutMs?: number;
}

export interface RequestOptions {
  readonly method?: 'GET' | 'POST' | 'PATCH' | 'PUT' | 'DELETE';
  readonly body?: unknown;
  readonly query?: Record<string, string | number | boolean | undefined>;
  readonly signal?: AbortSignal;
  /** Skip the Authorization header — used only by POST /auth/login. */
  readonly anonymous?: boolean;
}

const DEFAULT_TIMEOUT_MS = 15_000;

export class HttpClient {
  private readonly baseUrl: string;
  private readonly getToken: () => string | null;
  private readonly onUnauthorized?: () => void;
  private readonly fetchImpl: typeof fetch;
  private readonly timeoutMs: number;

  constructor(options: HttpClientOptions) {
    this.baseUrl = options.baseUrl.replace(/\/+$/, '');
    this.getToken = options.getToken ?? (() => null);
    this.onUnauthorized = options.onUnauthorized;
    this.fetchImpl = options.fetchImpl ?? globalThis.fetch.bind(globalThis);
    this.timeoutMs = options.timeoutMs ?? DEFAULT_TIMEOUT_MS;
  }

  async request<T>(path: string, options: RequestOptions = {}): Promise<T> {
    const url = this.buildUrl(path, options.query);
    const headers: Record<string, string> = { Accept: 'application/json' };

    if (options.body !== undefined) {
      headers['Content-Type'] = 'application/json';
    }

    if (!options.anonymous) {
      const token = this.getToken();
      if (token) {
        headers['Authorization'] = `Bearer ${token}`;
      }
    }

    // Our own timeout, combined with any caller-supplied signal.
    const timeout = AbortSignal.timeout(this.timeoutMs);
    const signal = options.signal
      ? AbortSignal.any([options.signal, timeout])
      : timeout;

    let response: Response;
    try {
      response = await this.fetchImpl(url, {
        method: options.method ?? 'GET',
        headers,
        body:
          options.body === undefined ? undefined : JSON.stringify(options.body),
        signal,
      });
    } catch (cause) {
      // Never reached the server. The caller decides whether to queue it.
      throw new WaypointApiError(
        cause instanceof Error && cause.name === 'TimeoutError'
          ? 'The request timed out before the server replied.'
          : 'Could not reach the server.',
        { status: 0, isOffline: true },
      );
    }

    // A 401 on an authenticated call means the session is gone, so the UI
    // returns to sign-in. An anonymous call (POST /auth/login) must NOT trigger
    // that: a wrong password is a normal failure the form must show in place,
    // not a redirect that wipes the message before it renders.
    if (response.status === 401 && !options.anonymous) {
      this.onUnauthorized?.();
    }

    if (response.status === 204) {
      return undefined as T;
    }

    const payload = await this.readBody(response);

    if (!response.ok) {
      const body = (payload ?? {}) as ApiErrorBody;
      throw new WaypointApiError(
        body.message ?? `Request failed (${response.status})`,
        {
          status: response.status,
          code: body.code,
          fieldErrors: body.fieldErrors,
          constraintResults: body.constraintResults,
        },
      );
    }

    return payload as T;
  }

  get<T>(
    path: string,
    options?: Omit<RequestOptions, 'method' | 'body'>,
  ): Promise<T> {
    return this.request<T>(path, { ...options, method: 'GET' });
  }

  post<T>(
    path: string,
    body?: unknown,
    options?: Omit<RequestOptions, 'method'>,
  ): Promise<T> {
    return this.request<T>(path, { ...options, method: 'POST', body });
  }

  patch<T>(
    path: string,
    body?: unknown,
    options?: Omit<RequestOptions, 'method'>,
  ): Promise<T> {
    return this.request<T>(path, { ...options, method: 'PATCH', body });
  }

  put<T>(
    path: string,
    body?: unknown,
    options?: Omit<RequestOptions, 'method'>,
  ): Promise<T> {
    return this.request<T>(path, { ...options, method: 'PUT', body });
  }

  delete<T>(
    path: string,
    options?: Omit<RequestOptions, 'method' | 'body'>,
  ): Promise<T> {
    return this.request<T>(path, { ...options, method: 'DELETE' });
  }

  private buildUrl(
    path: string,
    query?: Record<string, string | number | boolean | undefined>,
  ): string {
    const normalised = path.startsWith('/') ? path : `/${path}`;
    const search = new URLSearchParams();

    for (const [key, value] of Object.entries(query ?? {})) {
      if (value !== undefined) {
        search.set(key, String(value));
      }
    }

    const qs = search.toString();
    return `${this.baseUrl}${normalised}${qs ? `?${qs}` : ''}`;
  }

  private async readBody(response: Response): Promise<unknown> {
    const text = await response.text();
    if (!text) return undefined;
    try {
      return JSON.parse(text);
    } catch {
      // A proxy error page or a Go panic trace, not JSON. Keep it for the log.
      return { message: text.slice(0, 500) } satisfies ApiErrorBody;
    }
  }
}
