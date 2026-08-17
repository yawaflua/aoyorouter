FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

ENV GOCACHE=/root/.cache/go-build
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go build -o ./app cmd/app/main.go

    
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

FROM alpine:latest

WORKDIR /app

RUN apk add --no-cache curl \
    && addgroup -S appgroup \
    && adduser -S appuser -G appgroup \
    && mkdir -p /app/auth /app/migrations \
    && chown -R appuser:appgroup /app \
    && touch /app/config.yaml

COPY --from=builder --chown=appuser:appgroup /app/app ./
COPY --from=builder --chown=appuser:appgroup /app/migrations ./migrations/
COPY --from=frontend --chown=appuser:appgroup /app/dist ./dist

USER appuser

EXPOSE 8080
ENV ENV=prod

CMD ["./app"]
HEALTHCHECK --interval=30s --timeout=30s --start-period=5s --retries=3 \
    CMD curl --fail --silent --show-error \
    -H "Authorization: Password ${INITIAL_PASSWORD}" \
    "http://127.0.0.1:8080/api/aoyo/v1/healthz" || exit 1
