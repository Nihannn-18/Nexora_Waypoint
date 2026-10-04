#!/bin/bash
#
# Deploy (or restart) the Waypoint production stack on the instance.
#
# Called by waypoint.service at boot and by the GitHub Actions workflow over SSM
# Run Command after it has updated /opt/waypoint/image.env with the new SHA tag.
# The repository is never cloned onto the instance: the image comes from ECR and
# configuration from SSM.

set -euo pipefail

CONFIG=/opt/waypoint/config.env
IMAGE_ENV=/opt/waypoint/image.env
COMPOSE=/opt/waypoint/docker-compose.prod.yml

# Export so the docker compose child process can interpolate ${ECR_REPO} and
# ${IMAGE_TAG} in the compose file, not just read them in this shell.
set -a
# shellcheck disable=SC1091
. "$CONFIG"
if [ -f "$IMAGE_ENV" ]; then
  # shellcheck disable=SC1090
  . "$IMAGE_ENV"
fi
set +a

/usr/local/bin/waypoint-render-env.sh

if [ "${IMAGE_TAG:-pending}" = "pending" ]; then
  echo "No API image published yet; starting RabbitMQ and Caddy only."
  docker compose --env-file "$CONFIG" -f "$COMPOSE" up -d rabbitmq caddy
  exit 0
fi

REGISTRY="${ECR_REPO%%/*}"
aws ecr get-login-password --region "$AWS_REGION" |
  docker login --username AWS --password-stdin "$REGISTRY"

docker compose --env-file "$CONFIG" -f "$COMPOSE" up -d --remove-orphans

# Drop images no longer referenced by the running tag.
docker image prune -f >/dev/null 2>&1 || true

echo "Deployed ${ECR_REPO}:${IMAGE_TAG}"
