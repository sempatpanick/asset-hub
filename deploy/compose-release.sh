#!/usr/bin/env bash
# Run from the installation directory. Always isolate the AssetHub project.
set -euo pipefail
task_compose=(docker compose --project-name assethub --env-file .env --env-file deploy/images.env)
if [[ -f deploy/backup.env ]]; then task_compose+=(--env-file deploy/backup.env); fi
task_compose+=(-f deploy/compose.yaml -f deploy/compose.production.yaml)
task_backup=$("${task_compose[@]}" config --environment | sed -n 's/^BACKUP_ENABLED=//p')
case "${task_backup:-true}" in
false) task_compose+=(-f deploy/compose.no-backup.yaml) ;;
true) ;;
*)
  echo 'BACKUP_ENABLED must be true or false.' >&2
  exit 1
  ;;
esac
exec "${task_compose[@]}" "$@"
