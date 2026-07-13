---
description: Implement every card in the Todo list of the "Task Management" Trello board, each as its own branch + PR to staging (1 card = 1 branch = 1 PR)
---

Implement **every card in the `Todo` list of the "Task Management" Trello
board**. Each card is one feature; every card becomes **its own branch and its
own pull request into `staging`** — never batch multiple cards into one branch
or PR.

Read Trello through the globally-configured `trello` MCP server (tools
`set_active_board`, `get_lists`, `get_cards_by_list_id`, `get_card`,
`add_comment`, `move_card`, …). If those MCP tools aren't loaded in the current
session, tell the user to restart Claude Code so the `trello` server's tools
load.

## Fixed Trello targets

This command always operates on these exact ids (resolve by name via
`list_boards` / `get_lists` if an id ever changes):

- **Board:** `Task Management` — `6a54dd8eecaab3bd510528ba`
- **Source list (read cards here):** `Todo` — `6a54dda5bd020d9d6740ade7`
- **In-flight list (move card here once its PR is open):** `In Progress` —
  `6a54dda869fc7862c4139b49`
- Do **not** move cards to `Done` (`6a54ddadf123f4f7f7955c95`) — the `/release`
  command does that when the work actually ships.

## Expected card shape

Each Todo card is a single feature and carries:

- **Title** — the feature name (card name).
- **Description** — what to build and any context.
- **Acceptance criteria** — the checklist the implementation must satisfy
  (in the description or as a Trello checklist). Treat these as the definition
  of done for that card's PR.

If a card is missing a description or acceptance criteria, do **not** guess the
feature — skip it and report it as skipped-for-missing-detail.

## Procedure

1. **Select the board.** `set_active_board` with `6a54dd8eecaab3bd510528ba`
   (the "Task Management" board).

2. **Read every card in Todo.** `get_cards_by_list_id` for
   `6a54dda5bd020d9d6740ade7`, then `get_card` on each to pull the full
   description, checklists (acceptance criteria), attachments, **and the card's
   `shortUrl` / `url` — you must attach this link to the PR**. Print a numbered
   plan of the cards you're about to work through.

3. **Preflight the repo once.** `staging` is the base for every card's branch:

   ```
   git fetch origin --prune
   git rev-parse --verify origin/staging   # must exist; if not, stop and report
   ```

   The working tree must be clean before starting. If it is dirty, stop and
   report — do not stash or discard the user's work.

4. **For each card, in order, do a full isolated cycle:**

   a. **Branch off fresh `staging`:**

      ```
      git checkout staging && git pull origin staging
      git checkout -b feat/<slug>
      ```

      `<slug>` is a short kebab-case slug derived from the card title.

   b. **Implement the feature** to satisfy the card's description and *every*
      acceptance-criterion. Follow all of CLAUDE.md — especially the theming
      rule (every UI change must work in **both** light and dark theme) and the
      frontend stack conventions. Keep the change focused on that one card.

   c. **Verify before opening a PR** — do not open a PR on a broken build:

      ```
      make build      # and make lint / make test as relevant to the change
      ```

      For UI changes, verify both themes per CLAUDE.md.

   d. **Commit and push.** Include the Trello card link as a trailer so
      `/release` can find it later:

      ```
      git add -A
      git commit -m "[trello] feat: <card title>" -m "Trello: <card shortUrl>"
      git push -u origin feat/<slug>
      ```

   e. **Open a draft PR into `staging`** (never `main`). The **title must be
      prefixed with `[trello]`**: `[trello] feat: <card title>`. The body must
      follow CLAUDE.md → Pull requests (What & why / Before vs. after / Why it
      matters / Scope & notes), include the acceptance criteria as a checklist,
      and **must attach the Trello card link** on its own line, e.g.
      `**Trello card:** <card shortUrl>`. Then label it:

      ```
      gh pr create --draft --base staging \
        --title "[trello] feat: <card title>" --body "<see CLAUDE.md rules; include the Trello card link>"
      gh label create feature --description "New feature" --color 0E8A16 2>/dev/null || true
      gh pr edit <number> --add-label feature
      ```

   f. **Comment the PR link back on the card** (`add_comment`, e.g.
      `Implemented in PR: <url>`) and **move the card to `In Progress`**
      (`move_card` → `6a54dda869fc7862c4139b49`).

5. **Do not merge** any PR — leave them as drafts for the user to review. Never
   commit to `main`, never force-push, never target `main`.

6. **Report** a table of every card: card title → branch → PR number/URL →
   status (opened / skipped-for-missing-detail / failed-verification). Leave the
   repo checked out on `staging` and clean.

## Rules

- **Always the Todo list of the Task Management board.** No board argument.
- **1 card = 1 branch = 1 PR.** Never combine cards.
- **Every PR:** title prefixed `[trello]`, Trello card link in the body, base
  `staging` (never `main`), opened as a draft. This command never merges.
- Skip — do not fabricate — any card missing a description or acceptance
  criteria, and report it.
- If a card's verification fails and you can't make it pass, stop work on that
  card, leave its branch/PR as-is (or close the PR), report it, and continue to
  the next card rather than forcing a broken change through.
- Respect both light and dark theme for any UI, and follow the repo's frontend
  and PR conventions throughout.
