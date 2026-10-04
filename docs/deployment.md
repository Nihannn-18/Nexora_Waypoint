# Deployment & infrastructure

Two ways to run Waypoint:

- **Docker Compose** — the guaranteed reproducible fallback. Works from a fresh
  clone with no cloud account. This is the path a judge uses.
- **AWS + Vercel** — the hosted deployment target. Optional, and never a
  prerequisite for the Compose path.

> AWS, Vercel and S3 are **team implementation/deployment decisions**, not
> requirements of the Challenge Booklet. The official functional requirements are
> unchanged by the choice of host or object store.

---

## Local / fallback — Docker Compose

```bash
cp .env.example .env
docker compose up --build
```

That single command starts every component the application needs:

| Service    | What it is                                  | URL                             |
| ---------- | ------------------------------------------- | ------------------------------- |
| `postgres` | PostgreSQL 17, the authoritative store      | localhost:5432                  |
| `rabbitmq` | RabbitMQ 4, the planning job queue          | localhost:15672 (management UI) |
| `api`      | Go REST API; runs migrations, then the seed | http://localhost:8080/healthz   |
| `web`      | Next.js app, all four role workspaces       | http://localhost:3000           |

On `api` start-up the binary connects, applies the embedded goose migrations,
then runs the idempotent reference seed. Nothing else needs running.

To reset everything, including the database and uploaded media:

```bash
docker compose down -v && docker compose up --build
```

### Local media storage

Media (loader shortfall photos, driver POD photos) uses the **local filesystem**
by default:

```
MEDIA_STORAGE=local
MEDIA_ROOT=/data/media
```

`/data/media` is a **named Docker volume** (`media-data`), so:

- `docker compose down` **preserves** uploaded media.
- `docker compose down -v` **deletes** it — this is the deliberate full reset.

