#!/usr/bin/env bash
# Run from the installation directory. Always isolate the AssetHub project.
set -euo pipefail
exec docker compose --project-name assethub --env-file .env --env-file deploy/images.env \
  -f deploy/compose.yaml -f deploy/compose.production.yaml "$@"
