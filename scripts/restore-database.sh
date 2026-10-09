#!/usr/bin/env bash
set -euo pipefail
# Restore only to a NEW database. This script never overwrites a live database.
task_repo=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
task_env=${1:?Usage: restore-database.sh ENV_FILE SNAPSHOT NEW_DATABASE_NAME}
task_snapshot=${2:?Snapshot required}
task_database=${3:?New database name required}
[[ "$task_database" =~ ^assethub_restore_[a-z0-9_]+$ ]] || { echo 'Use a new name beginning assethub_restore_.' >&2; exit 1; }
[[ -f "$task_snapshot" && -f "$task_snapshot.sha256" ]] || { echo 'Snapshot/checksum missing.' >&2; exit 1; }
sha256sum --check --status "$task_snapshot.sha256"
task_compose=(docker compose --env-file "$task_env" -f "$task_repo/deploy/compose.yaml")
"${task_compose[@]}" exec -T postgres createdb --username=assethub -- "$task_database"
"${task_compose[@]}" exec -T postgres pg_restore --username=assethub --dbname="$task_database" --no-owner --no-acl --exit-on-error < "$task_snapshot"
echo "Restored to $task_database. Verify metadata and encrypted bytes in isolation before switching DATABASE_URL."
echo 'Keep the existing database and offline master/signing keys until the recovery drill passes.'