No AWS credentials are needed for this path. See
[`docs/api.md`](api.md#media-proof-photos) for the upload/download flow.

### Stopping

```bash
docker compose down          # stop containers, keep data + media
docker compose down -v       # stop and wipe data + media
```

---

## Hosted — AWS + Vercel

The hosted topology does **not** put Next.js or PostgreSQL on EC2. The frontend is
on Vercel; the database is Neon; only the Go API and RabbitMQ run on a single
small EC2 instance.

```
GitHub ── Actions (OIDC, no keys) ──► ECR ──► EC2 (docker compose)
                                              ├─ api      (Go, :8080, loopback only)
                                              ├─ rabbitmq (internal network only)
                                              └─ caddy    (:80/:443, Let's Encrypt)

Vercel ── apps/web (Next.js) ──HTTPS──► https://<ip>.sslip.io/api/v1
                                              │
                                              ├─ Neon PostgreSQL (DATABASE_URL)
                                              └─ private S3 bucket (media)
```

### Components

| Piece       | Choice                                                                                    |
| ----------- | ----------------------------------------------------------------------------------------- |
| Frontend    | **Vercel**, project root `apps/web`, builds through Nx from the workspace root            |
| API         | Go binary in a container, image in **ECR**, tagged with the commit SHA                    |
| Runtime     | One **EC2 `t3.small`** (amd64) in the default VPC, static **Elastic IP**                  |
| TLS / edge  | **Caddy** on the instance; Let's Encrypt cert for a free `<ip>.sslip.io` hostname         |
| Queue       | **RabbitMQ 4** as a compose service, no published ports                                   |
| Database    | **Neon PostgreSQL** — never EC2                                                            |
| Media       | Existing **private S3** bucket; credentials from the EC2 instance role                    |
| Delivery    | **GitHub Actions → ECR → SSM Run Command**; the repo is not cloned onto EC2               |

The production stack lives in [`infra/deploy/docker-compose.prod.yml`](../infra/deploy/docker-compose.prod.yml),
which defines only `rabbitmq`, `caddy` and `api`. The root `docker-compose.yml`
remains the judge stack (Postgres + RabbitMQ + API + web).

### Why `sslip.io`?

The frontend is HTTPS, so a plain `http://<ip>:8080` API would be blocked by the
browser as mixed content. Rather than buy a domain, Caddy obtains a real Let's
Encrypt certificate for a free `sslip.io` name that resolves to the Elastic IP
(e.g. `203-0-113-10.sslip.io`). No DNS account, no cost.

### Configuration

Configuration is entirely environment-driven; nothing is hard-coded.

| Variable             | Purpose                                    | Source in production        |
| -------------------- | ------------------------------------------ | --------------------------- |
| `DATABASE_URL`       | Neon connection (libpq)                    | SSM (SecureString)          |
| `RABBITMQ_URL`       | AMQP connection for the API                | SSM (SecureString)          |
| `RABBITMQ_USER`      | Broker account                             | SSM (String)                |
| `RABBITMQ_PASSWORD`  | Broker password                            | SSM (SecureString)          |
| `SESSION_TTL`        | Login session lifetime (Go duration)       | SSM (String)                |
| `DEMO_SEED_PASSWORD` | Password for the four seeded demo accounts | SSM (SecureString)          |
| `DEMO_MODE`          | Demo clock on/off                          | SSM (String)                |
| `DEMO_CLOCK_START`   | Demo clock start instant                   | SSM (String)                |
| `APP_ENV`            | `production` (JSON logs, strict CORS)      | SSM (String)                |
| `CORS_ORIGIN`        | The Vercel origin allowed to call the API  | SSM (String)                |
| `MEDIA_STORAGE`      | `s3` in production                         | SSM (String)                |
| `S3_BUCKET`          | Private media bucket                       | SSM (String)                |
| `AWS_REGION`         | Bucket/ECR/SSM region                      | SSM (String)                |

**Secrets never appear in the repository, Terraform, Docker images, GitHub or
the frontend.** At deploy time `infra/deploy/render-env.sh` reads
`/waypoint/prod/*` with `--with-decryption` into `/opt/waypoint/.env` (mode 0600)
and splits the broker credentials into `/opt/waypoint/rabbitmq.env`.

On the frontend, `NEXT_PUBLIC_API_BASE_URL` is the only build-time value. It is a
**public** URL, not a secret, and is set in Vercel.

### SSM parameters

Created out of band (not by Terraform, so no secret enters Terraform state) under
`/waypoint/prod`. The EC2 instance role may read only this path. See
[`infra/terraform/README.md`](../infra/terraform/README.md) for the exact
`put-parameter` shapes.

### CI/CD

[`.github/workflows/deploy.yml`](../.github/workflows/deploy.yml) runs on pushes
to `development`/`main` and on manual dispatch:

1. Assume the deploy role with **GitHub OIDC** — no AWS access keys.
2. Build `apps/api/Dockerfile` and push `waypoint-api:<sha>` (and `:latest`) to ECR.
3. Send one SSM Run Command that refreshes the compose/Caddy files, records the
   new image tag and runs `waypoint-deploy.sh` on the instance.
4. Poll `https://<ip>.sslip.io/readyz` until it is ready.

Required repository **variables** (non-secret): `AWS_REGION`, `ECR_REPOSITORY`,
`EC2_INSTANCE_ID`, `API_HEALTH_URL`, `AWS_DEPLOY_ROLE_ARN`. Their values come
from `terraform output`. No AWS secret is stored in GitHub.

### Deployment procedure (outline)

1. `terraform apply` the infrastructure in `infra/terraform` (see its README).
2. Create the `/waypoint/prod/*` SSM parameters out of band.
3. Set the GitHub variables above from `terraform output`.
4. Push to `development` (or run the workflow manually) to build, push and deploy.
5. Point Vercel at `apps/web` and set `NEXT_PUBLIC_API_BASE_URL` to
   `terraform output api_base_url`, then deploy the frontend.
6. Verify `/healthz` and `/readyz` over HTTPS and sign in to all four roles.

### Rollback

Images are SHA-tagged and the last 15 are retained. To roll back without touching
the database:

```bash
# Find a good tag in ECR, then on the instance (via SSM Session Manager):
aws ecr describe-images --repository-name waypoint-api \
  --query 'sort_by(imageDetails,&imagePushedAt)[-5:].[imageTags[0],imagePushedAt]' --output table

printf 'IMAGE_TAG=%s\n' '<previous-sha>' > /opt/waypoint/image.env
/usr/local/bin/waypoint-deploy.sh
```

Rollback never resets the database and never reruns destructive migrations.

### Health checks

| Endpoint   | Service | Meaning                                                        |
| ---------- | ------- | -------------------------------------------------------------- |
| `/healthz` | api     | Liveness only — does **not** touch the database                |
| `/readyz`  | api     | Readiness — pings PostgreSQL and RabbitMQ, names a failure 503 |
| `/`        | web     | The Next.js app responds (Vercel)                              |

Both containers also carry Docker `HEALTHCHECK` directives.

### Troubleshooting

| Symptom                              | Check                                                                                 |
| ------------------------------------ | ------------------------------------------------------------------------------------- |
| Deploy step fails in SSM             | SSM Run Command output; `journalctl -u waypoint` and `docker compose ps` on the host   |
| API 502 through Caddy                | `docker compose logs api`; confirm `/opt/waypoint/.env` rendered and RabbitMQ is up    |
| Certificate not issued               | Port 80 reachable from the internet; Caddy logs (`docker compose logs caddy`)          |
| Wrong CORS origin                    | `CORS_ORIGIN` SSM value equals the exact Vercel origin (scheme + host, no trailing /)  |
| Media 403                            | Instance role S3 policy and `S3_BUCKET`/`AWS_REGION`; bucket remains private           |

### Security model

- No SSH: access via **SSM Session Manager**.
- Public ingress is 80/443 only; the API and RabbitMQ have no published ports.
- EC2 runtime uses a **least-privilege instance role** (SSM read on
  `/waypoint/prod`, ECR pull, one S3 bucket, Session Manager). No administrator.
- GitHub uses **OIDC**; no long-lived AWS keys are created or stored.
- S3 stays private (public access blocked, AES256); clients receive short-lived
  presigned URLs after server-side authorisation.
- Demo controls (`/demo/*`) are mounted only when `DEMO_MODE=true`.

### Cost

Roughly **$8–18 / month** while running: EC2 `t3.small` (~$15), 20 GiB gp3
(~$1.6), Elastic IP (free while attached), ECR/S3/SSM (cents). No NAT gateway,
ALB, RDS, ECS or EKS. Terminate the instance after the review window to stop the
compute charge.

### Fallback procedure

If the AWS deployment is unavailable, run the stack locally with Docker Compose
(the section above). It requires no AWS account and exercises the same code paths
with `MEDIA_STORAGE=local` and the compose Postgres.
