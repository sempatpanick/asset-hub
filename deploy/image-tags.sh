#!/usr/bin/env bash
# Hash committed inputs, so unchanged images are reused across releases.
set -euo pipefail
task_service=${1:?Specify backend or web}
task_paths=(.dockerignore deploy/image-tags.sh)
case "$task_service" in
backend) task_paths+=(services/backend deploy/backend.Dockerfile) ;;
web) task_paths+=(apps/web packages/api-client package.json pnpm-lock.yaml pnpm-workspace.yaml deploy/web.Dockerfile deploy/nginx.conf) ;;
*)
  echo 'Unknown image service.' >&2
  exit 1
  ;;
esac
task_digest=$(git ls-tree -r HEAD -- "${task_paths[@]}" | sha256sum)
printf 'source-%s\n' "${task_digest%% *}"
