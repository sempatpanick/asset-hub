FROM golang:1.27.2@sha256:5bc7f572bbaa98885a3a1fd9c0aa76b59e3e14e8628bfc316bbfd0c701e4818c AS go
FROM node:26.11.1-bookworm@sha256:f1233c415b41ffcf237c717b3dea92e9d4ea006e0e4c71edcb790605d74f5404 AS node
FROM mcr.microsoft.com/playwright:v1.64.0-noble@sha256:06a9939e57531807f8d5fd76ce44b53165ffb7d7501d87ab10e285c20b1e971f AS tooling
COPY --from=go /usr/local/go /usr/local/go
COPY --from=node /usr/local/ /usr/local/
ENV PATH="/usr/local/go/bin:${PATH}" NEXT_TELEMETRY_DISABLED=1
# The snapshot recovery test needs the same major client version as PostgreSQL.
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl git build-essential shellcheck \
    && install -d /usr/share/postgresql-common/pgdg \
    && curl --fail --silent --show-error https://www.postgresql.org/media/keys/ACCC4CF8.asc -o /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc \
    && echo 'deb [signed-by=/usr/share/postgresql-common/pgdg/apt.postgresql.org.asc] https://apt.postgresql.org/pub/repos/apt noble-pgdg main' > /etc/apt/sources.list.d/pgdg.list \
    && apt-get update && apt-get install -y --no-install-recommends postgresql-client-18 \
    && rm -rf /var/lib/apt/lists/* && npm install --global pnpm@12.10.1
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
COPY --from=automation scripts/ci/local-verify.sh scripts/ci/deploy-release.sh ./scripts/ci/
COPY --from=automation tests/deploy/local-deploy.test.mjs ./tests/deploy/local-deploy.test.mjs
COPY --from=automation tests/deploy/release.test.mjs ./tests/deploy/release.test.mjs
CMD ["bash", "scripts/ci/local-verify.sh"]
