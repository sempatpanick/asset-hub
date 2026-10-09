#!/usr/bin/env bash
set -euo pipefail
umask 077
task_repo=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
task_env=${1:-"$task_repo/.env"}
task_backup=${2:-/mnt/assethub-backup}
[[ -f "$task_env" ]] || { echo 'Environment file missing.' >&2; exit 1; }
mountpoint -q -- "$task_backup" || { echo 'Backup filesystem is not mounted.' >&2; exit 1; }
[[ -f "$task_backup/.assethub-backup" ]] || { echo 'Backup marker missing.' >&2; exit 1; }
install -d -m 0700 -- "$task_backup/database"
task_temp=$(mktemp "$task_backup/database/.snapshot-XXXXXXXX")
trap 'rm -f -- "$task_temp"' EXIT
docker compose --env-file "$task_env" -f "$task_repo/deploy/compose.yaml" exec -T postgres pg_dump --username=assethub --dbname=assethub --format=custom --no-owner --no-acl > "$task_temp"
[[ -s "$task_temp" ]] || { echo 'Empty snapshot rejected.' >&2; exit 1; }
sync -f "$task_temp"
task_dest="$task_backup/database/$(date -u +%Y%m%dT%H%M%SZ)-$(basename -- "$task_temp").dump"
mv -- "$task_temp" "$task_dest"
sha256sum -- "$task_dest" > "$task_dest.sha256"
chmod 0600 -- "$task_dest" "$task_dest.sha256"
sync -f "$task_backup/database"
echo "PostgreSQL snapshot saved: $task_dest"
echo 'Copy protected configuration and keys separately; this snapshot contains credential hashes and private metadata.'
