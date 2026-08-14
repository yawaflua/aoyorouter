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

USER appuser

EXPOSE 8080
ENV env=stage

CMD ["./app"]
HEALTHCHECK --interval=30s --timeout=30s --start-period=5s --retries=3 \
    CMD curl --fail --silent --show-error \
    -H "Authorization: Password ${INITIAL_PASSWORD}" \
    "http://127.0.0.1:8080/api/aoyo/v1/healthz" || exit 1
