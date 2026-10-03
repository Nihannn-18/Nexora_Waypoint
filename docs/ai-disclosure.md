# AI tool disclosure

Required by the Challenge Booklet for every phase. **This is a template — complete it
honestly before submitting.** Lines marked ⚠️ need the team's own input.

The competition does not penalise AI use; it penalises not declaring it. An inaccurate
disclosure is a rule violation, and a vague one invites questions in the semifinal.

---

## Summary

⚠️ _Replace with one or two sentences describing how the team used AI tools overall._

---

## Phase 1 — Designathon (Day 5, submitted)

⚠️ _Carry over the disclosure from `Nexora_Designathon`, which listed the SRS write-up among
other items. Keep the two documents consistent — a judge may read both._

| Tool | Used for | Human review |
| ---- | -------- | ------------ |
| ⚠️   | ⚠️       | ⚠️           |

---

## Phase 2 — Hackathon (Day 10)

### Repository scaffold

| Tool               | Used for                                                                                                                                                                                                                                                                                                                                                     | Human review                                                                                                                                                                                                                                                |
| ------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Claude (Anthropic) | Initialising this Nx monorepo: workspace configuration, the Next.js app shell and Tailwind design-token stylesheet, the three shared libraries (`shared-types`, `api-client`, `ui`), the Go service skeleton (config, router, JSON helpers, trip-time package), the Nx target wiring for Go, Dockerfiles, `docker-compose.yml`, and the documents in `docs/` | Every file reviewed by the team before the initial commit. The build, test, lint and type-check pipeline was executed and verified green. Docker image builds were **not** executed in the generating environment and were verified separately by the team. |

Design tokens, screen identifiers, personas, constraint rules and the E-0x rule numbering
were transcribed from the team's own Day 5 Designathon submission and the team's system
specification. They are not AI-generated design decisions.

### Application code

⚠️ _Complete as the build proceeds. Suggested granularity:_

| Area                            | Tool               | Extent                                                                                                                                                                                                                                                                                                                                                                                                                                                                    | Human review                                                                                                                                                                                                |
| ------------------------------- | ------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Database schema and migrations  | ⚠️                 | ⚠️                                                                                                                                                                                                                                                                                                                                                                                                                                                                        | ⚠️                                                                                                                                                                                                          |
| Authentication and RBAC         | ⚠️                 | ⚠️                                                                                                                                                                                                                                                                                                                                                                                                                                                                        | ⚠️                                                                                                                                                                                                          |
| Constraint validator            | ⚠️                 | ⚠️                                                                                                                                                                                                                                                                                                                                                                                                                                                                        | ⚠️                                                                                                                                                                                                          |
| Planning engine and worker      | ⚠️                 | ⚠️                                                                                                                                                                                                                                                                                                                                                                                                                                                                        | ⚠️                                                                                                                                                                                                          |
| Dispatcher screens              | ⚠️                 | ⚠️                                                                                                                                                                                                                                                                                                                                                                                                                                                                        | ⚠️                                                                                                                                                                                                          |
| Loader screens                  | ⚠️                 | ⚠️                                                                                                                                                                                                                                                                                                                                                                                                                                                                        | ⚠️                                                                                                                                                                                                          |
| Driver screens and offline sync | ⚠️                 | ⚠️                                                                                                                                                                                                                                                                                                                                                                                                                                                                        | ⚠️                                                                                                                                                                                                          |
| Store manager screens           | ⚠️                 | ⚠️                                                                                                                                                                                                                                                                                                                                                                                                                                                                        | ⚠️                                                                                                                                                                                                          |
| Tests                           | ⚠️                 | ⚠️                                                                                                                                                                                                                                                                                                                                                                                                                                                                        | ⚠️                                                                                                                                                                                                          |
| Seed data                       | Claude (Anthropic) | Go import/seed code for the reference CSVs, the four demo accounts and the Task 2B S1 demo day (orders, fleet availability, history migration `00003`), plus its tests. **The Task 1 / Task 2A / Task 2B CSV datasets were supplied by a human teammate and were not generated, fabricated or synthesised by the AI agent.** Only the Task 2B files are embedded for the Hackathon; Task 1 / 2A are Datathon inputs and are intentionally not tracked (see `.gitignore`). | Pending team review. The seed integration test (`TestSeedIntegration`) runs only when `WAYPOINT_TEST_DATABASE_URL` is set; it was run manually against PostgreSQL 17 and is **not part of CI** (see below). |

### Integration test status — NOT CONFIGURED in CI

`TestSeedIntegration` (in `apps/api/internal/seed`) is the only test that exercises the real
migrations and seed against PostgreSQL. It is **skipped** when `WAYPOINT_TEST_DATABASE_URL` is
unset, so `npm run verify` and `nx test api` do **not** run it. CI has no PostgreSQL service,
so this is reported honestly as **NOT CONFIGURED**, not as passing.

Run it by hand:

```bash
docker compose up -d postgres
WAYPOINT_TEST_DATABASE_URL='postgres://waypoint:waypoint@localhost:5432/waypoint?sslmode=disable' npx nx test api
```

Two follow-ups belong to a separate PR, not this bootstrap one:

- Add a PostgreSQL service container to `.github/workflows/ci.yml` and set
  `WAYPOINT_TEST_DATABASE_URL` so the integration test runs in CI.
- CI currently triggers on `push`/`pull_request` to **`main`** only, so pull requests based on
  `development` (as this one is) receive no CI at all. Broaden the triggers.

---

## Phase 3 — Datathon (Day 15)

⚠️ _Complete before the Datathon submission._

| Tool | Used for | Human review |
| ---- | -------- | ------------ |
| ⚠️   | ⚠️       | ⚠️           |

### Compliance with the Datathon restrictions

The booklet imposes three restrictions on the Datathon specifically. Confirm each:

- [ ] **No pre-trained models**, except for synthetic data generation or preprocessing.
- [ ] **No proprietary API-based modelling or preprocessing.** No competition data was sent
      to an external modelling or preprocessing API. In particular, route and travel data was
      **not** sent to a commercial routing API: Task 2B uses the supplied
      `district_travel.csv` and `service_allowance.csv` with the official formula.
- [ ] **No low-code, no-code or fully automated end-to-end modelling tools.**
- [ ] The supplied datasets were not shared, published or used outside the competition.

---

## What to record as you go

Noting these at the time is far easier than reconstructing them on deadline day:

- Which tool, and for what — code generation, review, debugging, documentation, data work.
- How much of the result survived review, honestly. "Generated then substantially rewritten"
  is a normal and perfectly acceptable answer.
- Who reviewed it.
- Anything the team deliberately did **not** use AI for, and why. This is worth stating: it
  shows the boundary was a decision rather than an accident.
