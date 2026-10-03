# Waypoint API — Go service

```bash
npx nx serve api     # from the repo root
go run ./cmd/api     # or from this directory
```

Then:

```bash
curl localhost:8080/healthz
curl localhost:8080/readyz
curl localhost:8080/api/v1/meta
```

Full endpoint reference: [`../../docs/api.md`](../../docs/api.md).
Build order and conventions: [`../../CONTRIBUTING.md`](../../CONTRIBUTING.md).

---

## Layout

```
cmd/api/main.go           entrypoint: config, logging, server, graceful shutdown
internal/
├── config/               environment settings, validated at start-up
├── domain/               shared vocabulary — constraint codes, roles, statuses
├── auth/                 Better Auth boundary, identity, RBAC, scope, middleware  ← tested
├── catalog/              SKUs: lookup, filters, read API  ← tested
├── orders/               order intake, lifecycle, read API  ← tested
├── routes/               confirmation, routes, legs, allocations, deferrals  ← tested
├── loading/              loader picking list, load counts, shortfall photo  ← tested
├── httpx/                router, JSON helpers, middleware
└── planning/             trip time, budgets + deterministic planning engine  ← tested
```

Packages to add as the service grows: `routes` (confirmation, route/leg persistence),
`deferrals`, `loading`, `delivery`, `notify`, `forecast`, `queue` (RabbitMQ), and a second
binary at `cmd/worker` for the planning worker.

---

## Module path

`waypoint.lk/api` — deliberately not a GitHub URL. This service is never imported from
outside the monorepo, and a domain-style path never has to change if the repository is
renamed or moved. Imports look like `waypoint.lk/api/internal/planning`.

---

## Dependencies

None yet. The scaffold builds on a fresh clone with no network, which is why `go.sum` does
not exist. Add what you need:

```bash
go get github.com/jackc/pgx/v5         # PostgreSQL
go get github.com/rabbitmq/amqp091-go  # planning queue
go mod tidy
```

Auth is **not** a `go get`: Better Auth in the web app owns authentication, sessions and
identity, and the Go API verifies the authenticated request. The exact verification mechanism
is TBD, so do not add `golang-jwt/jwt/v5` or `bcrypt` as the auth mechanism.

The router is `net/http`'s `ServeMux` using Go 1.22+ method-and-path patterns
(`"POST /api/v1/orders"`, `"GET /api/v1/orders/{id}"`), which covers this API without a
dependency. Swap in chi or gin if you want middleware groups — nothing in `internal/`
depends on the choice.

---

## Configuration

Every setting has a working default except `JWT_SECRET`, which is refused when
`APP_ENV=production`. Authentication is moving to Better Auth, so `JWT_SECRET` is legacy
scaffolding pending that decision. See [`../../.env.example`](../../.env.example).

| Variable            | Default                 | Notes                                                          |
| ------------------- | ----------------------- | -------------------------------------------------------------- |
| `APP_ENV`           | `development`           | `development` gives debug logs and human-readable output       |
| `PORT`              | `8080`                  |                                                                |
| `DATABASE_URL`      | localhost               | libpq connection string                                        |
| `RABBITMQ_URL`      | localhost               | AMQP URL                                                       |
| `JWT_SECRET`        | —                       | Legacy; required when `APP_ENV=production` pending Better Auth |
| `TOKEN_TTL_MINUTES` | `720`                   | 12 hours, so a driver is not signed out mid-shift              |
| `CORS_ORIGIN`       | `http://localhost:3000` |                                                                |
| `TZ`                | `Asia/Colombo`          | Validated at start-up                                          |

### Why the timezone matters more than it looks

The 16:00 cutoff and every delivery window are **wall-clock times in Asia/Colombo**. A
fallback to UTC is a five-and-a-half-hour error in the one calculation the whole system turns
on. Two defences are in place: `config.Load` refuses to start on an unknown zone, and
`main.go` imports `_ "time/tzdata"` to embed the IANA database in the binary, so a minimal
container with no tzdata package still resolves `Asia/Colombo` correctly.

---

## What is already tested

`internal/planning` has table-driven tests for:

- the official trip-time formula, including the booklet's worked example
  (37 + 9×2 + 15 + 15 + 16 = **101 minutes**)
- one-order trips having zero inter-stop journeys, three-order trips having two
- Fresh's 270-minute budget and Style+Tech's **shared** 480-minute budget, at the boundary:
  270 accepts, 271 rejects; 480 accepts, 481 rejects
- the shared pool genuinely being shared — Style minutes spent must reduce what Tech can do
- window arithmetic: early arrival waits, arrival at the closing minute is on time, one
  minute later is late
- fuel estimation and weekly quota at the boundary

The same vectors are pinned in `libs/shared-types/src/lib/trip-time.spec.ts`. **If you change
a number on one side, change it on the other in the same commit** — that pairing is what stops
the client's meters and the server's authority from drifting apart.

```bash
npx nx test api
npx nx test-cover api
```

---

## Nx targets

Defined in `project.json`, shelling out to the Go toolchain so each behaves exactly as it
does run by hand here.

| Target              | Runs                                                  |
| ------------------- | ----------------------------------------------------- |
| `serve`             | `go run ./cmd/api`                                    |
| `build`             | static binary → `dist/apps/api/waypoint-api`          |
| `test`              | `go test ./...`                                       |
| `test-cover`        | `go test -race -coverprofile` plus a coverage summary |
| `lint`              | `go vet ./...`                                        |
| `fmt` / `fmt-check` | `gofmt -w .` / fail if unformatted (CI uses this)     |
| `tidy`              | `go mod tidy`                                         |
| `docker-build`      | builds the image from the workspace root              |

---

## The two rules that must not bend

**One authority on feasibility.** Every hard rule lives in one constraint validator. The
dispatcher's UI greys out impossible moves, but `POST /allocations/confirm` revalidates the
entire plan against current database state inside the transaction that persists it. A plan
the client believed was valid is still rejected if the facts moved underneath it. Implement a
rule twice and the two copies will disagree under deadline pressure, producing a wrong plan
that no test catches.

**Suggestions are never allocations.** A planning run writes `planning_result` rows and
nothing else. Only dispatcher confirmation creates `route`, `route_leg` and `allocation`
rows. That separation is what makes a suggestion safe to be wrong.
