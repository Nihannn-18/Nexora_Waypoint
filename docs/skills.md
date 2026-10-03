# Agent skills

Procedural knowledge packs installed for coding agents working on Waypoint. They
live in `.agents/skills/` (project-level, committed) and are recorded in
`skills-lock.json`. Skills are loaded on demand by the agent; they do not affect
the build, tests or runtime.

Install pattern: `npx skills add <owner/repo> --skill <name> --agent opencode`.

> Rules of engagement: skills adapt to the Waypoint architecture defined in
> `AGENTS.md` and `CLAUDE.md`; the project specification is authoritative. No
> skill is a licence to change the stack.

---

## Installed

| Skill                                 | Source                                         | Purpose                                               | Waypoint relevance                                                         | Priority     |
| ------------------------------------- | ---------------------------------------------- | ----------------------------------------------------- | -------------------------------------------------------------------------- | ------------ |
| `better-auth-best-practices`          | `better-auth/skills`                           | Correct Better Auth setup and integration patterns    | Background reference; Waypoint authentication is Go-owned, not Better Auth | Installed    |
| `better-auth-security-best-practices` | `better-auth/skills`                           | Security hardening for Better Auth                    | Background reference for session/identity security                         | Installed    |
| `email-and-password-best-practices`   | `better-auth/skills`                           | Email/password credential patterns                    | The four seeded demo accounts use password sign-in                         | Installed    |
| `api-security-checklist`              | `reason-machines/security-skills`              | API security review checklist                         | Go REST surface: RBAC, scope, strict JSON, no secret leakage               | Required     |
| `go-backend-dev`                      | `googlecloudplatform/devrel-demos`             | Go backend service patterns                           | Go 1.24 REST API conventions                                               | Required     |
| `domain-modeling`                     | `mattpocock/skills`                            | Build and sharpen the domain model, stress-test terms | Routes, allocation, deferral vocabulary (CLAUDE.md §5)                     | Required     |
| `codebase-design`                     | `mattpocock/skills`                            | Deep-module design at clean seams                     | Layered Go services + mirrored shared types                                | Required     |
| `implement`                           | `mattpocock/skills`                            | Build from a spec, driving TDD, closing with review   | Vertical-slice feature delivery                                            | Required     |
| `diagnosing-bugs`                     | `mattpocock/skills`                            | Gated diagnosis loop: failing repro → minimise → fix  | Constraint validator and planner defects                                   | Required     |
| `code-review`                         | `mattpocock/skills`                            | Two-axis review: standards + spec fidelity            | Review before merge                                                        | Required     |
| `improve-codebase-architecture`       | `mattpocock/skills`                            | Find "deepening" opportunities, report                | Architecture review of the monorepo                                        | Recommended  |
| `neon`                                | `neondatabase/agent-skills`                    | Neon overview and branch-first workflow               | Neon PostgreSQL branches for the linked project                            | Required     |
| `neon-postgres`                       | `neondatabase/agent-skills`                    | Neon Postgres patterns, driver, pooling               | Database is Neon PostgreSQL 17                                             | Required     |
| `neon-postgres-branches`              | `neondatabase/agent-skills`                    | Choosing/creating branch types for dev, preview, CI   | Branch-per-feature workflow                                                | Recommended  |
| `vercel-react-best-practices`         | `vercel-labs/agent-skills`                     | Prioritised React/Next performance rules              | React 19 / Next.js 16 App Router                                           | Required     |
| `vercel-composition-patterns`         | `vercel-labs/agent-skills`                     | Composable component architecture                     | Shared `@waypoint/ui` components                                           | Required     |
| `web-design-guidelines`               | `vercel-labs/agent-skills`                     | Web Interface Guidelines audit (spacing, type, a11y)  | Accessibility + visual-fidelity gates (AGENTS.md §13)                      | Required     |
| `frontend-design`                     | `anthropics/skills`                            | Production-grade interface design                     | Designathon screen fidelity across four roles                              | Required     |
| `shadcn`                              | `shadcn-ui/ui`                                 | shadcn/ui component usage guidance                    | UI component authoring (skill only — shadcn not initialised)               | Recommended  |
| `pwa-development`                     | `mindrally/skills`                             | PWA / service worker / offline patterns               | Offline-first Driver workflow, IndexedDB outbox (AGENTS.md §11)            | Required     |
| `tdd`                                 | `mattpocock/skills`                            | Red-green-refactor, one vertical slice at a time      | Test-with-every-change (CLAUDE.md §12)                                     | Required     |
| `playwright-best-practices`           | `currents-dev/playwright-best-practices-skill` | Playwright selectors, fixtures, CI                    | Frontend/E2E test matrix                                                   | Recommended  |
| `grill-me`                            | `mattpocock/skills`                            | Relentlessly interview a plan until resolved          | Requirements alignment before coding                                       | Recommended  |
| `aws-storage`                         | `aws/agent-toolkit-for-aws`                    | Choose/operate AWS object, file and block storage     | Guides the S3 media backend (team deployment decision)                     | Recommended  |
| `securing-s3-buckets`                 | `aws/agent-toolkit-for-aws`                    | Create and secure S3 buckets per Well-Architected     | Private-bucket + encryption posture for POD/shortfall media                | Recommended  |
| `aws-compute`                         | `aws/agent-toolkit-for-aws`                    | Provision/operate EC2 workloads                       | EC2 + Elastic IP hosting target (team deployment decision)                 | Recommended  |
| `find-skills`                         | `vercel-labs/skills`                           | Discover and install skills mid-session               | Tooling discovery                                                          | Pre-existing |

