#!/bin/sh
set -eu
pnpm install --frozen-lockfile
pnpm run generate:api
pnpm run format:check
pnpm run typecheck
pnpm run lint
pnpm run test:deploy
pnpm run build
pnpm run test:e2e
mkdir -p .local
cd services/backend
task_unformatted=$(gofmt -l .)
if [ -n "$task_unformatted" ]; then
  printf '%s\n' "$task_unformatted" >&2
  exit 1
fi
go build ./...
go test ./...
go vet ./...
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o ../../.local/assethub-api-arm64 ./cmd/api
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o ../../.local/assethub-admin-arm64 ./cmd/admin
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o ../../.local/assethub-worker-arm64 ./cmd/worker
cd ../..
pnpm audit --audit-level=high
