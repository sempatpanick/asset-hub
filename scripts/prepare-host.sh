#!/usr/bin/env bash
set -euo pipefail
# Run once as root after mounting the independent backup filesystem.
task_data=${1:-/srv/assethub}
task_backup=${2:-/mnt/assethub-backup}
[[ $(id -u) = 0 ]] || { echo 'Run as root.' >&2; exit 1; }
[[ "$task_data" = /* && "$task_data" != / && "$task_backup" = /* && "$task_backup" != / ]] || { echo 'Absolute non-root paths required.' >&2; exit 1; }
mountpoint -q -- "$task_backup" || { echo 'Mount the independent backup filesystem first.' >&2; exit 1; }
install -d -m 0755 -- "$task_data"
[[ $(stat -c %d -- "$task_data") != $(stat -c %d -- "$task_backup") ]] || { echo 'Staging and backup must use different filesystems.' >&2; exit 1; }
for task_dir in staging cache local; do
  install -d -o 65532 -g 65532 -m 0700 -- "$task_data/$task_dir"
done
chmod 0755 -- "$task_backup"
install -d -o 65532 -g 65532 -m 0700 -- "$task_backup/assets"
install -d -o 0 -g 0 -m 0700 -- "$task_backup/database"
if [[ ! -e "$task_backup/.assethub-backup" ]]; then
  printf 'assethub-backup-v1\n' > "$task_backup/.assethub-backup"
  chmod 0644 -- "$task_backup/.assethub-backup"
fi
echo 'Asset directories prepared. Keep encryption/signing keys offline and separate.'