---

## Skipped

| Skill                                                                                                   | Reason                                                                                                                                                                                                                |
| ------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `next-best-practices`                                                                                   | No valid source: `vercel-labs/next-skills` is deprecated/emptied; it now lives only under `vercel/next.js` or a hidden `.agents/` path. `vercel-react-best-practices` already covers the App Router guidance we need. |
| `resolving-merge-conflicts`                                                                             | Removed from `mattpocock/skills` by its maintainer; only a stale, unmaintained skills.sh listing remains.                                                                                                             |
| `create-auth`                                                                                           | Better Auth scaffolding command; Waypoint implements Go-owned opaque sessions instead.                                                                                                                                |
| `cloudflare`                                                                                            | Waypoint does not use Cloudflare R2 in the current architecture.                                                                                                                                                      |
| `supabase` / `supabase-postgres-best-practices`                                                         | The database is Neon PostgreSQL directly; Neon-specific skills are preferred.                                                                                                                                         |
| Neon Auth                                                                                               | Authentication is Go-owned, not Neon Auth.                                                                                                                                                                            |
| `drizzle-orm`, `prisma`, `firebase`, `graphql`, `spring-boot`, `react-native`, `expo`, websocket skills | Not in the Waypoint stack; excluded by decision.                                                                                                                                                                      |
| `shadcn` initialisation                                                                                 | The skill is installed, but shadcn/Radix is **not** initialised: `libs/ui` has its own token-based components and no `components.json`.                                                                               |
| `aws-infrastructure`                                                                                    | Not published by AWS. The name resolves only to an unlicensed community skill (`shipshitdev/skills`); excluded. The official `aws-compute` and `securing-s3-buckets` cover the EC2 + S3 needs.                        |

---

## Failed

None. Every install in the required set resolved. Two skills needed a
non-default invocation:

- `better-auth-security-best-practices` is not under a root-declared container
  in `better-auth/skills`, so it required `--full-depth` (or a direct path).
- `go-backend-dev` is nested at
  `ai-ml/mcp-servers/godoctor/skills/go-backend-dev` in `googlecloudplatform/devrel-demos`;
  installed successfully with `--full-depth`. No fallback was needed.

---

## Security notes

- `web-design-guidelines` carries a Socket "warn" flag and fetches a guidelines
  markdown from `raw.githubusercontent.com` at review time. Its `SKILL.md` was
  reviewed: it contains no shell directives. Accepted.
- `shadcn` carries a Socket "warn" and contains shell-command directives
  (`!`npx shadcn@latest info --json``). It is installed as guidance only; shadcn
  is not initialised, so the CLI directives are inert unless a developer chooses
  to use shadcn.
- All skills run with full agent permissions: review before relying on one.
