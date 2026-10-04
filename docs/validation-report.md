# Final System Validation Report — Waypoint Delivery Planning System

Date: 2026-10-04
Branch: `feature/waypoint/final-system-validation`
Base: `development` (`b269621`)
Scope: Agent 14 — final QA/integration pass against the **real running stack**.

This pass validated the system as an integrated application — Browser → Next.js →
Go API → PostgreSQL → RabbitMQ — not just as static code. Every live claim below
was executed against `docker compose` on this machine. Where something could not
be exercised it is marked **BLOCKED** or **NOT TESTED**, never PASS.

## Environment status

| Check | Result |
| --- | --- |
| Docker daemon (`docker info`) | **PASS** — available (28.3.3) |
| `docker compose up -d --build` | **PASS** — api, web, postgres, rabbitmq all healthy |
| `GET /api/v1/meta` | **PASS** — `{now, demoMode:true, timezone}` reachable |
| `GET /healthz` / web `/signin` | **PASS** — 200 |

> The first `up` failed because the local, uncommitted `.env` set `MEDIA_STORAGE=s3`
> without the rest of the S3 wiring. That is a local environment mismatch, not a
> product defect; the Compose default is `local`. The stack was started with the
> documented `MEDIA_STORAGE=local` and the S3 path was validated separately (below).

---

## 0. Defects found and fixed

Every item is a genuine implementation defect reproduced against the running
stack, fixed minimally, and covered by a regression test. No test was weakened,
skipped or deleted to obtain green.

### F1 — Malformed `depotId` returned 500 instead of a field error
- **Symptom:** `POST /users` (and vehicle/outlet create) with a non-UUID `depotId`
  returned `500 INTERNAL_ERROR` ("invalid input syntax for type uuid").
- **Root cause:** `depot_id = $1` in `useradmin/store.go` and
  `catalog/network_write.go` passed untrusted text into a `UUID` column before the
  existence check could run.
- **Fix:** compare as text (`depot_id::text = $1`), the idiom already used by
  `assignment/repository.go`. A malformed id now matches nothing and surfaces as
  `400 depotId is not a known depot`.
- **Files:** `apps/api/internal/useradmin/store.go`,
  `apps/api/internal/catalog/network_write.go`.
- **Regression:** `useradmin/integration_test.go` (malformed depot → `ErrInvalid`);
  `catalog/network_write_integration_test.go` (`DepotExists` malformed → false, nil).

### F2 — Unknown SKU returned 500 on order creation
- **Symptom:** `POST /orders` with an unknown `itemId` returned `500`.
- **Root cause:** `catalog/repository.go` `GetByID` compared `item_id = $1`; a
  non-UUID id raised a uuid-cast error that was not `pgx.ErrNoRows`.
- **Fix:** `item_id::text = $1`, so an unknown/malformed SKU is `ErrNotFound` → `400`.
- **Files:** `apps/api/internal/catalog/repository.go`.
- **Regression:** live API suite (`unknown SKU -> 400`); catalog integration suite.

### F3 — Malformed UUID path parameters returned 500
- **Symptom:** `GET /orders/{id}`, `/planning-jobs/{id}`, `/planning-jobs/{id}/results`,
  `/routes/{id}`, `/routes/{id}/legs`, `/routes/{id}/loading` and `/legs/{id}`
  returned `500` for a non-UUID id instead of `404`.
- **Root cause:** the read queries compared `order_id`/`job_id`/`route_id`/`leg_id`
  directly to a `UUID` column.
- **Fix:** `::text` casts on the client-reachable reads.
- **Files:** `orders/repository.go`, `planning/repository.go`,
  `routes/repository.go`, `loading/repository.go`, `delivery/driver_read.go`.
- **Regression:** live API probes now return `404`; package integration suites pass.

### F4 — Seed summary was not idempotent
- **Symptom:** `TestSeedIntegration` failed: the demo driver-vehicle assignment
  count reported `1` on the first run and `0` on the second.
- **Root cause:** `seedDemoAssignment` returned `RowsAffected()`, so an
  `ON CONFLICT DO NOTHING` re-run reported 0 even though the row was still there.
- **Fix:** report the assignment's presence (a stable count), keeping `DO NOTHING`
  so a dispatcher's manual reassignment of VEH014 for the demo day is not clobbered.
- **Files:** `apps/api/internal/seed/seed.go`.
- **Regression:** `seed` integration suite now passes (idempotency assertion).

### F5 — An outlet could hold more than one store manager
- **Symptom:** creating two store managers for the same outlet, or creating one for
  an occupied outlet, left multiple `app_user` rows with that `outlet_id`, breaking
  the "an outlet has exactly one manager" invariant.
- **Root cause:** `useradmin` create/update set `outlet_id` directly and did not
  release the previous manager; only the assignment endpoint did (and only one).
- **Fix:** `releaseOutletManagers` clears every other manager of the outlet (audited)
  inside the same transaction, called from `CreateAccount` and `UpdateAccount` for
  `STORE_MANAGER`.
- **Files:** `apps/api/internal/useradmin/store.go`.
- **Regression:** `useradmin/integration_test.go` (second manager releases the first).

### F6 — A failed sign-in showed no error
- **Symptom:** entering a wrong password stayed on `/signin` but never displayed the
  "We could not sign you in" message.
- **Root cause:** the shared HTTP client called `onUnauthorized` on **any** 401,
  including anonymous `POST /auth/login`; the client then redirected to `/signin`,
  wiping the form error before it rendered.
- **Fix:** only trigger the session-expiry handler for authenticated requests
  (`!options.anonymous`).
- **Files:** `libs/api-client/src/lib/http.ts`.
- **Regression:** `http.spec.ts` — anonymous 401 must not call `onUnauthorized`.

---

## 1. Automated validation

