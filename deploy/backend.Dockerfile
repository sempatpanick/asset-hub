FROM --platform=$BUILDPLATFORM golang:1.27.3@sha256:6f1acc898d064a4e9d5773ee574a9af6a9b786306939ac14d7fd806bf62046db AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY services/backend/go.mod services/backend/go.sum ./
RUN go mod download
COPY services/backend/ ./
RUN export CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH; go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api && go build -trimpath -ldflags="-s -w" -o /out/admin ./cmd/admin && go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker
FROM gcr.io/distroless/static-debian13:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3
COPY --from=build /out/ /app/
USER 65532:65532
EXPOSE 8085
CMD ["/app/api"]
