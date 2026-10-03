# Nexora_Waypoint

**Waypoint Delivery Planning System** — one plan, four roles, every outlet served or explained.

Team **Nexora** · Rootcode Tech-Triathlon 2026 · Hackathon (Day 10) submission.

A delivery planning system for Waypoint Group: 120 outlets, 60 vehicles, two depots
(Peliyagoda and Kandy) and three brands — Fresh, Style and Tech — sharing one fleet.
On a typical day the fleet cannot serve everyone, so the system's real job is not only
to build a plan but to say **which orders were deferred and why**.

> **Status: scaffold.** The workspace, build pipeline, shared domain contract, Docker
> stack and route shells are in place and verified. The database, the planning engine and
> the role screens are being built on top of this. See [Build status](#build-status).

---

## Contents

- [Quick start](#quick-start)
- [Seeded accounts](#seeded-accounts)
- [Judge walkthrough](#judge-walkthrough)
- [Architecture](#architecture)
- [Workspace layout](#workspace-layout)
- [Common commands](#common-commands)
- [Departures from the Day 5 design and the specification](#departures-from-the-day-5-design-and-the-specification)
- [Build status](#build-status)
- [Documentation](#documentation)

---

## Quick start

### With Docker — the one command a judge needs

```bash
cp .env.example .env     # JWT_SECRET is the only required value; a default is provided
docker compose up --build
```

| Service             | URL                             |
| ------------------- | ------------------------------- |
| Web app             | <http://localhost:3000>         |
| API health          | <http://localhost:8080/healthz> |
| RabbitMQ management | <http://localhost:15672>        |

`docker compose up` starts PostgreSQL, RabbitMQ, the Go API and the Next.js web app, and
seeds the database. Nothing else needs running.

To start again from an empty database:

```bash
docker compose down -v && docker compose up --build
```

### Without Docker — for development

Requires **Node 22**, **Go 1.24**, and PostgreSQL and RabbitMQ reachable (the compose file
can supply just those two: `docker compose up postgres rabbitmq`).

```bash
npm ci
npx nx dev web      # → http://localhost:3000
npx nx serve api    # → http://localhost:8080
```

### Neon (managed Postgres and agent tooling)

The repository is linked to a Neon project, so a developer can build against a managed
Postgres branch instead of the local container. The Neon CLI is the entry point:

```bash
npm i -g neon@latest && neon login   # one-off; opens a browser
neon skills -y                       # Neon agent skills for coding agents
neon mcp -y                          # Neon MCP server for coding agents
neon link --project-id tiny-wildflower-59177855 --branch production -y
neon config init                     # writes neon.ts
neon deploy                          # apply neon.ts and pull the branch's env
```

- `.neon` holds the link (org, project, branch) and is git-ignored.
- `neon.ts` is the branch config / infrastructure-as-code file. It declares the Neon
  services a branch should have; `neon deploy` (alias for `neon config apply`) reconciles
  the linked branch to it. `neon config plan` previews the diff first.
- `neon link` and `neon deploy` pull the branch's Neon variables into `.env.local`
  (`DATABASE_URL`, `DATABASE_URL_UNPOOLED`, `NEON_BRANCH`), which is git-ignored.
- The Neon skill lives at `.agents/skills/neon/`; the MCP server is registered in the
  project-level `opencode.json` and signs in on first use.

---

## Seeded accounts

One account per role, matching the Designathon personas. All use the password
`waypoint2026`.

| Role          | Email                     | Persona      | Designed for                 |
| ------------- | ------------------------- | ------------ | ---------------------------- |
| Dispatcher    | `priyantha.w@waypoint.lk` | Priyantha W. | Desktop, 1440                |
| Loader        | `nadeesha.p@waypoint.lk`  | Nadeesha P.  | Dock tablet 1024 / phone 402 |
| Driver        | `kasun.p@waypoint.lk`     | Kasun P.     | Phone, 402                   |
| Store manager | `ishara.s@waypoint.lk`    | Ishara S.    | Desktop or phone             |

The sign-in screen lists all four, so there is no need to type an address.

**Seeded demo day.** Planning day **Friday 25 September 2026**, delivery day
**Saturday 26 September 2026** — a festival week chosen deliberately because demand
exceeds the fleet: 186 confirmed orders, 54 vehicles available and 6 in the workshop.
That is the condition the system exists for, and the walkthrough below runs through it.

---

## Judge walkthrough

Roughly 10 minutes, covering all four roles and the overcapacity degradation path.

1. **Sign in as the Store manager** (`ishara.s@waypoint.lk`). Place an order for the next
   delivery day. Note the generated order number and that the order appears as `PLACED`.
2. Still as the Store manager, place a **second order for the same day** — one chilled, one
   dry. Fresh outlets legitimately have two orders per delivery day; the system keeps them
   separate rather than merging them.
3. **Sign in as the Dispatcher** (`priyantha.w@waypoint.lk`). On Dispatch Control, note the
   live countdown to the 16:00 cutoff and the day's KPI cards.
4. Open **Order Queue** and close the queue for the delivery day. The confirmed orders
   freeze; anything arriving later is held for the following run.
5. Open **Plan & Allocate** and request a suggested plan. The request returns immediately
   with a job id; the board fills in when the worker finishes.
6. **Read the rule panel** on the right. Each vehicle lane shows live weight, volume, time
   and fuel meters, and every rule carries its E-0x identifier from the Day 5 design.
7. **Try an infeasible move**: drag a chilled order onto a dry-box vehicle. The move is
   refused and the blocking rule (`REEFER_REQUIRED`, E-02) is named. Capacity, van-only
   access, depot, window and fuel violations behave the same way.
8. **Work the overcapacity path.** Demand exceeds the fleet, so the plan proposes
   deferrals. **Save is blocked until every deferral has a reason** — this is the heart of
   the submission. Apply one reason in bulk to all orders blocked by the same constraint,
   then publish.
9. Open **Deferral Logs**. Every deferral is there with its reason, blocking constraint,
   who decided it and when. Nothing is deferred silently.
10. **Sign in as the Loader** (`nadeesha.p@waypoint.lk`). Open a trip's load list. It is in
    **reverse stop order** — the last delivery loads first, nearest the door. Flag a missing
    item against a specific SKU line, not the trip as a whole. The dispatcher is notified.
11. **Sign in as the Driver** (`kasun.p@waypoint.lk`). Work the run sheet and record a
    delivery with proof of delivery.
12. **Go offline** (DevTools → Network → Offline, or switch off Wi-Fi). Record two more
    outcomes. The banner stays **neutral grey, never red** — losing signal in hill country
    is expected, not a fault. Both entries are saved on the device.
13. **Come back online.** The queue uploads in capture order, keeping the original device
    timestamps. Retrying a sync never double-records a delivery.
14. **Back as the Store manager**, see the arrival time update, then confirm receipt and
    report one damaged carton. For a deferred order, see the notice **with its reason**.
15. **Back as the Dispatcher**, open Trip Tracker to see live progress, and Capacity
    Forecast for predicted demand by depot, brand and week.

---

## Architecture

```
                   ┌─────────────────────────────────┐
                   │   Next.js 16 web app  (PWA)     │
                   │  Dispatcher · Loader · Driver   │
                   │        · Store manager          │
                   └────────────────┬────────────────┘
          Driver offline:           │ HTTPS / REST
          IndexedDB outbox ─────────┤ /api/v1
          → POST /sync/events       │
                   ┌────────────────┴────────────────┐
                   │          Go REST API            │
                   │  auth · orders · planning       │
                   │  routes · delivery · forecast   │
                   └────┬───────────────────────┬────┘
                        │                       │ AMQP
            ┌───────────┴──────────┐  ┌─────────┴──────────┐
            │     PostgreSQL       │  │  Planning worker   │
            │  authoritative state │  │  PlanningEngine    │
            │  + immutable audit   │  │                    │
            └──────────────────────┘  └────────────────────┘
```

The division of labour, in one line each:

- **PostgreSQL** stores facts. Derived values — remaining capacity, remaining minutes —
  are never persisted as authoritative, so a route edit cannot leave a stale number behind.
- **The Go API** calculates and validates. One constraint validator is the only authority
  on feasibility, and it revalidates the whole plan inside a transaction at confirmation
  time, however much the client already checked.
- **RabbitMQ** carries planning jobs, so a full-depot allocation run never blocks an HTTP
  request. Suggestions are written to their own tables and are never a final allocation.
- **The web app** explains the result to each role.
- **IndexedDB** protects the Driver's work when there is no signal.

Full detail in [`docs/architecture.md`](docs/architecture.md).

---

## Workspace layout

An [Nx](https://nx.dev) monorepo — one workspace, one `npm ci`, one command surface across
a TypeScript frontend and a Go backend.

```
Nexora_Waypoint/
├── apps/
│   ├── web/                   Next.js 16 · App Router · Tailwind v4
│   │   └── src/app/
│   │       ├── (auth)/signin  G-01
│   │       ├── dispatcher/    D-01 … D-09
│   │       ├── loader/        L-01 … L-04, L-T1 … L-T3
│   │       ├── driver/        R-01 … R-04, DG-B1 … DG-B3
│   │       └── store/         S-01 … S-07
│   └── api/                   Go 1.24 service
│       ├── cmd/api/           entrypoint
│       └── internal/
│           ├── config/        environment, validated at start-up
│           ├── domain/        shared vocabulary + constraint codes
│           ├── httpx/         router, JSON helpers, middleware
│           └── planning/      trip time, budgets, windows, fuel
├── libs/
│   ├── shared-types/          the frontend ⇄ backend contract
│   ├── api-client/            typed REST client
│   └── ui/                    cross-role components + design tokens
├── infra/postgres/init/       bootstrap SQL (extensions, timezone)
├── docs/                      architecture, data model, API, disclosure
├── docker-compose.yml
└── .env.example
```

### Why the domain contract lives in `libs/shared-types`

Constraint codes, the order lifecycle, the time budgets and the trip-time formula appear in
both languages. Rather than letting them drift, each has one named home:
`libs/shared-types` for TypeScript and `apps/api/internal/{domain,planning}` for Go, with
**the same test vectors pinned on both sides** — including the booklet's worked example
(37 + 9×2 + 15 + 15 + 16 = 101 minutes). If the two implementations ever disagree, a test
fails rather than a plan quietly going wrong.

The Go side is authoritative. The TypeScript mirror exists so the dispatcher's board can
update its meters without a round-trip; the server revalidates regardless.

---

## Common commands

Run from the repository root.

| Command                            | Does                                         |
| ---------------------------------- | -------------------------------------------- |
| `npx nx dev web`                   | Next.js dev server with hot reload           |
| `npx nx serve api`                 | `go run ./cmd/api`                           |
| `npx nx run-many -t test`          | Jest **and** `go test ./...`                 |
| `npx nx run-many -t lint`          | ESLint **and** `go vet`                      |
| `npx nx build web`                 | Production build (standalone output)         |
| `npx nx build api`                 | Static binary → `dist/apps/api/waypoint-api` |
| `npx nx affected -t test lint`     | Only what your branch touched                |
| `npx nx graph`                     | Project dependency graph in the browser      |
| `npx nx fmt api` / `fmt-check api` | `gofmt -w .` / fail if unformatted           |
| `npx nx tidy api`                  | `go mod tidy`                                |
| `neon status`                      | Print the linked branch's live Neon config   |
| `neon deploy`                      | Apply `neon.ts` and pull the branch's env    |

Nx has no official Go plugin. Rather than depend on a third-party one days before a
deadline, `apps/api/project.json` wraps the Go toolchain in `nx:run-commands`: the commands
behave exactly as they do when run by hand in `apps/api`, and Nx still provides caching and
`nx affected`.

---

## Departures from the Day 5 design and the specification

The competition requires significant departures to be recorded. These are ours.

### Backend language: Go instead of Spring Boot

Our specification named Spring Boot. We build in **Go 1.24**. Everything the specification
requires of the backend is unchanged — one authoritative constraint validator, DTOs at the
boundary, transactional confirmation, optimistic locking on route edits, JWT with
server-side depot and outlet scope, idempotent offline sync, an immutable audit trail. Only
the language and framework differ.

The reasons: a static binary starts in milliseconds and produces a far smaller container,
which matters for a judge running `docker compose up` on an unknown machine; and the team's
throughput over three days is higher in Go.

Where the specification names a Spring concept, read the Go equivalent:

| Specification                               | This implementation                                               |
| ------------------------------------------- | ----------------------------------------------------------------- |
| `@Transactional` on allocation confirmation | explicit `pgx` transaction around the revalidate-and-persist step |
| JPA entities with DTO mapping               | plain structs; persistence types never cross the HTTP boundary    |
| Spring `@Service` beans                     | packages under `internal/`, constructor-injected                  |
| Flyway migrations                           | SQL migrations run by the API at start-up                         |
| Spring AMQP listener                        | `amqp091-go` consumer in `cmd/worker`                             |
| Bean-validation annotations                 | explicit validation in each handler, strict JSON decoding         |

### Frontend: unchanged

Next.js with TypeScript and Tailwind, as specified. Tailwind v4 with the Day 5 tokens as
CSS custom properties, so a colour is defined once.

### Fonts self-hosted rather than loaded from a CDN

Inter and Cousine ship with the app via Fontsource instead of Google Fonts. The Driver
surface has to render correctly with no connectivity, and a CDN stylesheet is one more
thing that fails on a hill-country route.

### Any further departures

Record them here as they happen, with the reason. An undocumented departure costs marks
under _fidelity to the Day 5 design_; a documented one does not.

---

## Build status

Verified on this scaffold:

- `nx build web` — 6 routes, standalone output, all Day 5 design tokens present in the
  compiled CSS
- `nx build api` — static binary; serves `/healthz`, `/readyz` and `/api/v1/meta`, logs
  structurally and shuts down gracefully
- `nx run-many -t test` — 34 tests green (21 shared-types, 9 api-client, 4 ui) plus the Go
  suite, including the booklet's worked example in both languages
- `nx run-many -t lint` — clean across all five projects
- `tsc --noEmit` — clean under `strict` with `noUncheckedIndexedAccess`
- `docker compose config` — valid

Not yet verified: **the Docker image builds**. The scaffold was prepared in an environment
without a Docker daemon, so `docker compose up --build` has not been executed end to end.
Run it once locally before relying on it.

Not yet built: database schema and migrations, seed data, authentication, the four role
screen sets, the planning engine and worker, the offline outbox, and realtime progress.

---

## Documentation

| Document                                                         | Contents                                                                      |
| ---------------------------------------------------------------- | ----------------------------------------------------------------------------- |
| [`docs/architecture.md`](docs/architecture.md)                   | Components, request flows, degradation behaviour                              |
| [`docs/data-model.md`](docs/data-model.md)                       | Tables, relationships, the constraint catalogue                               |
| [`docs/api.md`](docs/api.md)                                     | Every endpoint with role, purpose and payload                                 |
| [`docs/prioritisation-policy.md`](docs/prioritisation-policy.md) | The documented fairness ordering behind deferrals                             |
| [`docs/ai-disclosure.md`](docs/ai-disclosure.md)                 | Required AI tool disclosure                                                   |
| [`CONTRIBUTING.md`](CONTRIBUTING.md)                             | Conventions, branch naming, and the backend handoff                           |
| [`CLAUDE.md`](CLAUDE.md)                                         | Guidance for coding agents: rules, architecture, domain, screens, build order |

---

Questions to the organisers: <tech-triathlon@rootcode.io>
