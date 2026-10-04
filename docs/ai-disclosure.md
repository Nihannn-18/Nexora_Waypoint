# AI tool disclosure

Required by the Challenge Booklet for every phase. This is the team's honest account of how
AI tools were used. The competition does not penalise AI use; it penalises not declaring it.

---

## Summary

The team used **Claude (Anthropic)** as a coding assistant throughout the Hackathon phase: to
scaffold the repository, to write and refactor backend and frontend code, to write tests, to
review and document the system, and to produce this audit-and-fix pass. Every AI-assisted
change was reviewed by a team member before it was committed, and the build, lint, type-check
and test pipeline was run on the result. No competition dataset rows were pasted into a model.
The team made the design, scope and business-rule decisions; the model implemented them.

---

## Phase 1 — Designathon (Day 5, submitted)

The Day 5 design — screens, personas, visual style, the three degradation scenarios and the
written system specification — was produced by the team. Design tokens, screen identifiers
(D-01, L-02, R-01, S-04 …), personas and the E-0x rule numbering in this repository were
transcribed from that submission. They are not AI-generated design decisions.

| Tool               | Used for                                                                | Human review                                       |
| ------------------ | ----------------------------------------------------------------------- | -------------------------------------------------- |
| Claude (Anthropic) | Editing and tidying the written SRS prose; formatting the Designathon document | Team read and approved the final text before submission |

---

## Phase 2 — Hackathon (Day 10)

| Area                            | Tool               | Extent                                                                                                                                                                                                                                                                                                                                                                | Human review                                                                                                                                                                                                          |
| ------------------------------- | ------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Repository scaffold             | Claude (Anthropic) | Nx monorepo, Next.js app shell and Tailwind tokens, the three shared libraries, the Go service skeleton, Nx Go target wiring, Dockerfiles, `docker-compose.yml`, `docs/` | Every file reviewed by the team; build, test, lint and type-check run green                                                                          |
| Database schema and migrations  | Claude (Anthropic) | The numbered migrations `00001`–`00008` and `docs/data-model.md`: tables, CHECK/UNIQUE constraints, the append-only audit log. The business rules the schema encodes were supplied by the team's specification. | Reviewed against the agreed data model; the PostgreSQL integration tests exercise the real schema                                       |
| Authentication and RBAC         | Claude (Anthropic) | Go-owned opaque sessions: `app_user.password_hash`, the Argon2id `internal/password` package, the opaque-token verifier, `authstore`, `authapi` (login/logout/me), seeded hashed demo credentials, and the media scope authorizer | All four seeded roles were logged in and out by hand; wrong-role, out-of-scope and absent tokens are rejected; `go test ./...` green |
| Constraint validator            | Claude (Anthropic) | The 17 hard rules and the official trip-time formula as pure functions in `internal/planning`, mirrored in `libs/shared-types`, with the booklet's pinned vector on both sides | Reviewed against the booklet; a table-driven test covers each rule's passing, failing and boundary case |
| Planning engine and prioritisation | Claude (Anthropic) | The deterministic assignment engine, the job runner and the documented prioritisation policy (a lexicographic policy, not a numeric score) | Reviewed against `docs/prioritisation-policy.md`; unit tests pin each policy rule and the constraint boundaries |
| Allocation confirmation         | Claude (Anthropic) | The transactional revalidate-and-persist path (`internal/routes`): row locks, in-transaction duplicate check, atomic route/leg/allocation/deferral writes | Reviewed; integration tests cover atomicity, idempotency and concurrent confirmation |
| Dispatcher screens              | Claude (Anthropic) | Dashboard, order queue and close, plan review and confirmation, routes/tracker, deferral log, fleet, notifications, audit | Reviewed by the team; verified against PostgreSQL with real sessions after the auth merge |
| Loader screens                  | Claude (Anthropic) | L-01 trips, L-02 reverse-order picking list, L-03 flag sheet, flags and account tabs, and the depot-scoped `GET /loading/routes` | Reviewed; shortfall photo upload and cross-depot refusal verified by hand |
| Driver screens and offline sync | Claude (Anthropic) | R-01/R-02/POD/DG-B, the IndexedDB outbox and idempotent reconcile, service worker, `GET /driver/routes` | Reviewed; Go and Jest tests green; offline capture and reconnection exercised |
| Store manager screens           | Claude (Anthropic) | S-01 home, S-02/S-03 place order and **confirm order**, S-04/S-05 detail (including the deferral reason), S-07 list | Reviewed; store tests assert rendered states and server-scoped requests |
| Audit, notifications and demo clock | Claude (Anthropic) | `internal/audit`, `internal/notify`, `internal/clock`, `GET /meta`; the queue-close, confirmation and deferral audit writes added in the final pass; the demo-mode stage jump and transactional reset (`internal/demo`, `POST /demo/clock`, `POST /demo/reset`) and the dispatcher's demo controls | Reviewed; integration tests assert the audit rows commit with their mutation |
| Seed data                       | Claude (Anthropic) | Go import/seed code for the reference CSVs, the four demo accounts and the S1 demo day, plus tests. **The supplied CSV datasets were provided by a human teammate and were not generated, fabricated or synthesised.** | Reviewed by the team; the seed integration test checks totals reconcile |
| Tests                           | Claude (Anthropic) | Jest suites for the web and shared libraries, and the Go unit/integration tests | Run on every change via `npm run verify` and `go test ./...`; the team does not weaken a failing test to make CI pass |
| Documentation                   | Claude (Anthropic) | `README.md`, `docs/architecture.md`, `docs/api.md`, `docs/data-model.md`, `docs/deployment.md` | Reviewed and corrected by the team where the code and prose disagreed |
| Dispatcher master data and assignments | Claude (Anthropic) | Vehicle/outlet create-edit (`internal/catalog/network_write*.go`), the `driver_vehicle_assignment` migration and `internal/assignment`, the assignment screens, and driver-run narrowing in `internal/delivery` | Reviewed; Go unit/handler/integration tests and Jest component tests pin RBAC, validation, conflict and scoping behaviour |
| Dispatcher account management and password reset | Claude (Anthropic) | The `internal/useradmin` package (create/list/edit/deactivate operational accounts), the `password_reset_token` migration (`00010`) and the `app_user.created_at` column, the public forgot/reset endpoints, the `Users` screen and the forgot/reset screens | Reviewed; the server-side role/scope rules (no DISPATCHER creation, depot/outlet shape, store-manager depot derivation, Argon2id hashing, token hashing/single-use, generic forgot reply, session revocation) were chosen by the team and pinned by Go unit/handler/integration and Jest tests |
| Release-validation fixes | Claude (Anthropic) | Planning results read back each deferral's binding constraint; confirmation rejects an omitted DEFER proposal, keeps the engine's constraint when none is sent, defaults the deferral's target date to the next operating day from `calendar_day`, and stamps `decided_at` from the injected clock; seeded S1 orders take business-date timestamps | Found during release validation against the running stack; pinned by Go unit and PostgreSQL integration tests |
| Validation UX fixes | Claude (Anthropic) | Sign-in without hardcoded accounts and a show/hide password field (also on reset); Store layout and responsive pass with a Deferred view on My orders; Fleet as the single vehicle list (Vehicles redirects to it); notifications behind the top-bar bell only; signed-in name in the Dispatcher sidebar; one Sign out per shell; the 16:00 cutoff countdown computed from the device clock | Investigated against the running app at 1440/1024/402 px before and after; Jest specs updated and added |
| Operational assignment workflows | Claude (Anthropic) | Loader→depot assignment (reusing `app_user.depot_id`), the day's driver-vehicle assignment list, store-manager depot derivation on assignment, the `LOADER_ASSIGNED`/`LOADER_UNASSIGNED` audit actions, and the unified `Assignments` Dispatcher screen | Reviewed; Go unit/handler tests and Jest component tests pin loader/manager depot derivation, the bulk driver read, RBAC (dispatcher-only mutations, driver/loader self-scoped reads) and the three-workflow UI |

