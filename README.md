# personal-assistant

A personal assistant with an LLM tool-calling agent, a web client, and optional
WhatsApp transport. It can manage your calendar, email, reminders, and notes.

## Stack

- **Server:** Go 1.25+, PostgreSQL (main data) + MongoDB (logs). CGO is still
  required (the WhatsApp session and the one-time `migrate-db` ETL use SQLite) — `server/`
- **Client:** TypeScript, React, Vite, Tailwind — `client/`

## Quick start

```bash
# 1. Configure the server (see server/config/config.example.yaml)
cp server/config/config.example.yaml server/config/config.yaml
#    then set a WEB_PASSWORD and ENCRYPTION_KEY (openssl rand -base64 32)

# 2. Install client deps
make deps

# 3. Run
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
| `make deps` / `make tidy`    | Install client deps / tidy Go modules     |

Component-specific targets live in `server/Makefile` and `client/Makefile`.

## Docs

- [LLM agent setup](docs/setup/llm-agent.md) — configure the assistant's LLM.
- More under [`docs/`](docs/).
