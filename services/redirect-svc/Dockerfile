# syntax=docker/dockerfile:1.7
# Build context: repository root (shared pkg/ lives there).
#   docker buildx build -f services/redirect-svc/Dockerfile --platform linux/arm64,linux/amd64 .

# Compile natively on the build host and cross-compile for the target; no QEMU.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY pkg ./pkg
COPY services/redirect-svc ./services/redirect-svc
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath \
      -ldflags "-s -w -X github.com/prasadgujar22/shorty/pkg/buildinfo.Version=${VERSION} -X github.com/prasadgujar22/shorty/pkg/buildinfo.Commit=${COMMIT}" \
      -o /out/redirect-svc ./services/redirect-svc

# Distroless static: no shell, no package manager, runs as uid 65532.
FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.source="https://github.com/prasadgujar22/shorty" \
      org.opencontainers.image.description="Shorty redirect-svc" \
      org.opencontainers.image.licenses="MIT"
COPY --from=build /out/redirect-svc /redirect-svc
USER 65532:65532
EXPOSE 8080 9090
ENTRYPOINT ["/redirect-svc"]
