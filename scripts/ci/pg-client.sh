#!/usr/bin/env bash
# Invoke through pg_dump/pg_restore symlinks; use the server's pinned client.
set -euo pipefail
task_tool=$(basename -- "$0")
case "$task_tool" in
pg_dump | pg_restore) ;;
*)
  echo 'Invoke this helper as pg_dump or pg_restore.' >&2
  exit 1
  ;;
esac
task_pass=${PGPASSFILE:?The snapshot test must supply its restricted pgpass file}
task_root=$(dirname -- "$task_pass")
exec docker run --rm --network host --user "$(id -u):$(id -g)" \
  --env PGPASSFILE="$task_pass" \
  --mount "type=bind,source=$task_root,target=$task_root" \
  "${TEST_POSTGRES_IMAGE:?Set the pinned PostgreSQL image}" "$task_tool" "$@"
