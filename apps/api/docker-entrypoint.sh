#!/bin/sh
# Runs as root, fixes ownership of the mounted media volume, then drops to the
# unprivileged waypoint user before starting the API.
#
# A Docker named volume is created root-owned and masks any ownership baked into
# the image, so the app user could not write uploads. This chown is the smallest
# fix; the process itself never runs as root.
set -eu

MEDIA_ROOT="${MEDIA_ROOT:-/data/media}"

if [ "$(id -u)" = "0" ]; then
    mkdir -p "$MEDIA_ROOT"
    chown -R waypoint:waypoint "$MEDIA_ROOT"
    exec su-exec waypoint "$@"
fi

exec "$@"
