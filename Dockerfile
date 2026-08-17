FROM oven/bun:1-alpine AS frontend
WORKDIR /app

COPY frontend/package.json frontend/bun.lock ./
RUN bun install --frozen-lockfile

COPY frontend/index.html frontend/svelte.config.js frontend/vite.config.ts frontend/tsconfig*.json ./
COPY frontend/public ./public
COPY frontend/src ./src

ARG VITE_API_BASE_URL=""
ENV VITE_API_BASE_URL=${VITE_API_BASE_URL}

RUN bun run build

FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
COPY --from=frontend /app/dist ./frontend/dist

ENV GOCACHE=/root/.cache/go-build
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o ./app ./cmd/app

RUN --mount=type=cache,target=/go/pkg/mod \
    GOBIN=/app/bin go install github.com/pressly/goose/v3/cmd/goose@v3.27.3

FROM alpine:latest

WORKDIR /app

RUN apk add --no-cache curl \
    && addgroup -S appgroup \
    && adduser -S appuser -G appgroup \
    && mkdir -p /app/auth /app/migrations \
    && echo "api-keys: " > /app/config.yaml \
    && chown -R appuser:appgroup /app



COPY --from=builder --chown=appuser:appgroup /app/app ./
COPY --from=builder /app/bin/goose /usr/local/bin/goose
COPY --from=builder --chown=appuser:appgroup /app/migrations ./migrations/
COPY --chmod=755 docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

USER appuser

EXPOSE 8080
ENV ENV=prod

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["./app"]
HEALTHCHECK --interval=30s --timeout=30s --start-period=15s --retries=5 \
    CMD curl --fail --silent --show-error \
    -H "Authorization: Password ${INITIAL_PASSWORD}" \
    "http://127.0.0.1:8080/api/aoyo/v1/healthz" || exit 1
