FROM golang:1.27.4@sha256:205c7636d74e0ffbe3d007162df7d74f859f5a8c8322dfb9f907977686c126cf AS go
FROM node:26.13.1-bookworm@sha256:b7087bc4d973034b30854b378ba107f9fab622b47514cb7e2592e41b8bab8d49 AS node
FROM mcr.microsoft.com/playwright:v1.65.1-noble@sha256:1507cbfc550a63a6b7396791ca4f00df17beb56342d2447b3f106c306f525579 AS tooling
# Keep the toolchains independent of the large browser image for cache reuse.
COPY --link --from=go /usr/local/go /usr/local/go
COPY --link --from=node /usr/local/ /usr/local/
ENV PATH="/usr/local/go/bin:${PATH}" NEXT_TELEMETRY_DISABLED=1
# The snapshot recovery test needs the same major client version as PostgreSQL.
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl git build-essential shellcheck \
    && install -d /usr/share/postgresql-common/pgdg \
    && curl --fail --silent --show-error https://www.postgresql.org/media/keys/ACCC4CF8.asc -o /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc \
    && echo 'deb [signed-by=/usr/share/postgresql-common/pgdg/apt.postgresql.org.asc] https://apt.postgresql.org/pub/repos/apt noble-pgdg main' > /etc/apt/sources.list.d/pgdg.list \
    && apt-get update && apt-get install -y --no-install-recommends postgresql-client-19 \
    && rm -rf /var/lib/apt/lists/* && npm install --global pnpm@12.21.0
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
