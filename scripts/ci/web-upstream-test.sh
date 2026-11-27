#!/usr/bin/env bash
set -euo pipefail

task_network=assethub-web-dns-test-$RANDOM-$$
task_image=$task_network:runtime
task_web=$task_network-web
task_first=$task_network-first
task_second=$task_network-second
task_paths=(/healthz /readyz /v1/auth/me /v1/uploads/probe /v1/assets)

cleanup() {
  local task_status=$?
  if ((task_status != 0)); then
    docker logs "$task_web" >&2 || true
    docker logs "$task_first" >&2 || true
    docker logs "$task_second" >&2 || true
  fi
  docker rm -f "$task_web" "$task_first" "$task_second" > /dev/null 2>&1 || true
  docker network rm "$task_network" > /dev/null 2>&1 || true
  docker image rm "$task_image" > /dev/null 2>&1 || true
  exit "$task_status"
}
trap cleanup EXIT

start_api() {
  local task_container=$1
  docker run -d --name "$task_container" --network "$task_network" \
    --network-alias api --tmpfs /var/cache/nginx:mode=1777 --entrypoint sh "$task_image" -ec '
      cat > /tmp/api.conf <<EOF
pid /tmp/nginx.pid;
error_log stderr;
events {}
http {
  access_log off;
  server {
    listen 8085;
    location / { return 200 "$1:\$uri"; }
  }
}
EOF
      exec nginx -c /tmp/api.conf -g "daemon off;"
    ' sh "$task_container" > /dev/null
}

probe() {
  docker exec "$task_web" wget -q -T 2 -O - "http://127.0.0.1:3005$1"
}

verify_api() {
  local task_expected=$1 task_attempt task_path
  for ((task_attempt = 0; task_attempt < 30; task_attempt++)); do
    if [[ $(probe /healthz 2> /dev/null) == "$task_expected:/healthz" ]]; then
      for task_path in "${task_paths[@]}"; do
        [[ $(probe "$task_path") == "$task_expected:$task_path" ]]
      done
      return
    fi
    sleep 1
  done
  printf 'Web did not route /healthz to %s\n' "$task_expected" >&2
  return 1
}

docker build -f deploy/web.Dockerfile --target runtime -t "$task_image" .
docker network create "$task_network" > /dev/null
docker run -d --name "$task_web" --network "$task_network" --read-only \
  --tmpfs /tmp --tmpfs /var/cache/nginx --cap-drop ALL "$task_image" > /dev/null
for ((task_attempt = 0; task_attempt < 10; task_attempt++)); do
  if probe / > /dev/null 2>&1; then break; fi
  sleep 1
done
probe / > /dev/null

start_api "$task_first"
verify_api "$task_first"
# Bring up the replacement first so Docker must give it a different address.
start_api "$task_second"
docker rm -f "$task_first" > /dev/null
verify_api "$task_second"
printf 'Web served static content before the API started and followed the replacement API address.\n'
