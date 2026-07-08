# CLAUDE.md

## Project

**personal-assistant** — A personal assistant application.

## Repository

- **Repo:** https://github.com/irfanmaulana007/personal-assistant
- **Main branch:** `main`

## Development

- **Server:** Go 1.25+, SQLite (CGO required) — code in `server/`
- **Client:** TypeScript, React, Vite, Tailwind — code in `client/`
- **Build all:** `make build`
- **Build server:** `make build-server`
- **Build client:** `make build-client`
- **Run:** `make run`
- **Dev server (hot reload):** `make dev-server`
- **Dev client:** `make dev-client`
- **Test:** `make test`
- **Lint:** `make lint`
- **Dependencies:** `make tidy` (Go), `cd client && npm install` (JS)

## Conventions

- Write clear, concise commit messages
- Keep PRs focused and small

## Pull requests

- Always open a PR with a detailed description and merge it to `main`.
- Every PR must have at least one label describing its type. Use one of:
  `feature`, `fix`, `docs`, `improvement`, `refactor`, `chore`
  (create the label with `gh label create` if it does not exist yet).
- Apply the label with `gh pr edit <n> --add-label <type>`.
