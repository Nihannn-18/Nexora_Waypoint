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
