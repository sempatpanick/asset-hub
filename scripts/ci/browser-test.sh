#!/usr/bin/env bash
set -euo pipefail
mkdir -p .local
if curl --silent --fail http://127.0.0.1:18080/healthz > /dev/null; then
  echo 'Port 18080 already serves an application; refusing to reuse it.' >&2
  exit 1
fi
task_root=$PWD
task_binary="$task_root/.local/browser-fixture"
(cd services/backend && go build -o "$task_binary" ./integration/preview)
(cd services/backend && exec "$task_binary") > .local/browser-fixture.log 2>&1 &
task_pid=$!
cleanup() {
  task_status=$?
  if kill -0 "$task_pid" 2> /dev/null; then
    curl --silent --fail --max-time 10 -X POST -H 'X-AssetHub-Fixture: stop' \
      http://127.0.0.1:18080/__fixture/stop > /dev/null || kill -TERM "$task_pid"
  fi
  wait "$task_pid" || task_status=1
  exit "$task_status"
}
trap cleanup EXIT
task_ready=false
for ((task_attempt = 0; task_attempt < 90; task_attempt++)); do
  if ! kill -0 "$task_pid" 2> /dev/null; then
    cat .local/browser-fixture.log >&2
    exit 1
  fi
  if curl --silent --fail --max-time 2 http://127.0.0.1:18080/readyz > /dev/null; then
    task_ready=true
    break
  fi
  sleep 1
done
if [[ "$task_ready" != true ]]; then
  cat .local/browser-fixture.log >&2
  echo 'Browser fixture did not become ready.' >&2
  exit 1
fi
pnpm run test:e2e "$@"
