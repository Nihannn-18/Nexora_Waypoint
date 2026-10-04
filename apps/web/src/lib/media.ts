import { WaypointApiError, type MediaUpload } from '@waypoint/api-client';
import { API_BASE_URL, tokenStore } from './api';

/**
 * Uploads bytes to a media slot minted by the API's create-upload endpoint and
 * returns the `fileRef` to persist on the record.
 *
 * A bare `fetch(uploadUrl)` gets two things wrong, and both matter here:
 *
 *  1. The local backend (`MEDIA_STORAGE=local`) returns a *relative* key such as
 *     `/api/v1/media/shortfall/…`. Fetched as-is from the web origin it never
 *     reaches the API, so it is resolved against `API_BASE_URL` first. S3
 *     presigned URLs are already absolute and pass through unchanged.
 *
 *  2. The local backend's PUT route is our own session-guarded API, so an inline
 *     upload carries the bearer token. A presigned S3 URL carries its own
 *     signature and must NOT receive the application token.
 *
 * This is the one upload path for both Loader shortfall photos and Driver POD
 * photos; neither role should build its own fetch.
 */
export async function uploadMediaObject(
  media: MediaUpload,
  body: Blob,
): Promise<string> {
  const url = new URL(media.uploadUrl, API_BASE_URL).toString();
  const headers: Record<string, string> = { ...media.headers };

  if (media.uploadMode === 'inline') {
    headers['Content-Type'] = body.type;
    const token = tokenStore.get();
    if (token) headers['Authorization'] = `Bearer ${token}`;
  }

  let response: Response;
  try {
    response = await fetch(url, { method: 'PUT', headers, body });
  } catch {
    throw new WaypointApiError('Could not reach the server.', {
      status: 0,
      isOffline: true,
    });
  }
  if (!response.ok) {
    throw new WaypointApiError(`Upload failed (${response.status})`, {
      status: response.status,
    });
  }

  return media.fileRef;
}

/**
 * Reads a stored media object (a driver's POD or a loader's shortfall photo)
 * and returns an object URL an `<img>` can show. The media route is
 * session-guarded, so a plain `<img src>` would arrive without the bearer
 * token; the bytes are fetched with it instead. The server decides whether the
 * caller may see the object (a store manager only for their own outlet). The
 * caller revokes the URL when the image unmounts.
 */
export async function fetchMediaObjectUrl(key: string): Promise<string> {
  const url = `${API_BASE_URL}/media/${key
    .split('/')
    .map(encodeURIComponent)
    .join('/')}`;
  const token = tokenStore.get();
  let response: Response;
  try {
    response = await fetch(url, {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    });
  } catch {
    throw new WaypointApiError('Could not reach the server.', {
      status: 0,
      isOffline: true,
    });
  }
  if (!response.ok) {
    throw new WaypointApiError(
      `Could not load the image (${response.status})`,
      {
        status: response.status,
      },
    );
  }
  return URL.createObjectURL(await response.blob());
}
