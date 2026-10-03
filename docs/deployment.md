# Deployment & infrastructure

Two ways to run Waypoint:

- **Docker Compose** — the guaranteed reproducible fallback. Works from a fresh
  clone with no cloud account. This is the path a judge uses.
- **AWS (EC2 + S3)** — the hosted deployment target. Optional, and never a
  prerequisite for the Compose path.

> AWS and S3 are **team implementation/deployment decisions**, not requirements
> of the Challenge Booklet. The official functional requirements are unchanged by
> the choice of host or object store.

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

## Hosted — AWS

```
Elastic IP
    |
  Nginx  (TLS optional; public IP is acceptable initially)
  /              \
Next.js        Go API
                 |  \        \
             RabbitMQ  S3   Neon Postgres
```

- **EC2** hosts Nginx, the Next.js app and the Go API. An Elastic IP gives a
  stable public address; a custom domain is **not** required to start.
- **Nginx** terminates public traffic and reverse-proxies the web app and the API.
  The Go API is not exposed directly on its application port.
- **S3** stores media in a **private** bucket; the API authorises each caller and
  returns short-lived presigned URLs. The bucket is never public.
- **Neon PostgreSQL** replaces the compose Postgres.
- **RabbitMQ** stays RabbitMQ — no SQS/SNS substitution.

### Configuration (environment variables)

All AWS-specific values arrive through the environment; nothing is hard-coded.

| Variable              | Purpose                                              | Local default           |
| --------------------- | ---------------------------------------------------- | ----------------------- |
| `DATABASE_URL`        | Postgres connection (Neon or compose)                | compose url             |
| `RABBITMQ_URL`        | AMQP connection                                      | compose url             |
| `BETTER_AUTH_SECRET`  | Better Auth signing secret (web app)                 | local placeholder       |
| `BETTER_AUTH_URL`     | Public origin the web app signs in against           | `http://localhost:3000` |
| `BETTER_AUTH_API_KEY` | Optional hosted Better Auth dashboard (`dash()`) key | unset                   |
| `MEDIA_STORAGE`       | `local` or `s3`                                      | `local`                 |
| `MEDIA_ROOT`          | Filesystem root when `local`                         | `/data/media`           |
| `S3_BUCKET`           | Private bucket name when `s3`                        | —                       |
| `AWS_REGION`          | Region of the bucket when `s3`                       | —                       |

**Credentials are never configured here.** The Go API uses the standard AWS SDK
chain, so on EC2 it reads an **instance role**; locally, SSO or environment
credentials. No access key is committed to the repository, `.env`, docs or
Dockerfiles. Grant the role least-privilege S3 access (get/put/delete on the one
bucket).

### IAM (least privilege)

The instance role needs, for the media bucket only:

```
s3:PutObject
s3:GetObject
s3:DeleteObject
```

and nothing else. The bucket policy denies public access and enables default
encryption.

### Deployment procedure (outline)

1. Provision an EC2 instance and an Elastic IP.
2. Create a private S3 bucket (`securing-s3-buckets` skill covers the hardening).
3. Attach an instance role with the least-privilege policy above.
4. Point `DATABASE_URL` at Neon, set `MEDIA_STORAGE=s3`, `S3_BUCKET`, `AWS_REGION`, and set
   `BETTER_AUTH_SECRET` (a strong random value) and `BETTER_AUTH_URL` to the public origin.
5. Install Nginx. Route by prefix, keeping Better Auth and the Go API on distinct paths on the
   same public origin:
   - `/api/auth/` → the Next.js web app (Better Auth's own routes),
   - `/api/v1/` → the Go API,
   - `/` → the Next.js web app.

   Order matters: a blanket `/api/` rule pointed at the Go API would swallow Better Auth's
   `/api/auth/*` routes. The web client calls Better Auth same-origin at `/api/auth/*` and sends
   the resulting bearer token to the Go API at `/api/v1/*`. `BETTER_AUTH_TRUSTED_ORIGINS` must
   include the public origin.

6. Build and run the API and web images on the instance.

### Fallback procedure

If the AWS deployment is unavailable, run the stack locally with Docker Compose
(the section above). It requires no AWS account and exercises the same code paths
with `MEDIA_STORAGE=local` and the compose Postgres.

---

## Health checks

| Endpoint   | Service | Meaning                                                        |
| ---------- | ------- | -------------------------------------------------------------- |
| `/healthz` | api     | Liveness only — does **not** touch the database                |
| `/readyz`  | api     | Readiness — pings PostgreSQL and RabbitMQ, names a failure 503 |
| `/`        | web     | The Next.js app responds                                       |

Both services also carry Docker `HEALTHCHECK` directives, so `docker compose ps`
shows their state.
