FROM --platform=$BUILDPLATFORM node:26.12.1-alpine3.23@sha256:bc2b334006f15c7167bbeb2f08f28c2df883f7594a0470e04454ebe7560cfe68 AS build
WORKDIR /src
# Bootstrap the pinned package manager; all project installs use pnpm.
RUN npm install --global pnpm@12.15.0
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/web/package.json ./apps/web/package.json
COPY packages/api-client/package.json ./packages/api-client/package.json
RUN pnpm install --frozen-lockfile
COPY apps/web/ ./apps/web/
COPY packages/api-client/ ./packages/api-client/
ENV NEXT_TELEMETRY_DISABLED=1
RUN pnpm run build
FROM nginx:1.30.5-alpine@sha256:0985e772fb9f729e6fa0980da05fca5d9c468e870eed43071545afa9d2e27d94 AS runtime
COPY deploy/nginx.conf /etc/nginx/nginx.conf
USER 101:101
EXPOSE 3005
CMD ["nginx", "-g", "daemon off;"]

# Release pipelines supply the exact static export exercised by browser tests.
# BuildKit skips the Node build stage when this target is selected.
FROM runtime AS release
COPY --from=web-export / /usr/share/nginx/html/

# Keep source builds as the default for ordinary Docker/Compose development.
FROM runtime AS source
COPY --from=build /src/apps/web/out/ /usr/share/nginx/html/
