# @waypoint/api-client

Typed access to the Go REST API. Plain `fetch`, framework-agnostic on purpose, so the same
client works in a server component, a client component and the Driver's offline outbox.

```ts
import { api } from '../lib/api';

const queue = await api.getOrderQueue('2026-09-26', 'DEP-PELIYAGODA');
```

## The two things it does that bare `fetch` does not

**It distinguishes offline from failed.** `WaypointApiError.isOffline` is true only when the
request never reached the server — dropped connection, DNS failure, timeout. The Driver's
outbox retries those and never retries a 4xx, because a rejected request will be rejected
again.

**It surfaces constraint violations as data.** A 422 carries `constraintResults`, so the
dispatcher's rule panel can highlight the specific rule that blocked a move instead of showing
a message. `isConstraintViolation` is the check.

One method per endpoint in [`docs/api.md`](../../docs/api.md). Adding an endpoint means three
edits in one commit: the Go handler, the client method, and the row in that document.

```bash
npx nx test api-client
```
