import { HttpClient, WaypointApiError } from './http';

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

function clientWith(fetchImpl: typeof fetch, token: string | null = 'opaque-123') {
  return new HttpClient({
    baseUrl: 'http://api.test/api/v1',
    getToken: () => token,
    fetchImpl,
  });
}

describe('HttpClient', () => {
  it('sends the bearer token and parses a JSON body', async () => {
    const fetchImpl = jest.fn().mockResolvedValue(jsonResponse({ ok: true }));
    const result = await clientWith(fetchImpl as unknown as typeof fetch).get<{
      ok: boolean;
    }>('/me');

    expect(result).toEqual({ ok: true });
    const [url, init] = fetchImpl.mock.calls[0];
    expect(url).toBe('http://api.test/api/v1/me');
    expect((init.headers as Record<string, string>)['Authorization']).toBe(
      'Bearer opaque-123',
    );
  });

  it('omits the token for an anonymous request', async () => {
    const fetchImpl = jest.fn().mockResolvedValue(jsonResponse({ token: 'x' }));
    await clientWith(fetchImpl as unknown as typeof fetch).request(
      '/auth/login',
      {
        method: 'POST',
        body: { email: 'a@b.c', password: 'x' },
        anonymous: true,
      },
    );

    const [, init] = fetchImpl.mock.calls[0];
    expect(
      (init.headers as Record<string, string>)['Authorization'],
    ).toBeUndefined();
  });

  it('builds a query string and drops undefined values', async () => {
    const fetchImpl = jest.fn().mockResolvedValue(jsonResponse([]));
    await clientWith(fetchImpl as unknown as typeof fetch).get(
      '/orders/queue',
      {
        query: { date: '2026-09-26', depotId: undefined, brand: 'FRESH' },
      },
    );

    expect(fetchImpl.mock.calls[0][0]).toBe(
      'http://api.test/api/v1/orders/queue?date=2026-09-26&brand=FRESH',
    );
  });

  it('flags a transport failure as offline so the outbox can retry it', async () => {
    const fetchImpl = jest
      .fn()
      .mockRejectedValue(new TypeError('Failed to fetch'));

    await expect(
      clientWith(fetchImpl as unknown as typeof fetch).get('/sync/status'),
    ).rejects.toMatchObject({ isOffline: true, status: 0 });
  });

  it('does NOT flag a server error as offline', async () => {
    const fetchImpl = jest
      .fn()
      .mockResolvedValue(jsonResponse({ message: 'boom' }, 500));

    const error = await clientWith(fetchImpl as unknown as typeof fetch)
      .get('/routes/live')
      .catch((e: unknown) => e);

    expect(error).toBeInstanceOf(WaypointApiError);
    expect((error as WaypointApiError).isOffline).toBe(false);
    expect((error as WaypointApiError).status).toBe(500);
  });

  it('exposes constraint results from a 422 so the rule panel can highlight them', async () => {
    const fetchImpl = jest.fn().mockResolvedValue(
      jsonResponse(
        {
          message: 'Allocation is not feasible',
          constraintResults: [
            {
              code: 'REEFER_REQUIRED',
              passed: false,
              detail: 'VEH014 is ambient',
            },
          ],
        },
        422,
      ),
    );

    const error = (await clientWith(fetchImpl as unknown as typeof fetch)
      .post('/allocations/validate', {})
      .catch((e: unknown) => e)) as WaypointApiError;

    expect(error.isConstraintViolation).toBe(true);
    expect(error.constraintResults?.[0]?.code).toBe('REEFER_REQUIRED');
  });

  it('calls onUnauthorized when the token has expired', async () => {
    const onUnauthorized = jest.fn();
    const fetchImpl = jest
      .fn()
      .mockResolvedValue(jsonResponse({ message: 'nope' }, 401));

    const client = new HttpClient({
      baseUrl: 'http://api.test/api/v1',
      getToken: () => 'stale',
      onUnauthorized,
      fetchImpl: fetchImpl as unknown as typeof fetch,
    });

    await client.get('/me').catch(() => undefined);
    expect(onUnauthorized).toHaveBeenCalledTimes(1);
  });

  it('does not call onUnauthorized for a failed anonymous login', async () => {
    const onUnauthorized = jest.fn();
    const fetchImpl = jest
      .fn()
      .mockResolvedValue(jsonResponse({ message: 'Invalid email or password' }, 401));

    const client = new HttpClient({
      baseUrl: 'http://api.test/api/v1',
      getToken: () => null,
      onUnauthorized,
      fetchImpl: fetchImpl as unknown as typeof fetch,
    });

    // The sign-in form must show the error in place; a wrong password is not an
    // expired session and must not trigger the return-to-sign-in redirect.
    await client
      .post('/auth/login', { email: 'a@b.c', password: 'wrong' }, { anonymous: true })
      .catch(() => undefined);
    expect(onUnauthorized).not.toHaveBeenCalled();
  });

  it('treats 204 as an empty success rather than a parse error', async () => {
    const fetchImpl = jest
      .fn()
      .mockResolvedValue(new Response(null, { status: 204 }));
    await expect(
      clientWith(fetchImpl as unknown as typeof fetch).post(
        '/routes/R1/dispatch',
      ),
    ).resolves.toBeUndefined();
  });

  it('survives a non-JSON error page without throwing a parse error', async () => {
    const fetchImpl = jest
      .fn()
      .mockResolvedValue(
        new Response('<html>502 Bad Gateway</html>', { status: 502 }),
      );

    const error = (await clientWith(fetchImpl as unknown as typeof fetch)
      .get('/me')
      .catch((e: unknown) => e)) as WaypointApiError;

    expect(error.status).toBe(502);
    expect(error.message).toContain('502 Bad Gateway');
  });
});
