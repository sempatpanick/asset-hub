FROM --platform=$BUILDPLATFORM node:26.13.1-alpine3.24@sha256:6bd9ab43c0ec0190ca7b2d193906cb8b7edf6e7f4afa43ddeccd6452710bffb5 AS build
WORKDIR /src
# Bootstrap the pinned package manager; all project installs use pnpm.
RUN npm install --global pnpm@12.21.0
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/web/package.json ./apps/web/package.json
COPY packages/api-client/package.json ./packages/api-client/package.json
RUN pnpm install --frozen-lockfile
COPY apps/web/ ./apps/web/
COPY packages/api-client/ ./packages/api-client/
ENV NEXT_TELEMETRY_DISABLED=1
RUN pnpm run build
FROM nginx:1.30.7-alpine@sha256:caf4e1e032a20f9c3c5e28fc7f25ffd768ad8677b4975c54193075997b3a97e3 AS runtime
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
