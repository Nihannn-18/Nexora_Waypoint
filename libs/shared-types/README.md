# @waypoint/shared-types

The contract between the Go API and the Next.js client.

Types plus pure functions — no React, no Next, no runtime dependency — so the planning
arithmetic can be unit-tested on its own and reused anywhere.

| File             | Holds                                                                              |
| ---------------- | ---------------------------------------------------------------------------------- |
| `domain.ts`      | Roles, brands, depots, vehicles, order statuses, the legal transition map          |
| `constraints.ts` | Constraint codes, the E-0x rule catalogue, time budgets, the prioritisation policy |
| `entities.ts`    | Read models as the API returns them                                                |
| `requests.ts`    | Request bodies, as the Go handlers expect them                                     |
| `trip-time.ts`   | Trip time, daily budgets, delivery windows, fuel                                   |

## The pairing rule

Everything in `domain.ts` and `constraints.ts` has a counterpart in
`apps/api/internal/domain`. **Change both in one commit.** A rename on one side alone compiles
cleanly and fails at runtime, which is the worst kind of bug to find on demo day.

## Which side is authoritative

The Go implementation. `trip-time.ts` is a mirror, present so the dispatcher's board can
update its capacity and minute meters as orders are dragged between lanes without a
round-trip. The server revalidates every plan regardless of what the client computed.

The same test vectors are pinned on both sides — including the booklet's worked example,
37 + 9×2 + 15 + 15 + 16 = 101 minutes. If the two implementations ever disagree, a test fails
rather than a plan quietly going wrong.

```bash
npx nx test shared-types
```
