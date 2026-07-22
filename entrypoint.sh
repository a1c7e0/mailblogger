#!/bin/sh
set -e

# Sync baked-in default theme into the themes volume.
# /app/default-theme/ contains the default theme as baked into the image.
# /app/themes/ is the volume mount — starts empty on first run.
# Running on every start means image upgrades push updated default theme.
# theme.json in the volume is NEVER overwritten after first run.
if [ -d /app/default-theme/default ]; then
  mkdir -p /app/themes/default
  # Preserve theme.json across updates
  if [ -f /app/themes/default/theme.json ]; then
    cp /app/themes/default/theme.json /tmp/theme.json.bak
  fi
  cp -ru /app/default-theme/default/. /app/themes/default/
  if [ -f /tmp/theme.json.bak ]; then
    mv /tmp/theme.json.bak /app/themes/default/theme.json
  fi
fi

exec "$@"
