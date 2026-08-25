# AoyoRouter

AoyoRouter is a self-hosted gateway and dashboard for using multiple AI coding providers through one API. Configure OpenAI Codex, Claude Code, xAI Grok, Google Antigravity, Kimi, or a custom OpenAI-compatible provider, create an AoyoRouter API key, and use supported models from compatible coding clients.

For example, Claude Code can connect to AoyoRouter and use a model exposed by any configured provider while keeping its normal client workflow.

## Features

- One gateway for Codex, Claude Code, Grok, Antigravity, and Kimi
- OAuth and API-key provider setup
- OpenAI- and Anthropic-compatible endpoints powered by [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI)
- API-key management, quotas, usage logs, and provider quota display
- HTTP and SOCKS proxy support
- Web dashboard for administration
### Supported providers

- OpenAI Codex
- Claude Code
- xAI Grok
- Google Antigravity
- Kimi
- OpenCode (Go/Zen)
- Cline
- Cursor
- Any OpenAI-compatible provider

## Requirements

For local development:

- Go 1.26 or newer
- PostgreSQL
- Bun, npm, pnpm, or Yarn

For container deployment, Docker with Compose is enough.

## Run with Docker Compose

Clone repository and start services using prebuilt image from GHCR:

```bash
git clone https://github.com/yawaflua/aoyorouter.git
cd aoyorouter
docker compose pull aoyorouter
docker compose up
```

Container image:

```text
ghcr.io/yawaflua/aoyorouter:latest
```

To build image locally from current source instead:

```bash
docker compose up --build
```

Open <http://localhost:8080>.

To choose dashboard password, set `INITIAL_PASSWORD` before startup:

```bash
INITIAL_PASSWORD='change-me' docker compose up
```

You can also put `INITIAL_PASSWORD=change-me` in root `.env` file before running Compose.

If `INITIAL_PASSWORD` is empty, AoyoRouter generates a one-time password and prints it in application logs:

```bash
docker compose logs aoyorouter
```

PostgreSQL data persists in `psql` Docker volume.

Container runs all pending [Goose](https://github.com/pressly/goose) migrations before starting AoyoRouter. To disable automatic migration for a custom deployment, set `RUN_MIGRATIONS=false` and run migrations separately before application startup.

## Run locally

Start PostgreSQL first. One quick option is starting only database from Compose:

```bash
docker compose up -d psql
```

Copy environment template and change `POSTGRES_HOST` because application runs on host:

```bash
cp .env.example .env
```

Use settings similar to:

```dotenv
POSTGRES_USER=postgres
POSTGRES_PASSWORD=postgres
POSTGRES_DB=postgres
POSTGRES_HOST=127.0.0.1
POSTGRES_PORT=5432
POSTGRES_SSL=disable
INITIAL_PASSWORD=change-me
```

Export variables from `.env`, then run backend:

```bash
set -a
source .env
set +a
go install github.com/pressly/goose/v3/cmd/goose@v3.27.3
goose -dir ./migrations postgres "host=$POSTGRES_HOST port=$POSTGRES_PORT user=$POSTGRES_USER password=$POSTGRES_PASSWORD dbname=$POSTGRES_DB sslmode=$POSTGRES_SSL" up
go run ./cmd/app/main.go
```

Backend and proxy API listen on <http://localhost:8080>. In another terminal, run frontend development server:

```bash
cd frontend
bun install # I recommend using Bun over npm/yarn/pnpm
VITE_API_BASE_URL=http://localhost:8080 bun run dev
```

Open URL printed by Vite, normally <http://localhost:5173>.

Equivalent `npm`, `pnpm` and `yarn` commands also work.

If `INITIAL_PASSWORD` is not set, read generated password from backend logs. Generated password changes after each fresh start where variable remains empty.

## First setup

1. Sign in to dashboard using `INITIAL_PASSWORD` or generated password from logs.
2. Add one or more providers.
3. Complete OAuth flow or enter provider credentials.
4. Create AoyoRouter API key.
5. Point compatible client to AoyoRouter and use created key.

Main compatible endpoints:

```text
GET  /v1/models
POST /v1/chat/completions
POST /v1/responses
POST /v1/messages
```

Example OpenAI-compatible request:

```bash
curl http://localhost:8080/v1/chat/completions \
  -H 'Authorization: Bearer YOUR_AOYOROUTER_API_KEY' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "YOUR_MODEL",
    "messages": [{"role": "user", "content": "Hello"}]
  }'
```

Claude Code-style clients should use:

```text
ANTHROPIC_BASE_URL=http://localhost:8080
ANTHROPIC_AUTH_TOKEN=YOUR_AOYOROUTER_API_KEY
```

Exact environment variable names can vary by client version. API base URL is `http://localhost:8080`, and client API key is key created in AoyoRouter dashboard.

## Configuration

Common environment variables:

| Variable | Default | Description |
| --- | --- | --- |
| `INITIAL_PASSWORD` | generated | Dashboard and management password |
| `HTTP_HOST` | `0.0.0.0` | HTTP bind address |
| `HTTP_PORT` | `8080` | Gateway port |
| `POSTGRES_HOST` | `127.0.0.1` | PostgreSQL hostname |
| `POSTGRES_PORT` | `5432` | PostgreSQL port |
| `POSTGRES_USER` | `postgres` | PostgreSQL user |
| `POSTGRES_PASSWORD` | `postgres` | PostgreSQL password |
| `POSTGRES_DB` | `postgres` | PostgreSQL database |
| `POSTGRES_SSL` | `disable` | PostgreSQL SSL mode |
| `WARP_LIMIT` | `10` | Maximum managed WARP proxy count |
| `RUN_MIGRATIONS` | `true` in container | Run Goose migrations before container startup |
| `ILL_NOT_USE_CLOUDFLARE_REALLY_NOT_NEEDED` | `false` | Skips getting cf endpoints, improves startup speed, but proxies will not work |
| `ENABLE_EFFORT_PRESETS` | `false` | Enables effort presets for provider models(will be shown as provider/model-effort) |

Do not expose dashboard or gateway publicly with default database credentials or weak `INITIAL_PASSWORD`. Put TLS-enabled reverse proxy in front for internet-facing deployments.

## Acknowledgements

AoyoRouter was inspired by OmniRouter and uses [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) as its multi-provider proxy engine.

## License

Licensed under [GNU General Public License v3.0](LICENSE).
