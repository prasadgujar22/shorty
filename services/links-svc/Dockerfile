# syntax=docker/dockerfile:1.7
# Build context: repository root (shared pkg/ lives there).
#   docker buildx build -f services/links-svc/Dockerfile --platform linux/arm64,linux/amd64 .

# Compile natively on the build host and cross-compile for the target; no QEMU.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY pkg ./pkg
COPY services/links-svc ./services/links-svc
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath \
      -ldflags "-s -w -X github.com/prasadgujar22/shorty/pkg/buildinfo.Version=${VERSION} -X github.com/prasadgujar22/shorty/pkg/buildinfo.Commit=${COMMIT}" \
      -o /out/links-svc ./services/links-svc

# Distroless static: no shell, no package manager, runs as uid 65532.
FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.source="https://github.com/prasadgujar22/shorty" \
      org.opencontainers.image.description="Shorty links-svc" \
      org.opencontainers.image.licenses="MIT"
COPY --from=build /out/links-svc /links-svc
USER 65532:65532
EXPOSE 8080 9090
ENTRYPOINT ["/links-svc"]
