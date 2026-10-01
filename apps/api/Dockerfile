# syntax=docker/dockerfile:1
#
# Waypoint Go API.
# Build context is the WORKSPACE ROOT, not apps/api — see docker-compose.yml.
#
#   docker build -f apps/api/Dockerfile -t waypoint-api .

# ---------------------------------------------------------------------------
# Build
# ---------------------------------------------------------------------------
FROM golang:1.24-alpine AS builder

WORKDIR /src

# Dependencies first so a source-only change does not re-download the module
# cache. go.sum does not exist until the first third-party dependency is added,
# hence the glob.
COPY apps/api/go.mod apps/api/go.sum* ./
RUN go mod download

COPY apps/api/ ./

# CGO off gives a fully static binary that runs in a scratch-like image.
# -trimpath keeps build paths out of the binary; -s -w drop the symbol table.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/waypoint-api ./cmd/api

# ---------------------------------------------------------------------------
# Runtime
# ---------------------------------------------------------------------------
# Alpine rather than distroless: during a 3-day build you will want to shell in
# and look around. The timezone database is embedded in the binary via
# `_ "time/tzdata"`, so no tzdata package is needed here.
FROM alpine:3.21

# wget for the healthcheck; ca-certificates for any outbound HTTPS later.
RUN apk add --no-cache ca-certificates wget \
    && addgroup -S waypoint \
    && adduser -S -G waypoint waypoint

WORKDIR /app
COPY --from=builder /out/waypoint-api /app/waypoint-api

ENV APP_ENV=production \
    PORT=8080 \
    TZ=Asia/Colombo

EXPOSE 8080

# Never run as root: a container escape should not land on a root shell.
USER waypoint

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=5 \
    CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null 2>&1 || exit 1

ENTRYPOINT ["/app/waypoint-api"]
