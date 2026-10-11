#!/usr/bin/env bash
set -euo pipefail
mkdir -p .local /evidence
cleanup() {
  task_status=$?
  cp -a .local test-results /evidence/ 2> /dev/null || true
  exit "$task_status"
}
trap cleanup EXIT
# Compare generated content consistently across Windows and Linux line endings.
node scripts/ci/check-generated-api.mjs --snapshot .local/generated-api.json
pnpm run generate:api
node scripts/ci/check-generated-api.mjs --check .local/generated-api.json
pnpm run format:check
pnpm exec prettier --check scripts/local-deploy.mjs scripts/ci/check-generated-api.mjs tests/deploy/local-deploy.test.mjs
pnpm run typecheck
pnpm run lint
pnpm exec eslint scripts/local-deploy.mjs scripts/ci/check-generated-api.mjs --max-warnings 0
pnpm run test:deploy
actionlint
shfmt -d -i 2 -sr deploy/image-tags.sh deploy/compose-release.sh scripts/ci/*.sh
shellcheck deploy/image-tags.sh deploy/compose-release.sh scripts/ci/*.sh
pnpm run build
(
  cd services/backend
  task_unformatted=$(gofmt -l .)
  [[ -z "$task_unformatted" ]] || {
    printf '%s\n' "$task_unformatted" >&2
    exit 1
  }
  go build ./...
  go vet ./...
  go test -race -count=1 -json ./... > ../../.local/backend-tests.json
  # Missing persistent fixtures must never turn into a passing release gate.
  node -e 'const fs=require("fs"); const events=fs.readFileSync("../../.local/backend-tests.json","utf8").trim().split("\n").map(JSON.parse); if(events.some(e=>e.Action==="skip" && e.Test)) process.exit(1)'
  for task_binary in api admin worker; do
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o "../../.local/$task_binary-arm64" "./cmd/$task_binary"
  done
)
bash scripts/ci/browser-test.sh --grep-invert 'backups disabled'
BACKUP_ENABLED=false bash scripts/ci/browser-test.sh --grep 'backups disabled'
pnpm audit --prod --audit-level=high
printf 'All local release checks passed.\n'
