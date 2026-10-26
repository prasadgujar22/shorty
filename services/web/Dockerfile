# syntax=docker/dockerfile:1.7
# Build context: repository root.
#   docker buildx build -f services/web/Dockerfile --platform linux/arm64,linux/amd64 .

# The static build is architecture-independent: run it once on the build host.
FROM --platform=$BUILDPLATFORM node:24-alpine AS build
WORKDIR /app
COPY services/web/package.json services/web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-fund
COPY services/web/ ./
RUN npm run build && node scripts/csp.mjs build > /tmp/security-headers.conf

# Non-root Nginx (uid 101), listens on 8080 (+ 9090 for health).
FROM nginxinc/nginx-unprivileged:1.31-alpine-slim
LABEL org.opencontainers.image.source="https://github.com/prasadgujar22/shorty" \
      org.opencontainers.image.description="Shorty web UI (SvelteKit static + Nginx)" \
      org.opencontainers.image.licenses="MIT"
COPY services/web/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /tmp/security-headers.conf /etc/nginx/snippets/security-headers.conf
COPY --from=build /app/build /usr/share/nginx/html
USER 101
EXPOSE 8080 9090
