# personal-assistant

A personal assistant with an LLM tool-calling agent, a web client, and optional
WhatsApp transport. It can manage your calendar, email, reminders, and notes.

## Stack

An **npm-workspaces monorepo** heading toward three independently-deployed
services (Backend, Web, and — later — a React Native Mobile app):

- **Server (Backend):** Go 1.25+, PostgreSQL (main data + WhatsApp/whatsmeow
  session) + MongoDB (logs). CGO-free — SQLite has been fully removed — `app/api/`
- **Client (Web):** TypeScript, React, Vite, Tailwind — `app/web/`
- **Shared package:** `@personal-assistant/shared` — TypeScript types,
  framework-agnostic utils, and the platform-agnostic API client, shared by the
  web app (and the future mobile app) — `packages/shared/`

Each service builds and deploys on its own path-filtered pipeline — see
[`docs/deployment/README.md`](docs/deployment/README.md).

## Quick start

```bash
# 1. Start local databases (PostgreSQL + MongoDB) — the app requires both.
docker run -d --name pa-pg    -e POSTGRES_USER=postgres -e POSTGRES_PASSWORD=postgres \
  -e POSTGRES_DB=personal_assistant -p 5432:5432 postgres:17
docker run -d --name pa-mongo -p 27017:27017 mongo:7

# 2. Configure the server (see app/api/config/config.example.yaml)
cp app/api/config/config.example.yaml app/api/config/config.yaml
#    - set a WEB_PASSWORD and ENCRYPTION_KEY (openssl rand -base64 32)
#    - set the database connection (postgres_dsn / mongo_uri / mongo_db) —
#      see the local-dev example in config.example.yaml

# 3. Install client deps
make deps

# 4. Run
make dev-server        # Go server on :8090 (hot reload via air)
make dev-client        # Vite dev client on :5273 (proxies /api to :8090)
```

Then open the client, log in with your web password, and **before chatting**,
open **Settings** and add your LLM API key — see
[docs/setup/llm-agent.md](docs/setup/llm-agent.md).

For a single production-style run: `make run` (serves the built client from the
Go server on `:8090`).

## Make targets

| Target                       | What it does                              |
| ---------------------------- | ----------------------------------------- |
| `make build`                 | Build server + client                     |
| `make run`                   | Build and run the server                  |
| `make dev-server`            | Server with hot reload (air)              |
| `make dev-client`            | Vite dev client (port 5273)               |
| `make test` / `make lint`    | Server tests / lint                       |
| `make deps` / `make tidy`    | Install workspace deps / tidy Go modules  |
| `make lint-client`           | Lint the web workspace (eslint)           |
| `make typecheck-shared`      | Type-check `@personal-assistant/shared`   |
| `make docker-build-backend`  | Build the backend-only API image          |
| `make docker-build-web`      | Build the web-only static (nginx) image   |

`make deps` runs a single workspace install at the repo root, which links the
shared package into the client. Component-specific targets live in
`app/api/Makefile` and `app/web/Makefile`; deployment lives in
[`docs/deployment/`](docs/deployment/README.md).

## Docs

- [LLM agent setup](docs/setup/llm-agent.md) — configure the assistant's LLM.
- More under [`docs/`](docs/).
