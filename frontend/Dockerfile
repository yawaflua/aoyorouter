FROM oven/bun:1-alpine AS build

WORKDIR /app

COPY package.json bun.lock ./
RUN bun install --frozen-lockfile

COPY index.html svelte.config.js vite.config.ts tsconfig*.json ./
COPY public ./public
COPY src ./src

ARG VITE_API_BASE_URL=""
ENV VITE_API_BASE_URL=${VITE_API_BASE_URL}

RUN bun run build

FROM busybox:1.37-musl

COPY --from=build --chown=65532:65532 /app/dist /www

USER 65532:65532
EXPOSE 8080

CMD ["httpd", "-f", "-p", "8080", "-h", "/www"]
