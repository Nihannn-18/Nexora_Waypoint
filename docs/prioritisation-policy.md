# Prioritisation policy

The brief is explicit that there is no single optimal allocation, and that the team must
**document** its prioritisation policy rather than silently encode a preference. This is that
document. It also serves as the basis for the roughly one-page written policy required by
Datathon Task 2B.

The policy is encoded in `PRIORITISATION_POLICY`
(`libs/shared-types/src/lib/constraints.ts`) so that code and prose cannot drift apart.

---

## Feasibility is not a preference

Before any ranking, every hard constraint must pass: capacity, refrigeration, van-only
access, home depot, trip count, time budgets, delivery and mall windows, weekly fuel, and no
split or duplicate assignment. **No order is ever served by relaxing a rule.** Ranking only
decides which of several feasible options is taken, and which orders are deferred when
nothing feasible remains.

---

## The ordering

Applied in sequence. Each rule only breaks ties left by the one above it.

### 1. Never defer an outlet that was deferred yesterday

An outlet skipped two days running has effectively been dropped from the network, and the
store manager has no basis for trusting any future promise. A single deferral is an
inconvenience; a repeated one is a broken relationship. The Task 2B dataset supplies
`deferred_yesterday` precisely so this is possible to honour.

The practical consequence: on a constrained day this rule can force a less efficient plan,
because yesterday's deferrals are placed before today's larger or better-fitting orders. That
cost is accepted deliberately.

### 2. Protect outlets unserved for seven or more days

A slower-moving version of the same failure. `days_since_last_served` catches outlets that
were never formally deferred but have simply kept losing to better-fitting neighbours — the
starvation that a purely efficiency-driven ranking produces and never reports.

### 3. Chilled and frozen before ambient

Only 12 reefer trucks and 4 refrigerated vans serve the whole network, so refrigerated
capacity is the binding resource on most days. An ambient order placed on a reefer consumes
capacity that nothing else can replace, whereas a chilled order has no alternative vehicle.

There is also an asymmetry in consequence: deferred chilled goods are a spoilage risk, while
deferred ambient goods are a delay.

### 4. Narrow delivery windows before wide ones

A mall outlet with a fixed two-hour access window is harder to place tomorrow than a street
outlet open all morning. Serving the constrained outlet today and deferring the flexible one
leaves tomorrow's planner more room, so the same total demand fits across two days.

### 5. Among equals, the largest order that still fits

Only once fairness and scarcity are settled. This is the efficiency term, and it is last
deliberately: putting it first produces a plan that looks excellent on utilisation and
quietly starves small outlets.

### 6. Style and Tech move before Fresh

Fresh must arrive before stores open at 08:00, and groceries are perishable. Style and Tech
have flexible or weekly schedules, so a one-day shift costs far less. When something has to
move, it should be the delivery that tolerates moving.

---

## What the dispatcher sees

The policy informs the _suggestion_; it does not make the decision. For every deferral the
board shows:

- the **binding constraint** — which rule made it infeasible, by its `E-0x` identifier
- the **consequence** — days since the outlet was last served, and whether it was deferred
  yesterday
- the **alternative**, where one exists — what would have to be deferred instead to serve it

The dispatcher can override any suggestion. An override is recorded as `reasonType: POLICY`
with free text, so a human judgement call is visibly distinct from a constraint-driven one.
Those are different kinds of fact and the deferral log should not blur them.

---

## Recording a deferral

`POST /allocations/confirm` is rejected if any confirmed order is neither allocated nor
deferred. Every deferral carries:

| Field                    | Meaning                                              |
| ------------------------ | ---------------------------------------------------- |
| `reasonType`             | `CONSTRAINT` — a hard rule left no feasible slot     |
|                          | `POLICY` — the dispatcher prioritised another outlet |
|                          | `OPERATIONAL` — breakdown, shortfall, depot problem  |
| `constraintCode`         | The blocking rule, when `reasonType` is `CONSTRAINT` |
| `reasonText`             | What the store manager reads in S-05                 |
| `decidedBy`, `decidedAt` | Attribution and timestamp                            |
| `deferredToDate`         | The run it moves to, where known                     |

---

## For Task 2B

The same ordering applies to the Scenario S1 peak-day allocation. The written submission
should set out, with figures:

1. **What limited service** — which resource ran out first. Reefer capacity, Fresh minutes
   and available vehicles are the usual candidates; name the one that actually bound, with
   the arithmetic.
2. **Which deferrals were unavoidable** — no feasible vehicle and trip existed under any
   ordering. Show the constraint.
3. **Which were a choice** — a feasible slot existed but was given to another order under
   rules 1–6. Name the rule and the order that took the slot.
4. **What each choice cost** — volume not served, and which outlets absorbed the deferral.

Run `check_allocation.py` before submitting. A policy that reads well but fails feasibility
validation scores nothing.
