FROM --platform=$BUILDPLATFORM golang:1.27.4@sha256:205c7636d74e0ffbe3d007162df7d74f859f5a8c8322dfb9f907977686c126cf AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY services/backend/go.mod services/backend/go.sum ./
RUN go mod download
COPY services/backend/ ./
RUN export CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH; go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api && go build -trimpath -ldflags="-s -w" -o /out/admin ./cmd/admin && go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker
FROM gcr.io/distroless/static-debian13:nonroot@sha256:8ce6dd87e9823c5b48dcfb2c8604714ccb351ce59607327bcc4957448da279b5
COPY --from=build /out/ /app/
USER 65532:65532
EXPOSE 8085
CMD ["/app/api"]
