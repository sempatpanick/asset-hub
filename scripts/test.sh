#!/bin/sh
set -eu
npm ci
npm run generate:api
npm run typecheck
npm run build
npm audit --audit-level=high
mkdir -p .local
cd services/backend
go test ./...
go vet ./...
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o ../../.local/assethub-api-arm64 ./cmd/api
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o ../../.local/assethub-admin-arm64 ./cmd/admin
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o ../../.local/assethub-worker-arm64 ./cmd/worker
