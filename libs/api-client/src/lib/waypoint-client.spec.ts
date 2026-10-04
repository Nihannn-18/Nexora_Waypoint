import { WaypointClient } from './waypoint-client';

describe('WaypointClient.meta', () => {
  it('reads GET /meta without a token', async () => {
    const body = {
      now: '2026-09-25T15:40:00+05:30',
      demoMode: true,
      timezone: 'Asia/Colombo',
    };
    const fetchImpl = jest.fn().mockResolvedValue(
      new Response(JSON.stringify(body), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    );
    const client = new WaypointClient({
      baseUrl: 'http://api.test/api/v1',
      getToken: () => 'token',
      fetchImpl: fetchImpl as unknown as typeof fetch,
    });

    await expect(client.meta()).resolves.toEqual(body);

    const [url, init] = fetchImpl.mock.calls[0];
    expect(url).toBe('http://api.test/api/v1/meta');
    expect(
      (init.headers as Record<string, string>)['Authorization'],
    ).toBeUndefined();
  });
});

function clientReturning(body: unknown, status = 200) {
  const fetchImpl = jest.fn().mockResolvedValue(
    new Response(JSON.stringify(body), {
      status,
      headers: { 'Content-Type': 'application/json' },
    }),
  );
  const client = new WaypointClient({
    baseUrl: 'http://api.test/api/v1',
    getToken: () => 'token',
    fetchImpl: fetchImpl as unknown as typeof fetch,
  });
  return { client, fetchImpl };
}

describe('WaypointClient dispatcher reads', () => {
  it('sends every order filter to the server and returns the page with its total', async () => {
    const page = { orders: [], total: 186, limit: 25, offset: 50 };
    const { client, fetchImpl } = clientReturning(page);
    await expect(
      client.listOrders({
        deliveryDate: '2026-09-26',
        depotId: 'd1',
        status: 'CONFIRMED',
        brand: 'FRESH',
        search: 'OUT014',
        limit: 25,
        offset: 50,
      }),
    ).resolves.toEqual(page);
    const url = new URL(fetchImpl.mock.calls[0][0]);
    expect(url.pathname).toBe('/api/v1/orders');
    expect(Object.fromEntries(url.searchParams)).toEqual({
      deliveryDate: '2026-09-26',
      depotId: 'd1',
      status: 'CONFIRMED',
      brand: 'FRESH',
      search: 'OUT014',
      limit: '25',
      offset: '50',
    });
  });

  it('unwraps list envelopes', async () => {
    const routes = [{ routeId: 'R1' }];
    await expect(
      clientReturning({ routes }).client.listRoutes({ date: '2026-09-26' }),
    ).resolves.toEqual(routes);
    await expect(
      clientReturning({ deferrals: [] }).client.getDeferrals(),
    ).resolves.toEqual([]);
    await expect(
      clientReturning({ unread: 3 }).client.getUnreadNotificationCount(),
    ).resolves.toBe(3);
  });

  it('marks all notifications read and reports how many changed', async () => {
    const { client, fetchImpl } = clientReturning({ markedRead: 2 });
    await expect(client.markAllNotificationsRead()).resolves.toBe(2);
    expect(fetchImpl.mock.calls[0][1].method).toBe('POST');
    expect(fetchImpl.mock.calls[0][0]).toBe(
      'http://api.test/api/v1/notifications/read-all',
    );
  });

  it('surfaces a confirm conflict as a 409 error, not a success', async () => {
    const { client } = clientReturning(
      {
        message: 'The plan conflicts with current state; reload and retry',
        code: 'CONFLICT',
      },
      409,
    );
    await expect(
      client.confirmAllocation({ jobId: 'J', routes: [], deferrals: [] }),
    ).rejects.toMatchObject({ status: 409, code: 'CONFLICT' });
  });

  it('closes the queue per brand and returns the frozen queue', async () => {
    const body = {
      date: '2026-09-26',
      depotId: 'd1',
      brands: ['FRESH'],
      closed: 1,
      alreadyClosed: [],
      queue: { orders: [], total: 0, limit: 200, offset: 0 },
    };
    const { client, fetchImpl } = clientReturning(body);
    await expect(
      client.closeQueue({
        date: '2026-09-26',
        depotId: 'd1',
        brands: ['FRESH'],
      }),
    ).resolves.toEqual(body);
    expect(fetchImpl.mock.calls[0][1].method).toBe('POST');
    expect(fetchImpl.mock.calls[0][0]).toBe(
      'http://api.test/api/v1/orders/close',
    );
    expect(JSON.parse(fetchImpl.mock.calls[0][1].body)).toEqual({
      date: '2026-09-26',
      depotId: 'd1',
      brands: ['FRESH'],
    });
  });

  it('reads the frozen queue for a date and depot', async () => {
    const page = { orders: [], total: 85, limit: 200, offset: 0 };
    const { client, fetchImpl } = clientReturning(page);
    await expect(
      client.getOrderQueue({ date: '2026-09-26', depotId: 'd1' }),
    ).resolves.toEqual(page);
    const url = new URL(fetchImpl.mock.calls[0][0]);
    expect(url.pathname).toBe('/api/v1/orders/queue');
    expect(Object.fromEntries(url.searchParams)).toEqual({
      date: '2026-09-26',
      depotId: 'd1',
    });
  });
});

describe('WaypointClient demo controls', () => {
  it('posts the stage to /demo/clock with the session token', async () => {
    const body = {
      stage: 'LOADING',
      now: '2026-09-26T03:30:00+05:30',
      demoMode: true,
      timezone: 'Asia/Colombo',
    };
    const { client, fetchImpl } = clientReturning(body);
    await expect(client.setDemoClock({ stage: 'LOADING' })).resolves.toEqual(
      body,
    );
    const [url, init] = fetchImpl.mock.calls[0];
    expect(url).toBe('http://api.test/api/v1/demo/clock');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body as string)).toEqual({ stage: 'LOADING' });
    expect((init.headers as Record<string, string>)['Authorization']).toBe(
      'Bearer token',
    );
  });

  it('posts /demo/reset and returns the reset summary', async () => {
    const body = {
      now: '2026-09-25T15:40:00+05:30',
      demoMode: true,
      timezone: 'Asia/Colombo',
      cleared: { customer_order: 3 },
      demoOrders: 85,
      demoVehicleDays: 60,
    };
    const { client, fetchImpl } = clientReturning(body);
    await expect(client.resetDemo()).resolves.toEqual(body);
    const [url, init] = fetchImpl.mock.calls[0];
    expect(url).toBe('http://api.test/api/v1/demo/reset');
    expect(init.method).toBe('POST');
  });
});
