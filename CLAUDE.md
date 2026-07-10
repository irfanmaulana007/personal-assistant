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

## Frontend stack

- **CSS:** Tailwind (utility classes; no CSS modules).
- **Components:** Radix UI primitives (e.g. Popover) for interactive components.
- **Charts:** Recharts for all data visualizations.
- **Layout:** page content uses the full available width — do not constrain it
  with `max-w-*` wrappers.
- **Theming:** always style UI for both light and dark theme. Dark mode is
  enabled via a `.dark` class on `<html>` (see `src/lib/theme.ts`), so every
  color utility that sets a background, border, or text color must have a
  matching `dark:` variant. Follow the existing palette: page background
  `bg-gray-100 dark:bg-gray-900`, cards `bg-white dark:bg-gray-800`, borders
  `border-gray-200 dark:border-gray-700`, headings `text-gray-900
  dark:text-gray-50`, secondary text `text-gray-500 dark:text-gray-400`.

## Pull requests

- Always open a PR with a detailed description and merge it to `main`.
- Every PR must have at least one label describing its type. Use one of:
  `feature`, `fix`, `docs`, `improvement`, `refactor`, `chore`
  (create the label with `gh label create` if it does not exist yet).
- Apply the label with `gh pr edit <n> --add-label <type>`.
