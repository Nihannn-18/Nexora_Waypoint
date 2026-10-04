# Final System Validation Report — Waypoint Delivery Planning System

Date: 2026-10-04
Branch: `fix/waypoint/order-business-timestamps`
HEAD: `341b08d` (fix(orders): use business date for order timestamps)
Base: `c3c7376` (development, includes merged PR #23 user management + assignments)

## Environment status

| Check | Result |
| --- | --- |
| `git status --short` | clean (no working-tree changes) |
| Docker daemon (`docker info`) | **BLOCKED** — daemon unreachable |
| `docker compose ps` | **BLOCKED** — cannot connect |
| Live API / UI validation | **BLOCKED** (no running stack) |

Because Docker is unavailable, every live API/UI test is reported as BLOCKED or
NOT TESTED. No live test is claimed as PASS. Only automated/static gates were
executed in this run.

## 1. Automated validation gates

| Gate | Command | Result |
| --- | --- | --- |
| Backend tests | `go test ./... -count=1` | **PASS** (all packages) |
| Backend vet | `go vet ./...` | **PASS** |
| Backend build | `go build ./...` | **PASS** |
| Go formatting | `gofmt -l .` | **PASS** (clean) |
| Frontend verify | `npm run verify` (nx cache reset) | **PASS** (lint 5, typecheck, test 5, build 2) |
| Whitespace | `git diff --check` | **PASS** |

Automated coverage below is recorded from inspection of the test suites; it was
run in this pass where noted. Live execution is blocked.

---

## 2. Authentication field validation

| # | Test | Input/Scenario | Expected | Actual | Status |
| --- | --- | --- | --- | --- | --- |
| A | Missing email | `{password}` | 400 | handler test `authapi/handler_test.go` | PASS (automated) |
| B | Missing password | `{email}` | 400 | handler test | PASS (automated) |
| C | Empty email | `{email:"",password}` | 400 | handler test | PASS (automated) |
| D | Empty password | `{email,password:""}` | 400 | handler test | PASS (automated) |
| E | Invalid email format | malformed | 401 generic | rules: required only; no format check on login | PASS (automated, generic 401) |
| F | Wrong password | valid email, bad pw | 401 generic | authstore test | PASS (automated) |
| G | Unknown email | unknown | 401 generic + Argon2 dummy | authstore test | PASS (automated) |
| H | Valid credentials | seeded | 200 + token | authapi test | PASS (automated) |
| I | Revoked/logged-out session | token after logout | 401 | authstore session lifecycle test | PASS (automated) |
| J | Invalid/expired session token | bad token | 401 | authstore test | PASS (automated) |
| — | No enumeration (unknown vs wrong pw message) | both 401 generic "Invalid email or password" | handler test | PASS (automated) |
| — | Unknown role at login | account with bad role | 403→generic 401 | **NOT TESTED** |
| — | Frontend login error UX | invalid creds | visible message | **BLOCKED** (live) |

---

## 3. User management — create user validation

Source rules: `apps/api/internal/useradmin/{useradmin,service,store}.go`.
Automated coverage: `service_test.go`, `handler_test.go`, `integration_test.go`.

| Area | Test | Expected | Status |
| --- | --- | --- | --- |
| Email | missing/@ missing/no dot/leading or trailing dot/whitespace | rejected (400) | PASS (automated: `TestCreateAccountInvalidEmail`) |
| Email | uppercase + surrounding whitespace | normalised lowercase/trim | PASS (automated: `TestCreateAccountDriver`) |
| Email | duplicate | 409 conflict | PASS (automated: `TestCreateAccountDuplicateEmail`) |
| Display name | missing/empty | rejected | PASS (automated) |
| Display name | whitespace-only | rejected | **NOT TESTED** (rule exists `useradmin.go:166-171`, no test) |
| Display name | leading/trailing whitespace | trimmed | PASS (automated) |
| Role | DRIVER/LOADER/STORE_MANAGER | accepted | PASS (automated) |
| Role | DISPATCHER | rejected | PASS (automated: `TestCreateAccountRejectsDispatcherRole`) |
| Role | unknown "ADMIN" | rejected | PASS (automated: `TestCreateAccountRejectsUnknownRole`) |
| Role | empty | rejected | **NOT TESTED** (covered indirectly) |
| Depot (driver/loader) | required | 400 if missing | PASS (automated: `TestCreateAccountRoleFieldRules`) |
| Depot (driver/loader) | unknown/inactive | rejected | PASS (automated: `TestCreateAccountInvalidDepot`) |
| Outlet (driver/loader) | within payload | rejected | PASS (automated: driver must not carry outlet) |
| Outlet (store manager) | required/valid | accepted | PASS (automated) |
| Outlet (store manager) | unknown | rejected | PASS (automated: `TestCreateAccountInvalidOutlet`) |
| Depot (store manager) | client value spoofed | ignored, derived from outlet | PASS (automated: `TestCreateAccountStoreManagerDerivesDepot`) |
| Password | >=8 | accepted | PASS (automated) |
| Password | <8 | rejected | PASS (automated: `TestCreateAccountPasswordPolicy`) |
| Password | empty/missing | rejected | PASS (automated) |
| Unknown JSON field | extra field present | 400 strict decode | PASS (automated: `TestCreateUserHandler`) |
| — | Live API creation of all three roles | 201 | **BLOCKED** |
| — | Frontend create-user validation UX | inline errors | **BLOCKED** |

---

## 4. User management — update / activate / deactivate

| Test | Expected | Status |
| --- | --- | --- |
| Update valid user | 200 | PASS (automated) |
| Update invalid role | rejected | PASS (automated: dispatcher not manageable) |
| Update invalid depot | 400 | PASS (automated) |
| Update invalid outlet | 400 | PASS (automated) |
| Update blank display name | rejected | **NOT TESTED** |
| Update driver/loader clear depot | rejected "is required" | **NOT TESTED** |
| Update driver/loader depot-exists recheck | 400 | **NOT TESTED** |
| Unauthorized update (non-dispatcher) | 403 | PASS (automated: `TestUserRoutesRBAC`) |
| Unauthenticated update | 401 | PASS (automated: `TestUserRoutesUnauthenticated`) |
| Deactivate user | 200, sessions revoked | PASS (automated: `TestDeactivateHandlerRevokes`) |
| Deactivated user authenticates | 401 generic | PASS (automated) |
| Activate user | 200 | PASS (automated: `TestSetActiveOperational`) |
| Reactivated user authenticates | 200 | PASS (automated) |
| Deactivate dispatcher | rejected | PASS (automated: `TestSetActiveRejectsDispatcher`) |
| No password/hash/token in responses | none | PASS (automated: `TestCreateUserHandler`) |
| Live update/activate/deactivate | — | **BLOCKED** |

---

## 5. Password reset validation

| # | Test | Expected | Status |
| --- | --- | --- | --- |
| 1 | Forgot, known email | generic 200 | PASS (automated) |
| 2 | Forgot, unknown email | identical generic 200 | PASS (automated: `TestForgotPasswordHandlerGeneric`) |
| 3 | Forgot, malformed email | generic 200 (no probe) | PASS (automated) |
| 4 | Missing email | generic/400 | PASS (automated) |
| 5 | Reset, missing token | 400 field error | PASS (automated: `TestResetPasswordValidation`) |
| 6 | Reset, invalid/unknown token | 400 single field | PASS (automated: `TestResetPasswordHandler`) |
| 7 | Reset, expired token | 400 | **NOT TESTED** (rule exists; no expiry test) |
| 8 | Reset, valid token | 200 | PASS (automated + integration) |
| 9 | New password too short | 400 | PASS (automated) |
| 10 | Confirm mismatch | 400 | PASS (automated) |
| 11 | Valid reset | new Argon2id hash | PASS (automated + integration) |
| 12 | Reuse token | rejected | PASS (automated: `integration_test.go`) |
| 13 | Existing session after reset | revoked | PASS (automated: `integration_test.go`) |
| 14 | New password login | succeeds | PASS (automated) |
| 15 | Old password login | fails | PASS (automated: `integration_test.go`) |
| — | Raw token never in response | absent | PASS (automated) |
| — | DEMO_MODE logs token only when enabled | handler `devResetLog` gated by `cfg.DemoMode` | PASS (code review; no live run) |
| — | Reset revokes sessions transactionally | yes | PASS (automated) |
| — | No Resend call in tests | no provider exists | PASS (n/a) |
| — | Live forgot/reset flow | — | **BLOCKED** |

---

## 6. Driver → Vehicle assignment validation

| # | Test | Expected | Status |
| --- | --- | --- | --- |
| 1 | Valid assignment | 200 | PASS (automated + **live verified earlier** when stack was up) |
| 2 | Missing driverId | 400 | **NOT TESTED** (rule exists `service.go:151`) |
| 3 | Unknown driverId | 404 | **NOT TESTED** |
| 4 | Non-driver user | 400 | PASS (automated: `TestAssignDriverValidation`) |
| 5 | Inactive driver | 400 | PASS (automated) |
| 6 | Missing vehicle | 400 | **NOT TESTED** (rule exists) |
| 7 | Unknown vehicle | 404 | **NOT TESTED** |
| 8 | Missing date | 400 | **NOT TESTED** (mutation path requires date) |
| 9 | Invalid date | 400 | PASS (automated + live: "26 Sep" → 400) |
| 10 | Non-canonical date | canonicalised | PASS (automated: ParseDate) |
| 11 | Depot mismatch | 400 | PASS (automated + live) |
| 12 | Duplicate driver/date | 409 | PASS (automated + live) |
| 13 | Duplicate vehicle/date | 409 (when driver differs) | PASS (automated) |
| 14 | Change assignment | 200 | PASS (automated + live) |
| 15 | Remove assignment | 200 | PASS (automated + live) |
| 16 | Remove nonexistent | 404 | PASS (automated + live) |
| 17 | Driver self-read | own only | PASS (automated + live) |
| 18 | Driver reads another's | not possible (self-scoped) | PASS (automated) |
| 19 | Non-dispatcher mutation | 403 | PASS (automated + live) |
| 20 | Unauthenticated | 401 | PASS (automated + live) |
| — | Date-based (not permanent) | yes | PASS (code + migration) |
| — | Driver run uses assignment | `delivery.narrowToAssignment` | PASS (automated) |

Note: "live verified earlier" = verified against the running Compose stack during
the prior assignment-verification task, before the current rebase. Not re-run now
because Docker is down.

---

## 7. Store Manager → Outlet validation

| # | Test | Expected | Status |
| --- | --- | --- | --- |
| 1 | Valid assignment | 200 | PASS (automated + live earlier) |
| 2 | Missing manager | 400 | **NOT TESTED** |
| 3 | Unknown manager | 404 | **NOT TESTED** |
| 4 | Non-store-manager | 400 | PASS (automated) |
| 5 | Inactive manager | 400 | PASS (automated) |
| 6 | Missing outlet | 400 | **NOT TESTED** |
| 7 | Unknown outlet | 404 | PASS (automated: `TestAssignManagerValidation`) |
| 8 | Invalid outlet | 404 | PASS (automated) |
| 9 | Depot spoofing | ignored/derived | PASS (automated + live earlier) |
| 10 | Reassignment | moves | PASS (automated) |
| 11 | Release previous manager | MANAGER_UNASSIGNED | PASS (automated + live earlier) |
| 12 | Unauthorized mutation | 403 | PASS (automated + live) |
| 13 | Cross-outlet access | 404 | PASS (live earlier: `/outlets/OUT090` → 404) |
| — | Outlet authoritative | yes | PASS |
| — | Depot derived on read+write | yes | PASS |

---

## 8. Loader → Depot validation

| # | Test | Expected | Status |
| --- | --- | --- | --- |
| 1 | Valid assignment | 200 | PASS (automated + live earlier) |
| 2 | Missing loader | 400 | **NOT TESTED** |
| 3 | Unknown loader | 404 | **NOT TESTED** |
| 4 | Non-loader | 400 | PASS (automated) |
| 5 | Inactive loader | 400 | PASS (automated) |
| 6 | Missing depot | 400 | PASS (automated) |
| 7 | Unknown depot | 404 | PASS (automated + live) |
| 8 | Inactive depot | 404 | PASS (DepotExists requires is_active; automated) |
| 9 | Change assignment | 200 | PASS (automated + live earlier) |
| 10 | Remove assignment | 200 | PASS (automated + live earlier) |
| 11 | Loader self-read | own only | PASS (automated + live earlier) |
| 12 | Another loader's assignment | self-scoped, not possible | PASS (automated) |
| 13 | Loader mutation | 403 | PASS (automated + live) |
| 14 | Other roles mutation | 403 | PASS (automated + live) |
| — | Depot from session, not params | yes | PASS |
| — | Loading scoped to loader depot | `identity.DepotID` | PASS (automated) |

---

## 9–16. Vehicle / Outlet / Order / Planning / Loader / Driver / Offline / Media

These areas exist and have automated coverage (catalog, orders, planning, routes,
loading, delivery, media packages all pass `go test`), but were **not** the focus
of this validation pass and could **not** be live-tested because Docker is down.
Status: **BLOCKED / NOT TESTED (live)**, automated: PASS (package suites run).

## 17. RBAC matrix

| Action | Unauthenticated | Wrong role | Automated | Live |
| --- | --- | --- | --- | --- |
| User management | 401 | 403 | PASS | BLOCKED |
| Assignment mutation | 401 | 403 | PASS | PASS (earlier) |
| Driver self-read | 401 | 403 | PASS | PASS (earlier) |
| Loader self-read | 401 | 403 | PASS | PASS (earlier) |
| Planning / master data / audit | 401 | 403 | PASS | BLOCKED |

Mechanism: `auth/middleware.go` `RequireRole`/`RequireAuthenticated`; predicates
in `auth/auth.go`; error mapping 401/403 at `middleware.go:183-196`.

## 18. Audit validation

| Event | Emitted at | Automated assert | Sensitive-free |
| --- | --- | --- | --- |
| USER_CREATED | `useradmin/store.go:175` | Yes (`integration_test.go`) | Yes (email+role only) |
| USER_UPDATED | `useradmin/store.go:234` | **No test** | Yes |
| USER_ACTIVATED / USER_DEACTIVATED | `useradmin/store.go:273-275` | **No test** | Yes |
| DRIVER_ASSIGNED / UNASSIGNED | `assignment/repository.go:233,271` | Yes | Yes |
| MANAGER_ASSIGNED / UNASSIGNED | `assignment/repository.go:329,351,399` | Yes | Yes |
| LOADER_ASSIGNED / UNASSIGNED | `assignment/repository.go:476,515` | **No test** | Yes |
| QUEUE_CLOSED / ALLOCATION_CONFIRMED / ORDER_DEFERRED / shortfall / delivery | other packages | package tests | Verified by inspection |

Finding: `audit/audit.go` defines no `USER_*` or `LOADER_*` constants; useradmin
and the loader-audit use raw string literals, while the shared TS
`AUDIT_ACTIONS` list does include `USER_CREATED`/`USER_UPDATED`/`USER_ACTIVATED`/
`USER_DEACTIVATED`. Go and TS remain compatible on the wire (same literal
strings); this is a maintainability inconsistency, not a live defect.
No audit row is written for password reset (by design — reset is self-service).
No sensitive values (password/hash/token) are stored in audit detail.

## 19–20. Frontend UX / End-to-end judge flow

**BLOCKED** — Docker (web) unavailable. Not tested in this pass.

---

## Critical failures

None identified in the executed (automated) gates. Docker unavailability is an
environment blocker, not a product defect.

## Non-critical failures

1. **Untested validation branches** (rules implemented, no dedicated test):
   - useradmin: whitespace-only displayName; update clearing a driver/loader
     depot; expired reset token; unknown-role login; empty `userId` paths.
   - assignment: required-id branches (missing driverId/vehicleId/date/manager/
     loader/outlet), unknown driver/vehicle, driver with no home depot.
   - audit: `USER_UPDATED`, `USER_ACTIVATED`, `USER_DEACTIVATED`,
     `LOADER_ASSIGNED`, `LOADER_UNASSIGNED` emission not asserted.
2. **Go/TS audit-constant divergence**: TS `AUDIT_ACTIONS` lists `USER_*`; Go
   `audit.go` has no matching constants (raw strings used). Cosmetic/maintainability.

## Blocked tests

All live API and live UI tests (sections 2–20 "live" rows and sections 9–16,
19, 20) are BLOCKED because the Docker daemon is unreachable and the Compose
stack is not running.

## Automated coverage

Backend and frontend automated gates all PASS on HEAD `341b08d`. The
user-management and assignment packages carry extensive unit/handler tests plus
DB integration tests (integration tests skip without
`WAYPOINT_TEST_DATABASE_URL`; not run in CI).

## Live API coverage

In this pass: none (Docker down). In the earlier assignment-verification pass
(on the pre-rebase branch): all three assignment workflows, RBAC 401/403,
conflicts (409), depot mismatch (400), six audit events, and driver/loader/store
scoping were verified live and passed.

## Live UI coverage

None in this pass (BLOCKED). Earlier: `/dispatcher/assignments` served 200 and
the page was reachable.

## Final release verdict

**BLOCKED — NOT CERTIFIED FOR RELEASE IN THIS PASS.**

- Automated (static/unit/handler/build) validation: **PASS**.
- Live API/UI validation: **BLOCKED** by Docker being unavailable — cannot be
  claimed.

Once the Docker daemon is restored and the stack is re-created
(`docker compose up -d --build`; no `--no-cache`, no prune), the BLOCKED live
sections (2, 3, 4, 5, 9–20) must be executed before a final verdict. The earlier
live results for assignments/RBAC/audit suggest those areas are healthy, but the
current pass did not re-execute them.

Provisional (automated-only) verdict: **READY WITH NON-BLOCKING ISSUES**, pending
successful live re-validation.
