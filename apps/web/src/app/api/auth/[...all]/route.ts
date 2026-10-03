import { toNextJsHandler } from 'better-auth/next-js';
import { auth } from '../../../../lib/auth';

/**
 * Better Auth's Next.js route handler: all `/api/auth/*` requests (sign-in,
 * sign-out, session, etc.) are served here. The Go API never issues or owns
 * credentials; it only verifies the session this handler creates.
 *
 * Node.js runtime is required: Better Auth uses `pg` and WebCrypto, neither of
 * which is available in the Edge runtime.
 */
export const runtime = 'nodejs';

export const { GET, POST } = toNextJsHandler(auth);