| Gate | Command | Result |
| --- | --- | --- |
| Backend tests | `go test ./... -count=1` | **PASS** (all packages) |
| Backend vet | `go vet ./...` | **PASS** |
| Backend build | `go build ./...` | **PASS** |
| Go formatting | `gofmt -l .` | **PASS** (clean) |
| Frontend verify | `npm run verify` | **PASS** (lint, typecheck, test, build) |
| Whitespace | `git diff --check` | **PASS** |

## 2. Live API validation

Executed with real sessions against `http://localhost:8080/api/v1`.

| Area | Result |
| --- | --- |
| Auth & sessions (401/403, logout revokes, no user enumeration) | **PASS** 91/91 |
| RBAC cross-role (store/loader/driver forbidden from dispatcher endpoints) | **PASS** |
| Reference data (`/depots`, `/outlets`, `/vehicles`, `/items`) | **PASS** |
| User management: email/name/role/depot/outlet/password/strict-JSON rules | **PASS** |
| Assignments: driver-vehicle, manager-outlet, loader-depot + conflicts | **PASS** |
| Orders: create/totals/cutoff/confirm/duplicate/cross-outlet | **PASS** 26/26 |
| Planning: suggest → poll → results → confirm; deferral omission rejected with nothing written; missing constraint/date derived | **PASS** |
| Loader: pick list, shortfall sum invariant, `missing > ordered` rejected | **PASS** 30/30 |
| Media (local): upload slot, PUT bytes, invalid type/purpose, cross-role/scope reads | **PASS** |
| Driver: outcomes, FAILED requires reason, POD required, reason misapplication rejected | **PASS** |
| Offline sync: per-event ACCEPTED/DUPLICATE/REJECTED, idempotent replay, 401 | **PASS** |
| Password reset: generic forgot, invalid/short/mismatch, valid, old password fails, session revoked, token reuse rejected | **PASS** 12/12 |
| Audit: records written, no secrets (`password`/hash/`token`) | **PASS** |

## 3. Integration validation (PostgreSQL)

Each package was run against a freshly created database so cross-package fixture
collisions cannot mask a result.

`assignment` `audit` `authstore` `catalog` `delivery` `demo` `loading` `notify`
`orders` `planning` `receipts` `routes` `seed` `useradmin` — **all PASS**.

## 4. Media / S3

| Path | Result |
| --- | --- |
| Local backend (Compose default) — shortfall + POD upload/download/authorize | **PASS** (live) |
| Private-key rules: server-generated keys, traversal rejected, cross-outlet read rejected, unauthenticated 401 | **PASS** (live) |
| Real S3 backend — `Put` → `Get` roundtrip, `PresignGet` minted, `Delete` | **PASS** (live, `TestS3LiveSmoke`, opt-in via `WAYPOINT_S3_TEST_BUCKET`) |
| Compose S3 wiring | **NOT CONFIGURED** — Compose passes only `MEDIA_STORAGE`/`MEDIA_ROOT`; the S3 backend was validated directly against the configured bucket. |

## 5. Playwright E2E (real stack)

`npm run e2e` (Chromium) — **23/23 PASS**. A global setup re-seeds the demo day and
confirms one real plan so the Loader and Driver workspaces have genuine routes.

- **G-01 auth:** all four roles sign in and land in their workspace; role cards never
  pre-fill credentials; wrong credentials show an in-place error; unauthenticated
  workspaces redirect to sign-in.
- **Dispatcher:** dashboard/nav, order queue with seeded orders, plan board run with
  the E-0x hard-rules panel, users list (credential-free), deferral log.
- **Store:** places an order and sees the server-issued number; outlet-scoped order
  form; My Orders.
- **Loader (402px):** trip loads in **reverse stop order**, save disabled until every
  line is counted, save succeeds; another depot's route is not readable.
- **Driver (402px):** cockpit run sheet; **FAILED requires a reason**; offline is a
  neutral "No signal · saved on this phone" state.
- **RBAC:** backend rejects store/driver on dispatcher endpoints (403) and
  unauthenticated calls (401).

## 6. Four-role end-to-end / judge walkthrough

Store order → queue close → suggest → inspect rules/deferrals → confirm plan →
loader reverse list → driver outcome + POD (with an offline save) → store status →
dispatcher tracker/audit — **PASS** via the live API suites and the Playwright
suite. Every action has a real persistent backend effect.

## 7. Remaining issues (non-blocking)

1. `GET /allocations/{date}` is documented in `docs/api.md` but is **not
   implemented** (404). It is not used by the UI (`WaypointClient` never calls it),
   so it is a documentation/endpoint gap rather than a broken screen.
2. Integration tests across packages reuse fixture ids (`OUT991`, `VEH991`); they
   must be run against a fresh database per package to avoid false collisions. This
   is a test-harness note, not a product defect.
3. The demo password-reset token is read from the API log (by design under
   `DEMO_MODE`); there is no email provider.

## 8. Blocked items

- **Compose S3 end-to-end** — Compose does not forward S3 config; the S3 backend
  itself was validated live against the configured private bucket. No API/UI S3
  wiring exists on this branch (a separate branch adds the Compose passthrough).
- Nothing else was blocked; the Docker stack was available for the whole pass.

## 9. Final verdict

**READY FOR SUBMISSION.**

- Automated (unit/build/lint/typecheck): **PASS**
- Live API across all four roles plus RBAC, planning, offline sync, media, reset,
  audit: **PASS**
- Integration suites (real PostgreSQL): **PASS**
- Playwright four-role E2E + offline: **PASS** against the real Docker stack
- Six genuine defects found and fixed with regression tests; no test weakened or
  skipped.

The only outstanding items are non-blocking documentation/harness notes and the
separate Compose S3 passthrough tracked on its own branch.
