/**
 * Ends the current session on both sides.
 *
 * Order matters: the server is asked to revoke the session first, so the token
 * cannot be reused even if the local copy lingers. If that call fails — the
 * session already expired, or the network is gone — the local token is cleared
 * regardless: a token the user asked to abandon must never stay usable on this
 * device. The final redirect uses a full navigation so every cached server
 * component is left behind.
 *
 * The API client is imported lazily so a screen that merely renders a sign-out
 * control does not pull the whole client into its module graph; the call is
 * asynchronous anyway.
 *
 * `navigate` is injectable so this can be unit-tested without touching the real
 * `window.location`.
 */
export async function endSession(
  navigate: (url: string) => void = (url) => window.location.assign(url),
): Promise<void> {
  const { api, tokenStore } = await import('./api');
  try {
    await api.logout();
  } catch {
    // The session may already be invalid or unreachable; clearing locally is
    // what actually ends it on this device.
  }
  tokenStore.clear();
  navigate('/signin');
}
