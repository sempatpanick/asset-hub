#!/usr/bin/env bash
set -euo pipefail
mkdir -p .local /evidence
cleanup() {
  task_status=$?
  cp -a .local test-results /evidence/ 2> /dev/null || true
  exit "$task_status"
}
trap cleanup EXIT
# Check generated artifacts against their committed bytes without modifying the checkout.
sha256sum packages/api-client/src/schema.d.ts apps/web/public/api-docs/openapi.{json,yaml} > .local/generated.sha256
pnpm run generate:api
sha256sum --check .local/generated.sha256
pnpm run format:check
pnpm exec prettier --check scripts/local-deploy.mjs tests/deploy/local-deploy.test.mjs
pnpm run typecheck
pnpm run lint
pnpm exec eslint scripts/local-deploy.mjs --max-warnings 0
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
