# CLAUDE.md

## Project

**personal-assistant** — A personal assistant application.

## Repository

- **Repo:** https://github.com/irfanmaulana007/personal-assistant
- **Main branch:** `main`

## Development

- **Server:** Go 1.25+, PostgreSQL (main data) + MongoDB (logs); CGO still
  required (WhatsApp session + `migrate-db` ETL use SQLite) — code in `server/`
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
- **Charts:** Recharts for all data visualizations. Read chart colors from
  `useChartTheme()` (`src/lib/useChartTheme.ts`) so they track the active theme
  — never hard-code hex values in a chart.
- **Layout:** page content uses the full available width — do not constrain it
  with `max-w-*` wrappers.

## Theming

**Rule: always respect both light and dark theme when building any UI.** No
screen, component, or state may be styled for only one theme. Verify every
change in both before considering it done.

- **How it works:** the app supports `light` / `dark` / `auto` (follow the OS).
  The choice is persisted in `localStorage` under `pa-theme` and applied by
  toggling a `.dark` class on `<html>`; Tailwind's `dark:` variant keys off it.
  See `src/lib/theme.ts` (and the pre-paint script in `index.html`).
- **Every color utility needs a `dark:` variant.** Any class that sets a
  background, border, text, ring, shadow, or icon color must pair a light value
  with a matching `dark:` value. A bare `bg-white` or `text-gray-900` with no
  `dark:` counterpart is a bug.
- **Follow the existing palette** so screens stay consistent:
  - Page background: `bg-gray-100 dark:bg-gray-900`
  - Cards / panels: `bg-white dark:bg-gray-800`
  - Borders / dividers: `border-gray-200 dark:border-gray-700`
  - Headings / primary text: `text-gray-900 dark:text-gray-50`
  - Secondary / muted text: `text-gray-500 dark:text-gray-400`
  - Accent (interactive): `text-indigo-700 dark:text-indigo-400`,
    buttons `bg-indigo-600 dark:bg-indigo-500`
- **Charts and other JS-driven colors** must come from `useChartTheme()` — do
  not read `document`/`localStorage` directly or hard-code per-theme hex.
- **Check contrast in both themes** — muted text and disabled states in
  particular must stay legible against their dark background.

## Pull requests

- Always open a PR and merge it to `main`.
- **The description must give the reader the bigger context — enough to review
  without reading the diff first.** Write it for someone who has not seen the
  change. Every PR description must cover:
  - **What & why** — what the PR is for and the problem or goal behind it.
  - **Before vs. after** — how things worked (or looked) before this change
    versus after it, so the delta is explicit. Call out behaviour, UI, or
    convention changes directly.
  - **Why it matters** — the impact: who or what benefits, what it unblocks or
    prevents, and any risk if it were not done.
  - **Scope & notes** — what is intentionally *not* changed, follow-ups,
    migrations, or anything a reviewer should watch for.
  - Use headings and bullets; prefer before/after snippets, tables, or
    screenshots (both light and dark theme for UI changes) over prose.
- Every PR must have at least one label describing its type. Use one of:
  `feature`, `fix`, `docs`, `improvement`, `refactor`, `chore`
  (create the label with `gh label create` if it does not exist yet).
- Apply the label with `gh pr edit <n> --add-label <type>`.
