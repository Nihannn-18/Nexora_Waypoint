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

# wget for the healthcheck; ca-certificates for outbound HTTPS; su-exec so the
# entrypoint can fix the media volume's ownership and then drop privileges.
RUN apk add --no-cache ca-certificates wget su-exec \
    && addgroup -S waypoint \
    && adduser -S -G waypoint waypoint

# Local media root, owned by the app user. When Compose mounts a named volume
# here the mount is root-owned and this ownership is masked, so the entrypoint
# re-applies it at start-up before dropping to the waypoint user.
RUN mkdir -p /data/media && chown -R waypoint:waypoint /data

WORKDIR /app
COPY --from=builder /out/waypoint-api /app/waypoint-api
COPY --chown=root:root apps/api/docker-entrypoint.sh /app/docker-entrypoint.sh
# Normalise line endings in the image. A Windows checkout can carry CRLF, which
# leaves the shebang as "#!/bin/sh\r" and makes the container fail to exec the
# entrypoint ("no such file or directory") even though the file is present.
RUN sed -i 's/\r$//' /app/docker-entrypoint.sh \
    && chmod 0755 /app/docker-entrypoint.sh

ENV APP_ENV=production \
    PORT=8080 \
    TZ=Asia/Colombo

EXPOSE 8080

# The entrypoint starts as root only to fix the media volume's ownership, then
# drops to the unprivileged waypoint user for the API process itself.
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=5 \
    CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null 2>&1 || exit 1

ENTRYPOINT ["/app/docker-entrypoint.sh"]
CMD ["/app/waypoint-api"]
