# Working in this repository

Three days, four people, one deadline. The conventions below exist to keep us out of each
other's way rather than to be thorough.

---

## First-time setup

```bash
git clone <repo-url> Nexora_Waypoint
cd Nexora_Waypoint
npm ci                       # installs the whole workspace, frontend and tooling
cp .env.example .env
```

You need **Node 22** and **Go 1.24**. Check with `node -v` and `go version`. For a database
and queue without running the whole stack:

```bash
docker compose up postgres rabbitmq
```

Confirm the workspace is healthy before you change anything:

```bash
npx nx run-many -t lint test
```

That runs ESLint and Jest _and_ `go vet` and `go test`. If it is green, the problem is
yours; if it is red on a fresh clone, say so in the group rather than working around it.

---

## Backend handoff — start here if you own the Go API

The scaffold deliberately stops before the real work so that nothing has to be undone.

### What is already there

```
apps/api/
├── go.mod                       module waypoint.lk/api — no dependencies yet
├── Dockerfile                   multi-stage, static binary, non-root
├── project.json                 the Nx targets
├── cmd/api/main.go              config, logging, routing, graceful shutdown
└── internal/
    ├── config/config.go         env settings, validated at start-up
    ├── domain/constraints.go    constraint codes, roles, brands, statuses
    ├── httpx/json.go            JSON helpers + the single error shape
    ├── httpx/router.go          ServeMux with /healthz, /readyz, /api/v1/meta
    └── planning/triptime.go     trip time, budgets, windows, fuel — tested
```

Verify it for yourself:

```bash
npx nx serve api
curl localhost:8080/healthz
curl localhost:8080/api/v1/meta
npx nx test api
```

### Why the module path is `waypoint.lk/api`

Not a GitHub URL, because this service is never imported from outside the monorepo and a
domain-style path never has to change if the repository moves or is renamed. Imports look
like `waypoint.lk/api/internal/planning`. Leave it alone.

### Adding dependencies

There are none yet, on purpose: the scaffold builds on a fresh clone with no network. Add
what you need and commit both files:

```bash
cd apps/api
go get github.com/jackc/pgx/v5
go get github.com/rabbitmq/amqp091-go
go get github.com/golang-jwt/jwt/v5
go mod tidy
```

`go.sum` appears on the first dependency. The Dockerfile already copies it when present.

### The two things that must not be compromised

**One authority on feasibility.** Every hard rule — capacity, refrigeration, van-only
access, home depot, trip count, time budgets, windows, fuel, duplicate assignment — lives in
one constraint validator and nowhere else. The dispatcher's UI greys out impossible moves,
but that is a convenience; the server revalidates the entire plan inside the confirmation
transaction regardless. If a rule gets implemented twice, the two copies will disagree under
deadline pressure and the plan will be wrong in a way no test catches.

**Suggestions are not allocations.** A planning run writes to its own tables. Nothing
becomes authoritative until the dispatcher confirms and the server has revalidated against
current state. Never write a final allocation during a suggestion.

### Suggested build order

Each step leaves the repository working, so nobody is blocked waiting for you.

1. Migrations and the schema — see [`docs/data-model.md`](docs/data-model.md)
2. Seed the reference CSVs: outlets, vehicles, items, calendar, district travel, service
   allowances
3. JWT, role-based access control, and the four seeded accounts
4. Items, order lines, order creation, generated order numbers
5. The 16:00 cutoff and the order queue
6. **The constraint validator, with a test per rule** — the list is in the specification's
   mandatory test suite, and the boundary cases matter: exactly at capacity accepts, one
   unit over rejects; 270 minutes accepts, 271 rejects; 480 accepts, 481 rejects
7. Routes and legs, using the already-tested `planning` package
8. Planning jobs, the RabbitMQ worker, and suggestion results
9. Allocation suggest → validate → recalculate → confirm
10. Deferrals with reasons, explanations and notifications
11. Loader shortfalls at the order-line level
12. Driver events, idempotent by `client_event_id`, and batch offline sync
13. Receipts and issues
14. Live route state
15. The demand forecast endpoint

### Conventions

- Handlers stay thin. No planning arithmetic in a handler — it belongs in `internal/planning`
  or the validator, where it can be tested without a server.
- Persistence structs never cross the HTTP boundary. Define a response type.
- Return errors; do not log and swallow. Log once, at the boundary.
- Wrap with context: `fmt.Errorf("confirming allocation: %w", err)`.
- Table-driven tests. The existing `planning` tests are the pattern to copy.
- `npx nx fmt api` before committing. CI fails on unformatted code.

---

## Frontend conventions

- **Never hard-code a colour.** The Day 5 tokens are CSS custom properties in
  `apps/web/src/app/global.css`; use `bg-page`, `text-ink-muted`, `text-success` and so on.
  A hex literal in a component is a bug.
- **Status is never signalled by colour alone** — pair it with a glyph or a word. Use
  `StatusBadge` from `@waypoint/ui`, which does this already.
- **Offline is neutral, never red.** Losing signal on a hill-country route is expected, not
  a fault the driver caused.
- **Touch targets**: `tap-target` (48px) everywhere, `tap-target-driver` (56px) for Driver
  primary actions.
- **IDs and times are monospace.** `<Mono>` and `<ClockTimeText>` from `@waypoint/ui`, so
  columns of `OUT001` and `07:34` line up.
- A screen used by one role lives in that role's folder. Only genuinely cross-role pieces go
  in `libs/ui`.
- If you add a lib that renders markup, add an `@source` line in `global.css` — Tailwind v4
  will not find its classes otherwise, and they compile to nothing silently.

---

## Where a change belongs

| Changing                                | Goes in                                                                           |
| --------------------------------------- | --------------------------------------------------------------------------------- |
| A constraint code, order status, or DTO | `libs/shared-types` **and** `apps/api/internal/domain`, same commit               |
| A feasibility rule's logic              | the Go validator only                                                             |
| A cross-role component                  | `libs/ui`                                                                         |
| One role's screen                       | `apps/web/src/app/<role>/`                                                        |
| An endpoint                             | `apps/api/internal/httpx` + a method on `WaypointClient` + a row in `docs/api.md` |

**The contract rule:** anything in `libs/shared-types/src/lib/{domain,constraints}.ts` has a
counterpart in `apps/api/internal/domain`. Change both in one commit. A rename on one side
alone compiles cleanly and fails at runtime, which is the worst kind of bug to find on
demo day.

---

## Branches and commits

```
feat/dispatcher-planning-board
feat/api-constraint-validator
fix/loader-reverse-stop-order
```

Branch from `main`, keep branches short-lived, open a pull request. CI runs lint, test and
build on every push.

Commit messages: what changed and why, imperative mood.

```
Add reefer check to constraint validator

Chilled and frozen orders were being placed on dry-box vehicles because
the temperature check ran against the order line rather than the order
header. Adds the header-level check plus boundary tests.
```

**Code pushed after the deadline is not considered.** Push early and often; do not sit on a
branch.

---

## Before the deadline

- [ ] `docker compose down -v && docker compose up --build` works from a clean clone
- [ ] All four seeded accounts sign in and reach their workspace
- [ ] The judge walkthrough in the README runs start to finish without a dead end
- [ ] The deployed public URL is live and stays live
- [ ] Any departure from the Day 5 design is recorded in the README
- [ ] `docs/ai-disclosure.md` is complete and honest
- [ ] The demo video shows all four roles, 5–8 minutes, unlisted on YouTube
- [ ] `npx nx run-many -t lint test build` is green
