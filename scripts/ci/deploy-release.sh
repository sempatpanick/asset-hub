#!/usr/bin/env bash
# Server-side entry point. Registry credentials arrive through stdin, never argv.
set -euo pipefail
task_release=${1:?Specify the staged release directory}
[[ $(id -u) != 0 ]] || {
  echo 'Use a dedicated non-root deployment user.' >&2
  exit 1
}
[[ "$task_release" =~ ^deploy/releases/[a-f0-9]{40}$ ]] || {
  echo 'Invalid release directory.' >&2
  exit 1
}
[[ -f .env && -r .env ]] || {
  echo 'Prepare the protected installation .env on the server first.' >&2
  exit 1
}
mkdir -p deploy
mkdir deploy/.deployment-lock 2> /dev/null || {
  echo 'Another deployment is running, or its lock needs operator review.' >&2
  exit 1
}
task_docker_config=''
cleanup() {
  if [[ -n "$task_docker_config" ]]; then rm -rf -- "$task_docker_config"; fi
  rmdir -- deploy/.deployment-lock
}
trap cleanup EXIT
if [[ ${DEPLOY_EXPECTED_RELEASE+x} ]]; then
  task_current=''
  if [[ -f deploy/current-release ]]; then task_current=$(cat deploy/current-release); fi
  [[ "$task_current" == "$DEPLOY_EXPECTED_RELEASE" ]] || {
    echo 'The active release changed while this candidate was being verified.' >&2
    exit 1
  }
fi
task_image_mode=${DEPLOY_IMAGE_MODE:-registry}
case "$task_image_mode" in registry | local) ;; *)
  echo 'Invalid image mode.' >&2
  exit 1
  ;;
esac
task_backend=''
task_web=''
while IFS='=' read -r task_key task_value; do
  if [[ "$task_image_mode" == registry ]]; then
    [[ "$task_value" =~ ^ghcr\.io/[a-z0-9._/-]+@(sha256:[a-f0-9]{64})$ ]] || {
      echo 'Release images must be GHCR digests.' >&2
      exit 1
    }
  else
    [[ "$task_value" =~ ^assethub-local/(backend|web):[a-f0-9]{40}$ && "${task_value##*:}" == "${task_release##*/}" ]] || {
      echo 'Local images must match the staged release commit.' >&2
      exit 1
    }
  fi
  case "$task_key" in
  BACKEND_IMAGE)
    [[ -z "$task_backend" ]] || exit 1
    task_backend=$task_value
    ;;
  WEB_IMAGE)
    [[ -z "$task_web" ]] || exit 1
    task_web=$task_value
    ;;
  *)
    echo 'Unexpected release variable.' >&2
    exit 1
    ;;
  esac
done < "$task_release/images.env"
[[ -n "$task_backend" && -n "$task_web" ]] || {
  echo 'Both image digests are required.' >&2
  exit 1
}
if [[ "$task_image_mode" == local ]]; then
  task_backend_id=''
  task_web_id=''
  while IFS='=' read -r task_key task_value; do
    [[ "$task_value" =~ ^sha256:[a-f0-9]{64}$ ]] || exit 1
    case "$task_key" in
    BACKEND_IMAGE_ID)
      [[ -z "$task_backend_id" ]] || exit 1
      task_backend_id=$task_value
      ;;
    WEB_IMAGE_ID)
      [[ -z "$task_web_id" ]] || exit 1
      task_web_id=$task_value
      ;;
    *) exit 1 ;;
    esac
  done < "$task_release/image-ids.env"
  [[ -n "$task_backend_id" && -n "$task_web_id" ]] || exit 1
  [[ $(docker image inspect --format '{{.Id}} {{.Os}}/{{.Architecture}}' "$task_backend") == "$task_backend_id linux/arm64" &&
  $(docker image inspect --format '{{.Id}} {{.Os}}/{{.Architecture}}' "$task_web") == "$task_web_id linux/arm64" ]] || {
    echo 'Loaded images do not match the verified ARM64 release.' >&2
    exit 1
  }
fi
task_compose=(docker compose --project-name assethub --env-file .env)
if [[ -f deploy/backup.env ]]; then task_compose+=(--env-file deploy/backup.env); fi
task_compose+=(--env-file "$task_release/images.env"
  -f "$task_release/compose.yaml" -f "$task_release/compose.production.yaml")
if [[ -n "${DEPLOY_BACKUP_ENABLED:-}" ]]; then
  case "$DEPLOY_BACKUP_ENABLED" in true | false) export BACKUP_ENABLED="$DEPLOY_BACKUP_ENABLED" ;; *)
    echo 'BACKUP_ENABLED must be true or false.' >&2
    exit 1
    ;;
  esac