### Final judge-readiness pass

A later review pass (on the `fix/waypoint/final-judge-readiness` branch) audited the merged
work and implemented: removing credential autofill from sign-in, adding explicit order
confirmation, resolving the loader/driver run date from planned routes, consistent logout for
all roles, restricting order creation by role, enforcing the cutoff delivery date server-side,
auditing dispatcher decisions, implementing the prioritisation policy, and exposing the
deferral reason to the store. These changes were AI-assisted and reviewed by the team.

### Dispatcher master data and operational assignments

A further pass (on the `feature/waypoint/dispatcher-master-data` branch) added Dispatcher
vehicle and outlet create/edit (server-validated, server-generated immutable ids, no hard
delete, audited in the same transaction), a date-based `driver_vehicle_assignment` (migration
`00009`), the store-manager-to-outlet assignment over the authoritative `app_user.outlet_id`,
the assignment screens on the vehicle/outlet detail pages, a deterministic S1 driver
assignment in the seed, and driver-run narrowing in `GET /driver/routes`. The business rules
(one driver/vehicle per operating date, depot compatibility, assigning a manager releases the
previous one) were chosen by the team; the model implemented them. Reviewed and verified with
Go unit/handler tests, web Jest tests and the full `npm run verify` pipeline.

### Integration test status — NOT CONFIGURED in CI

The PostgreSQL integration tests (`TestSeedIntegration`, `TestRoutesIntegration`,
`TestCloseQueueAuditIntegration`, `TestAssignmentIntegration`, `TestUserAdminIntegration`)
skip when `WAYPOINT_TEST_DATABASE_URL` is unset, so `npm run verify` and `nx test api` do
**not** run them; CI has no PostgreSQL service. This is reported honestly as **NOT
CONFIGURED**, not as passing. Run them by hand:

```bash
docker compose up -d postgres
WAYPOINT_TEST_DATABASE_URL='postgres://waypoint:waypoint@localhost:5432/waypoint?sslmode=disable' npx nx test api
```

`TestResetIntegration` (`internal/demo`) empties every operational table, so it additionally
needs `WAYPOINT_TEST_DEMO_RESET=1` and should be pointed at a disposable database, run on its
own rather than alongside the other packages' integration tests.

---

## Phase 3 — Datathon (Day 15)

The Datathon is separately judged and not part of this Hackathon build. If the team submits
it, the disclosure below will be completed for that phase. Its rules bind anyone working on
it, agents included: no pre-trained models, no proprietary API-based modelling or
preprocessing, no low-code or fully automated end-to-end modelling tools.

- [x] No pre-trained models were used to build the Hackathon system.
- [x] **No competition data was sent to any external modelling or preprocessing API.** In
      particular, route and travel data was not sent to a commercial routing API: Task 2B uses
      the supplied `district_travel.csv` and `service_allowance.csv` with the official formula.
- [x] No low-code, no-code or fully automated end-to-end modelling tools were used.
- [x] The supplied datasets were not shared, published or used outside the competition.

---

## What the team kept human

- The business rules, the prioritisation order and the deployment choices — the model
  implemented them, it did not choose them.
- The Figma screen design and personas, carried over from the Designathon.
- Placing the supplied competition CSVs in the repository, and every decision about the
  Datathon boundary.
- Reviewing each AI-assisted change and running the full verification pipeline before commit.
