import { WaypointApiError, type MediaUpload } from '@waypoint/api-client';
import { fetchMediaObjectUrl, uploadMediaObject } from './media';
import { tokenStore } from './api';

jest.mock('./api', () => ({
  API_BASE_URL: 'http://localhost:8080/api/v1',
  tokenStore: { get: jest.fn() },
}));

const getToken = tokenStore.get as jest.Mock;

const inlineSlot: MediaUpload = {
  fileRef: 'shortfall/OI1/abc',
  uploadMode: 'inline',
  uploadUrl: '/api/v1/media/shortfall/OI1/abc',
};

const presignedSlot: MediaUpload = {
  fileRef: 'pod/LEG1/xyz',
  uploadMode: 'presigned',
  uploadUrl:
    'https://bucket.s3.amazonaws.com/pod/LEG1/xyz?X-Amz-Signature=deadbeef',
  headers: { 'Content-Type': 'image/jpeg' },
};

const body = () => new Blob(['bytes'], { type: 'image/jpeg' });

const fetchMock = jest.fn();

beforeEach(() => {
  jest.clearAllMocks();
  global.fetch = fetchMock as unknown as typeof fetch;
});

describe('uploadMediaObject', () => {
  it('resolves the relative local key against the API base and sends the bearer token', async () => {
    getToken.mockReturnValue('session-token');
    fetchMock.mockResolvedValue({ ok: true, status: 201 });

    const ref = await uploadMediaObject(inlineSlot, body());

    expect(ref).toBe('shortfall/OI1/abc');
    expect(fetchMock).toHaveBeenCalledWith(
      'http://localhost:8080/api/v1/media/shortfall/OI1/abc',
      expect.objectContaining({
        method: 'PUT',
        headers: expect.objectContaining({
          'Content-Type': 'image/jpeg',
          Authorization: 'Bearer session-token',
        }),
      }),
    );
  });

  it('never sends the application token to a presigned S3 URL', async () => {
    getToken.mockReturnValue('session-token');
    fetchMock.mockResolvedValue({ ok: true, status: 200 });

    await uploadMediaObject(presignedSlot, body());

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(fetchMock.mock.calls[0][0]).toBe(presignedSlot.uploadUrl);
    expect(init.headers).not.toHaveProperty('Authorization');
  });

  it('still uploads when no token is stored rather than sending "Bearer null"', async () => {
    getToken.mockReturnValue(null);
    fetchMock.mockResolvedValue({ ok: true, status: 201 });

    await uploadMediaObject(inlineSlot, body());

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(init.headers).not.toHaveProperty('Authorization');
  });

  it('surfaces a rejected upload as a WaypointApiError with the status', async () => {
    getToken.mockReturnValue('session-token');
    fetchMock.mockResolvedValue({ ok: false, status: 401 });

    await expect(uploadMediaObject(inlineSlot, body())).rejects.toMatchObject({
      status: 401,
    });
  });

  it('marks a network failure as offline so the outbox can retry it', async () => {
    getToken.mockReturnValue('session-token');
    fetchMock.mockRejectedValue(new TypeError('fetch failed'));

    const err = await uploadMediaObject(inlineSlot, body()).catch((e) => e);
    expect(err).toBeInstanceOf(WaypointApiError);
    expect(err.isOffline).toBe(true);
  });
});

describe('fetchMediaObjectUrl', () => {
  it('reads the object with the bearer token and returns an object URL', async () => {
    getToken.mockReturnValue('session-token');
    const blob = new Blob(['img'], { type: 'image/jpeg' });
    fetchMock.mockResolvedValue({ ok: true, status: 200, blob: () => blob });
    const createObjectURL = jest.fn().mockReturnValue('blob:pod');
    URL.createObjectURL = createObjectURL;

    await expect(fetchMediaObjectUrl('pod/LEG1/xyz')).resolves.toBe('blob:pod');
    expect(fetchMock).toHaveBeenCalledWith(
      'http://localhost:8080/api/v1/media/pod/LEG1/xyz',
      { headers: { Authorization: 'Bearer session-token' } },
    );
    expect(createObjectURL).toHaveBeenCalledWith(blob);
  });

  it('reports a refused read as an API error, not an image', async () => {
    getToken.mockReturnValue('session-token');
    fetchMock.mockResolvedValue({ ok: false, status: 403 });

    await expect(fetchMediaObjectUrl('pod/LEG1/xyz')).rejects.toMatchObject({
      status: 403,
    });
  });
});
