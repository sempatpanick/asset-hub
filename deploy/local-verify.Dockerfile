FROM golang:1.27.3@sha256:6f1acc898d064a4e9d5773ee574a9af6a9b786306939ac14d7fd806bf62046db AS go
FROM node:26.12.1-bookworm@sha256:5c4012d11d9857c42713ba511fbc19645cac1706bb710d51d42b0d301dff7f88 AS node
FROM mcr.microsoft.com/playwright:v1.64.1-noble@sha256:47b510dd9230bf0b9bfc9b20102241e577ea7a99334ab4c3ca19f19770e9b8ea AS tooling
# Keep the toolchains independent of the large browser image for cache reuse.
COPY --link --from=go /usr/local/go /usr/local/go
COPY --link --from=node /usr/local/ /usr/local/
ENV PATH="/usr/local/go/bin:${PATH}" NEXT_TELEMETRY_DISABLED=1
# The snapshot recovery test needs the same major client version as PostgreSQL.
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl git build-essential shellcheck \
    && install -d /usr/share/postgresql-common/pgdg \
    && curl --fail --silent --show-error https://www.postgresql.org/media/keys/ACCC4CF8.asc -o /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc \
    && echo 'deb [signed-by=/usr/share/postgresql-common/pgdg/apt.postgresql.org.asc] https://apt.postgresql.org/pub/repos/apt noble-pgdg main' > /etc/apt/sources.list.d/pgdg.list \
    && apt-get update && apt-get install -y --no-install-recommends postgresql-client-18 \
    && rm -rf /var/lib/apt/lists/* && npm install --global pnpm@12.15.0
FROM tooling AS verify
WORKDIR /workspace
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/web/package.json ./apps/web/package.json
COPY packages/api-client/package.json ./packages/api-client/package.json
RUN pnpm install --frozen-lockfile
COPY services/backend/go.mod services/backend/go.sum ./services/backend/
RUN cd services/backend && go mod download
RUN GOBIN=/usr/local/bin go install github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 \
    && GOBIN=/usr/local/bin go install mvdan.cc/sh/v3/cmd/shfmt@v3.14.1
COPY . .
# Local deployment tooling is separate from the frozen application commit.
COPY --from=automation deploy/local-verify.Dockerfile deploy/compose.local-verify.yaml ./deploy/
COPY --from=automation scripts/local-deploy.mjs ./scripts/local-deploy.mjs
COPY --from=automation scripts/ci/check-generated-api.mjs ./scripts/ci/check-generated-api.mjs
COPY --from=automation scripts/ci/local-verify.sh scripts/ci/browser-test.sh scripts/ci/deploy-release.sh ./scripts/ci/
COPY --from=automation tests/deploy/local-deploy.test.mjs ./tests/deploy/local-deploy.test.mjs
COPY --from=automation tests/deploy/release.test.mjs ./tests/deploy/release.test.mjs
CMD ["bash", "scripts/ci/local-verify.sh"]
