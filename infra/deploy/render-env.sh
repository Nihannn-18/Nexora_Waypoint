#!/bin/bash
#
# Render the instance's runtime configuration from SSM Parameter Store.
#
# Every parameter under SSM_PARAMETER_PATH is written to
#   /opt/waypoint/.env          — passed to the API container via env_file
# and the RabbitMQ credentials are split out into
#   /opt/waypoint/rabbitmq.env  — passed only to the broker container
#
# Values are read with --with-decryption and written straight to files with mode
# 0600. Nothing is echoed: keys are written, values are not printed. The
# instance role may read only this one path.

set -euo pipefail

# shellcheck disable=SC1091
. /opt/waypoint/config.env

APP_ENV_FILE=/opt/waypoint/.env
BROKER_ENV_FILE=/opt/waypoint/rabbitmq.env

app_tmp="$(mktemp)"
broker_tmp="$(mktemp)"
trap 'rm -f "$app_tmp" "$broker_tmp"' EXIT

while IFS=$'\t' read -r name value; do
  [ -z "${name:-}" ] && continue
  key="${name##*/}"
  case "$key" in
    RABBITMQ_USER)
      printf 'RABBITMQ_DEFAULT_USER=%s\n' "$value" >>"$broker_tmp"
      ;;
    RABBITMQ_PASSWORD)
      printf 'RABBITMQ_DEFAULT_PASS=%s\n' "$value" >>"$broker_tmp"
      ;;
    *)
      printf '%s=%s\n' "$key" "$value" >>"$app_tmp"
      ;;
  esac
done < <(
  aws ssm get-parameters-by-path \
    --region "$AWS_REGION" \
    --path "$SSM_PARAMETER_PATH" \
    --with-decryption \
    --recursive \
    --query 'Parameters[].[Name,Value]' \
    --output text
)

chmod 0600 "$app_tmp" "$broker_tmp"
mv "$app_tmp" "$APP_ENV_FILE"
mv "$broker_tmp" "$BROKER_ENV_FILE"
