# Nexora_Waypoint

**Waypoint Delivery Planning System** — one plan, four roles, every outlet served or explained.

Team **Nexora** · Rootcode Tech-Triathlon 2026 · Hackathon (Day 10) submission.

A delivery planning system for Waypoint Group: 120 outlets, 60 vehicles, two depots
(Peliyagoda and Kandy) and three brands — Fresh, Style and Tech — sharing one fleet.
On a typical day the fleet cannot serve everyone, so the system's real job is not only
to build a plan but to say **which orders were deferred and why**.

> **Status: feature-complete for the judge walkthrough.** All four role workspaces are
> built and wired to the Go API, with one constraint engine, transactional confirmation,
> Go-owned sessions and idempotent offline sync. A small number of submitted behaviours
> remain documented departures — see
> [Implemented, partial and departed](#implemented-partial-and-departed), which is the
> authoritative status list. `README.md` and the code are kept in step; where they differ,
> the code wins.

---

## Contents

- [Run with Docker Compose](#run-with-docker-compose)
- [AWS Deployment](#aws-deployment)
- [Quick start](#quick-start)
- [Seeded accounts](#seeded-accounts)
- [Judge walkthrough](#judge-walkthrough)
- [Architecture](#architecture)
- [Workspace layout](#workspace-layout)
- [Common commands](#common-commands)
- [Implemented, partial and departed](#implemented-partial-and-departed)
- [Build status](#build-status)
- [Documentation](#documentation)

---

## Run with Docker Compose

**This is the guaranteed, reproducible way to run Waypoint. It needs no cloud account and no
AWS credentials.** A fresh clone runs the complete application with one command.

```bash
cp .env.example .env
docker compose up --build
```

Authentication is owned by the Go API: `POST /api/v1/auth/login` verifies an Argon2id password
hash and returns an opaque session token (only its SHA-256 hash is stored). The web client keeps
the token and sends it as `Authorization: Bearer` on every call. `SESSION_TTL` controls how long
a session lasts (default `12h`); see `docs/api.md`.

1. **Configure (optional).** The defaults work as-is. To change ports or the database
   password, edit `.env` after copying it.
2. **Start the stack.** The command above builds and starts PostgreSQL 17, RabbitMQ 4, the Go
   API (which runs migrations then seeds reference data) and the Next.js web app.
3. **Open the frontend** at <http://localhost:3000>. The API health check is
   <http://localhost:8080/healthz>; the RabbitMQ management UI is <http://localhost:15672>.
4. **Sign in** using the [seeded demo accounts](#seeded-accounts) (password `waypoint2026`).
5. **Local media storage.** Loader shortfall photos and driver POD photos are stored on the
   local filesystem at `MEDIA_ROOT` (`/data/media`), backed by a named Docker volume
   (`media-data`). `docker compose down` keeps them; `docker compose down -v` deletes them.
   No S3 bucket or AWS credentials are involved (see
   [`docs/deployment.md`](docs/deployment.md)).
6. **Stop** with `docker compose down` (keeps data) or `docker compose down -v` (wipes data
   and media).

| Service             | URL                             |
| ------------------- | ------------------------------- |
| Web app             | <http://localhost:3000>         |
| API health          | <http://localhost:8080/healthz> |
| RabbitMQ management | <http://localhost:15672>        |

---

## AWS Deployment

AWS is the **hosted deployment target**; Docker Compose above is the reproducible fallback.
Neither replaces the other, and AWS is never required to run locally.

The hosted stack is EC2 (Nginx + Next.js + Go API) with a private **S3** bucket for media,
**Neon** PostgreSQL and **RabbitMQ**. Configuration is entirely environment-driven
(`MEDIA_STORAGE=s3`, `S3_BUCKET`, `AWS_REGION`, `DATABASE_URL`); credentials come from an EC2
instance role, never the repository. `/healthz` and `/readyz` expose liveness and readiness.

Full procedure: [`docs/deployment.md`](docs/deployment.md). Media storage is a **team
deployment decision**, not a Challenge Booklet requirement.

---

## Quick start

### With Docker — the one command a judge needs

```bash
cp .env.example .env     # has working local defaults; change the DB password before deploying
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

The sign-in screen lists all four roles so a judge knows which account to use. Choosing a role only highlights the workspace — the email and password fields are never filled in; the judge types the seeded credentials above. The password is not present in the client bundle.

**Seeded demo day.** Planning day **Friday 25 September 2026**, delivery day
**Saturday 26 September 2026** — the Task 2B peak-day scenario S1, Peliyagoda: **85 confirmed
orders** (26 chilled, 59 ambient) and **10 vehicles in the workshop**. The supplied fleet file
lists 38 of the 60 vehicles; the 22 it omits are treated as available, so 50 are available.
These are the imported figures; the design mock-ups quoted different ones (186 / 54 / 6).
How many orders the fleet cannot serve is decided by the planning engine, not by the seed.

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

## Implemented, partial and departed

This is the authoritative status list. "Departure" means a submitted Day 5 behaviour that
is intentionally not built; each has its reason, and the screen says what it does instead of
faking the missing behaviour.

### Implemented

- **Go backend instead of Spring Boot** (see below). One authoritative constraint validator,
  transactional confirmation, optimistic route versions, Go-owned opaque sessions.
- **Authentication and RBAC.** Argon2id passwords, opaque server-side sessions, login /
  logout / me, role and depot/outlet scope enforced on every handler. All four roles sign out.
- **Orders.** Place (SKU-based, server-computed totals, snapshot dimensions), explicit
  **Confirm order**, the 16:00 cutoff enforced server-side with the delivery day rolled to the
  next operating run, and the confirmed order queue.
- **Dispatcher.** KPI dashboard with the cutoff countdown, unified order queue, **close queue**
  per brand (idempotent), async planning proposal and review, mandatory deferral reasons with
  bulk apply, transactional confirm/publish, routes and trip tracker, deferral history with
  repeat-skip highlighting, fleet, notifications and audit trail.
- **Planning engine.** Every hard constraint with the official trip-time formula, the
  documented [prioritisation policy](docs/prioritisation-policy.md) (deferred-yesterday,
  starved outlets, chilled-first, narrow windows, largest order, Fresh before Style/Tech),
  and full revalidation inside the confirmation transaction.
- **Loader.** Depot-scoped trips, picking list in **reverse stop order**, per-line check-off,
  and order-line shortfall flags (missing/damaged + optional photo) that notify the
  dispatcher and are audited.
- **Driver.** Run-sheet cockpit, Delivered/Failed/Delayed outcomes with reasons, POD
  (receiver + signature or photo), a local-first IndexedDB outbox, idempotent
  `client_event_id` sync, and visible pending/synced/conflict states. Offline is neutral grey.
- **Store manager.** Store home, place order, live/completed order list, order detail with the
  arrival window, and the **deferral reason** on a deferred order.
- **Audit trail** for delivery, loading, queue close, allocation confirmation and deferral,
  written in the same transaction as the mutation.
- **Demo clock** (`DEMO_MODE`) so the seeded past delivery day is "today" for the walkthrough.

### Partial

- **D-03 plan board** reviews and confirms the engine's proposal but has no drag-and-drop and
  no `POST /allocations/validate` / `recalculate` probe, so a candidate move is not previewed.
  Deferral reasons are given inline rather than in the D-04 modal. A `POLICY` override UI is
  not exposed.
- **D-06 tracker** shows progress from each stop's recorded status; per-trip driver, ETA and
  live offline/sync status need `GET /routes/live`, which is not implemented.
- **Loader "Mark trip loaded"** is intentionally not built: there is no route-level `LOADED`
  transition endpoint, so the screen ends at "Save counts" and shows server-side
  `routeReady` instead.
- **L-02a live plan changes** are not detected: the `route` table has `route_version`, but no
  code increments it and there is no server-side reorder endpoint, so polling would never
  see a change. The picking list holds the loader's unsaved counts.

### Departed (documented, not built)

- **D-08 forecast.** `GET /forecast/demand` is not implemented and forecasting belongs to the
  Datathon; the screen is not in the navigation.
- **DG-C breakdown recovery.** No mark-broken-down action or requeue drawer; a breakdown is
  currently handled as an explicit deferral.
- **S-06 receipt / GRN.** `POST /orders/{id}/receipt` and `GET /orders/{id}/eta` are not
  implemented, so no receipt form or per-order ETA is offered. The order detail shows the
  outlet's real delivery window and the deferral reason instead.
- **Driver account screen.** The header sign-out button ends the session; there is no separate
  account page (the two disabled tabs, Deliveries and Vehicle, have no API yet).

### Backend language: Go instead of Spring Boot

Our specification named Spring Boot. We build in **Go 1.24**. Everything the specification
requires of the backend is unchanged — one authoritative constraint validator, DTOs at the
boundary, transactional confirmation, optimistic locking on route edits, a Go-owned session
with server-side depot and outlet scope, idempotent offline sync, an immutable audit trail.

### Fonts self-hosted rather than loaded from a CDN

Inter and Cousine ship with the app via Fontsource instead of Google Fonts, so the Driver
surface renders with no connectivity.

### Any further departures

Record them here as they happen, with the reason. An undocumented departure costs marks
under _fidelity to the Day 5 design_; a documented one does not.

---

## Build status

Verified for this submission:

- `npx nx run-many -t test` — the Jest suites (web, shared-types, api-client, ui) and the Go
  suite, including the constraint boundaries and the booklet's worked example in both
  languages
- `npx nx run-many -t lint` and `go vet ./...` — clean across every project
- `npx nx run-many -t build` — production web build and the static Go binary
- `tsc --noEmit` — clean under `strict` with `noUncheckedIndexedAccess`
- `docker compose config` — valid; CI builds both images and smoke-tests the API container

`docker compose up --build` starts PostgreSQL, RabbitMQ, the Go API and the web app, and
seeds the reference data and the S1 demo day. The PostgreSQL integration tests
(`TestSeedIntegration`, `TestRoutesIntegration`, `TestCloseQueueAuditIntegration`) are not
part of CI; run them with `WAYPOINT_TEST_DATABASE_URL` set (see `docs/ai-disclosure.md`).

---

## Documentation

| Document                                                         | Contents                                                                             |
| ---------------------------------------------------------------- | ------------------------------------------------------------------------------------ |
| [`docs/architecture.md`](docs/architecture.md)                   | Components, request flows, degradation behaviour                                     |
| [`docs/data-model.md`](docs/data-model.md)                       | Tables, relationships, the constraint catalogue                                      |
| [`docs/api.md`](docs/api.md)                                     | Every endpoint with role, purpose and payload                                        |
| [`docs/prioritisation-policy.md`](docs/prioritisation-policy.md) | The documented fairness ordering behind deferrals                                    |
| [`docs/ai-disclosure.md`](docs/ai-disclosure.md)                 | Required AI tool disclosure                                                          |
| [`CONTRIBUTING.md`](CONTRIBUTING.md)                             | Conventions, branch naming, and the backend handoff                                  |
| [`CLAUDE.md`](CLAUDE.md)                                         | Guidance for coding agents: rules, architecture, domain, screens, build order        |
| [`AGENTS.md`](AGENTS.md)                                         | Consolidated booklet/spec checklist: invariants, offline/sync, test matrix, delivery |
| [`docs/skills.md`](docs/skills.md)                               | Agent skills installed, skipped and why                                              |
| [`docs/deployment.md`](docs/deployment.md)                       | Compose fallback, local media, AWS EC2/S3/Nginx/Neon, procedures                     |

---

Questions to the organisers: <tech-triathlon@rootcode.io>
