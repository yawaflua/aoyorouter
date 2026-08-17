#!/bin/sh
set -eu

if [ "${RUN_MIGRATIONS:-true}" = "true" ]; then
    goose \
        -dir /app/migrations \
        postgres \
        "host=${POSTGRES_HOST:-127.0.0.1} port=${POSTGRES_PORT:-5432} user=${POSTGRES_USER:-postgres} password=${POSTGRES_PASSWORD:-postgres} dbname=${POSTGRES_DB:-postgres} sslmode=${POSTGRES_SSL:-disable}" \
        up
fi

exec "$@"
