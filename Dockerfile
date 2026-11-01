# syntax=docker/dockerfile:1

ARG GO_VERSION=1.27.3

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION} AS build
ARG TARGETOS TARGETARCH VERSION=dev
WORKDIR /src
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=bind,source=go.mod,target=go.mod \
    --mount=type=bind,source=go.sum,target=go.sum \
    go mod download
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=bind,target=. \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -buildvcs=false \
      -ldflags="-s -w -X main.version=${VERSION}" \
      -o /out/ollametry ./cmd/ollametry

FROM gcr.io/distroless/static-debian13:nonroot
ARG VERSION=dev
LABEL org.opencontainers.image.title="ollametry" \
      org.opencontainers.image.description="Transparent Ollama proxy exporting OpenTelemetry GenAI metrics" \
      org.opencontainers.image.source="https://github.com/a1exus/ollametry" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.licenses="MIT"
COPY --from=build /out/ollametry /usr/local/bin/ollametry
ENV OLLAMETRY_LISTEN_ADDR=0.0.0.0:11435 \
    OLLAMETRY_ADMIN_ADDR=0.0.0.0:11436
EXPOSE 11435 11436
USER nonroot:nonroot
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD ["/usr/local/bin/ollametry", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/ollametry"]
