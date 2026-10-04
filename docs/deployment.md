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

| Variable             | Purpose                                    | Local default  |
| -------------------- | ------------------------------------------ | -------------- |
| `DATABASE_URL`       | Postgres connection (Neon or compose)      | compose url    |
| `RABBITMQ_URL`       | AMQP connection                            | compose url    |
| `SESSION_TTL`        | Login session lifetime (Go duration)       | `12h`          |
| `DEMO_SEED_PASSWORD` | Password for the four seeded demo accounts | `waypoint2026` |
| `MEDIA_STORAGE`      | `local` or `s3`                            | `local`        |
| `MEDIA_ROOT`         | Filesystem root when `local`               | `/data/media`  |
| `S3_BUCKET`          | Private bucket name when `s3`              | —              |
| `AWS_REGION`         | Region of the bucket when `s3`             | —              |

**Credentials are never configured here.** The Go API uses the standard AWS SDK
default credential chain. On AWS compute it reads the attached **IAM role**; for
local development it reads an AWS CLI profile / `~/.aws/credentials` (environment
variables are a last resort). No access key is committed to the repository,
`.env.example`, docs or Dockerfiles.

### Media storage (S3)

The hosted deployment stores media in a **private** S3 bucket.

| Setting         | Value                                                  |
| --------------- | ------------------------------------------------------ |
| Bucket          | `waypoint-project-media-985096928297-us-east-1-an`     |
| Region          | `us-east-1`                                            |
| `MEDIA_STORAGE` | `s3`                                                   |
| `S3_BUCKET`     | `waypoint-project-media-985096928297-us-east-1-an`     |
| `AWS_REGION`    | `us-east-1`                                            |
| IAM role policy | `WaypointMediaS3Access`                                |

The bucket is never public. The API authorises the caller (role + depot/outlet
scope) and mints **short-lived presigned URLs** for upload and download; unsigned
access is denied. Confirmed live: upload intent `201`, presigned `PUT` `200`,
authorised `GET` returns the exact bytes, unsigned/public access `403`.

### IAM — credential chain and role architecture

Production does **not** use a long-lived access key. Credentials come from the
AWS SDK default provider chain, so the compute resource holds an IAM role:

```
EC2 instance (the project's production compute)
        │  instance profile
        ▼
IAM Role  (e.g. WaypointEC2MediaRole)
        │  attached policy: WaypointMediaS3Access
        ▼
Private S3 bucket  waypoint-project-media-985096928297-us-east-1-an
```

`WaypointMediaS3Access` is least-privilege and scoped to this one bucket:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "BucketLevelList",
      "Effect": "Allow",
      "Action": ["s3:ListBucket"],
      "Resource": "arn:aws:s3:::waypoint-project-media-985096928297-us-east-1-an"
    },
    {
      "Sid": "ObjectLevelAccess",
      "Effect": "Allow",
      "Action": ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"],
      "Resource": "arn:aws:s3:::waypoint-project-media-985096928297-us-east-1-an/*"
    }
  ]
}
```

Do **not** substitute `AmazonS3FullAccess`. The instance/task role is the only
identity with S3 access; no access key is placed in the production environment.

### Local development credentials

Prefer the standard chain over environment variables:

```bash
aws configure --profile waypoint        # writes ~/.aws/credentials, never tracked
export AWS_PROFILE=waypoint             # or AWS_REGION=us-east-1 in your shell
```

Then set `MEDIA_STORAGE=s3`, `S3_BUCKET` and `AWS_REGION` in the git-ignored
`.env`, leaving `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` empty. Do not commit
`.env`.

### Credential rotation

Because a development access key was shared outside the repository, treat it as
compromised and rotate it. There is no committed secret to change. In the AWS
Console:

1. **IAM → Users →** the local development user **→ Security credentials**.
2. Under **Access keys**, select the exposed key and **Deactivate**, then
   **Delete**. (Deactivating first lets you confirm nothing still depends on it.)
3. **Create access key** → *Application running outside AWS* → copy the new key
   once.
4. Configure it locally via an AWS CLI profile (preferred) or a git-ignored
   `.env`; never paste it into the repo.
5. Verify: `aws s3 ls s3://waypoint-project-media-985096928297-us-east-1-an --profile waypoint`.

For production, prefer attaching the IAM role (above) so no local key is needed
at all; if the production user's key was also exposed, rotate it the same way and
move production to the role.

### Deployment procedure (outline)

1. Provision an EC2 instance and an Elastic IP.
2. Create/confirm the private S3 bucket (the `securing-s3-buckets` skill covers the hardening).
3. Create an IAM role with `WaypointMediaS3Access` and attach it as the instance profile.
4. Point `DATABASE_URL` at Neon, set `MEDIA_STORAGE=s3`, `S3_BUCKET`, `AWS_REGION` (leave access-key vars empty).
5. Install Nginx; proxy `/` to the web app and `/api/` to the Go API.
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
