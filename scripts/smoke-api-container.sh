#!/usr/bin/env bash
#
# Smoke-test the API container the way a judge runs it: build it, start it with
# its dependencies through Docker Compose, and prove that the entrypoint
# executed and the HTTP health endpoints answer.
#
# This exists because a Windows checkout can carry CRLF line endings, which turn
# the entrypoint shebang into "#!/bin/sh\r"; the container then fails with
# "exec /app/docker-entrypoint.sh: no such file or directory" even though the
# file is present. A build/start/health check catches that class of regression.
#
# It uses an isolated Compose project name so it never touches a developer's
# existing `waypoint` volumes, and it always tears the stack down afterwards.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

PROJECT="${COMPOSE_PROJECT_NAME:-waypoint-smoke}"
API_PORT="${API_PORT:-8080}"
BASE_URL="http://127.0.0.1:${API_PORT}"
TIMEOUT_SECONDS="${SMOKE_TIMEOUT_SECONDS:-180}"

compose() { docker compose -p "$PROJECT" "$@"; }

cleanup() {
  compose down -v --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

# Compose requires JWT_SECRET; a fresh clone has no .env until this copies one.
if [ ! -f .env ]; then
  cp .env.example .env
fi

echo "docker compose up (project ${PROJECT})"
compose up -d --build api

echo "Waiting up to ${TIMEOUT_SECONDS}s for ${BASE_URL}/readyz ..."
deadline=$((SECONDS + TIMEOUT_SECONDS))
until curl -fsS "${BASE_URL}/readyz" >/dev/null 2>&1; do
  if [ "$SECONDS" -ge "$deadline" ]; then
    echo "ERROR: API did not become ready within ${TIMEOUT_SECONDS}s." >&2
    compose ps --all >&2 || true
    compose logs api >&2 || true
    exit 1
  fi
  sleep 3
done

echo "Liveness:"
curl -fsS "${BASE_URL}/healthz"
echo
echo "Readiness:"
curl -fsS "${BASE_URL}/readyz"
echo

# Readiness implies the entrypoint ran; assert the log line as a second signal.
if ! compose logs api 2>&1 | grep -q "waypoint api listening"; then
  echo "ERROR: API is reachable but the expected startup log line is missing." >&2
  compose logs api >&2 || true
  exit 1
fi

compose ps
echo "Smoke test passed."