fi
"${task_compose[@]}" config --quiet
task_environment=$("${task_compose[@]}" config --environment)
task_app_env=$(sed -n 's/^APP_ENV=//p' <<< "$task_environment")
task_data=$(sed -n 's/^DATA_ROOT=//p' <<< "$task_environment")
task_backup=$(sed -n 's/^BACKUP_PATH=//p' <<< "$task_environment")
task_backup_enabled=$(sed -n 's/^BACKUP_ENABLED=//p' <<< "$task_environment")
task_http_port=$(sed -n 's/^HTTP_PORT=//p' <<< "$task_environment")
task_api_port=$(sed -n 's/^API_PORT=//p' <<< "$task_environment")
task_postgres_port=$(sed -n 's/^POSTGRES_PORT=//p' <<< "$task_environment")
task_redis_port=$(sed -n 's/^REDIS_PORT=//p' <<< "$task_environment")
unset task_environment
[[ "${task_app_env:-production}" == production ]] || {
  echo 'Deployment requires APP_ENV=production.' >&2
  exit 1
}
task_data=${task_data:-/srv/assethub}
task_backup=${task_backup:-/mnt/assethub-backup}
[[ "$task_data" == /* && "$task_data" != / && "$task_backup" == /* && "$task_backup" != / ]] || exit 1
case "${task_backup_enabled:-true}" in
false)
  task_compose+=(-f "$task_release/compose.no-backup.yaml")
  echo 'Backups explicitly disabled: uploads use primary storage and temporary SSD checkpoints.'
  ;;
true)
  mountpoint -q -- "$task_backup" || {
    echo 'Independent backup mount is absent.' >&2
    exit 1
  }
  [[ -f "$task_backup/.assethub-backup" ]] || {
    echo 'Backup marker is absent.' >&2
    exit 1
  }
  task_data_check=$task_data
  while [[ ! -d "$task_data_check" ]]; do task_data_check=$(dirname -- "$task_data_check"); done
  [[ $(stat -c %d -- "$task_data_check") != $(stat -c %d -- "$task_backup") ]] || {
    echo 'Staging and backup must use separate filesystems.' >&2
    exit 1
  }
  ;;
*)
  echo 'BACKUP_ENABLED must be true or false.' >&2
  exit 1
  ;;
esac
"${task_compose[@]}" config --quiet
for task_port in "${task_http_port:-3005}" "${task_api_port:-8085}" "${task_postgres_port:-5435}" "${task_redis_port:-6385}"; do
  [[ "$task_port" =~ ^[0-9]{1,5}$ ]] && ((10#$task_port > 0 && 10#$task_port <= 65535)) || exit 1
  task_containers=$(docker ps --filter "publish=$task_port" --format '{{.Names}}|{{.Label "com.docker.compose.project"}}')
  while IFS= read -r task_container; do
    if [[ -n "$task_container" && "${task_container##*|}" != assethub ]]; then
      echo "Port $task_port is published by another application; resolve the conflict first." >&2
      exit 1
    fi
  done <<< "$task_containers"
done
if [[ "$task_image_mode" == registry ]]; then
  IFS= read -r task_token
  [[ -n "$task_token" && -n "${GHCR_USER:-}" ]] || {
    echo 'Registry credentials are required.' >&2
    exit 1
  }
  task_tmp_root=$(realpath -- "${TMPDIR:-/tmp}")
  task_docker_config=$(mktemp -d "$task_tmp_root/assethub-registry.XXXXXXXX")
  [[ $(realpath -- "$task_docker_config") == "$task_tmp_root"/assethub-registry.* ]] || exit 1
  export DOCKER_CONFIG="$task_docker_config"
  printf '%s\n' "$task_token" | docker login ghcr.io --username "$GHCR_USER" --password-stdin > /dev/null
  unset task_token
fi
# Finish all downloads and preflight checks before stopping traffic/workers.
if [[ "$task_image_mode" == registry ]]; then
  "${task_compose[@]}" pull storage-init postgres redis migrate api worker web
else
  "${task_compose[@]}" pull storage-init postgres redis
fi
"${task_compose[@]}" up --pull never --no-build --no-deps --force-recreate --exit-code-from storage-init storage-init
"${task_compose[@]}" up -d --pull never --no-build --wait --wait-timeout 180 postgres redis
# Quiesce mutation traffic before migrations. A failed migration keeps it stopped.
"${task_compose[@]}" stop --timeout 45 web api worker
"${task_compose[@]}" up --pull never --no-build --no-deps --force-recreate --exit-code-from migrate migrate
"${task_compose[@]}" up -d --pull never --no-build --wait --wait-timeout 180 api worker web
# Promote the manifest only after the services are healthy. Previous releases remain.
install -m 0644 "$task_release/compose.yaml" deploy/compose.yaml
install -m 0644 "$task_release/compose.production.yaml" deploy/compose.production.yaml
install -m 0644 "$task_release/compose.no-backup.yaml" deploy/compose.no-backup.yaml
install -m 0644 "$task_release/compose-release.sh" deploy/compose-release.sh
install -m 0600 "$task_release/images.env" deploy/images.env
printf 'BACKUP_ENABLED=%s\n' "${task_backup_enabled:-true}" > deploy/backup.env
chmod 0600 deploy/backup.env
printf '%s\n' "${task_release##*/}" > deploy/current-release
