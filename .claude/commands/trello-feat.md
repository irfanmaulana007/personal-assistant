---
description: Implement Todo cards from the "Task Management" Trello board as branches + PRs to staging. `/trello-feat` or `/trello-feat all` runs every card (one subagent per card in its own worktree); `/trello-feat <card-url-or-id>` runs one specific card inline in the current session (1 card = 1 branch = 1 PR either way)
---

Implement cards from the `Todo` list of the "Task Management" Trello board. Each
card is one feature; every card becomes **its own branch and its own pull
request into `staging`** — never batch multiple cards into one branch or PR.

**This command takes an optional argument (`$ARGUMENTS`) that selects how many
cards to run — every card, or one specific card. Read "## Argument: all cards,
or one specific card" below and pick the matching procedure before doing
anything else.**

In **all-cards mode**, each card is handled by a dedicated subagent working in
its own isolated git worktree: you (the orchestrator) read the cards and fan
them out to subagents that run **in parallel**, then collect their results.
Because every agent works in a separate worktree on a separate branch, they
never collide with each other or with the user's working copy. In
**single-card mode** you do the one card's work **inline in the current
session** instead — no subagent — so its execution stays visible in this
session.

Read Trello through the globally-configured `trello` MCP server (tools
`set_active_board`, `get_lists`, `get_cards_by_list_id`, `get_card`,
`add_comment`, `move_card`, …). If those MCP tools aren't loaded in the current
session, tell the user to restart Claude Code so the `trello` server's tools
load.

## Argument: all cards, or one specific card

This command takes an optional argument, `$ARGUMENTS`. Decide the mode from it
**before doing anything else**:

- **empty** or **`all`** → **All-cards mode.** Run **every** valid card in the
  `Todo` list, fanning out one subagent per card in parallel. Follow
  "## Procedure — all cards".
- **a single Trello card** — a card URL
  (`https://trello.com/c/<shortLink>/…`), a bare `shortLink`, or a card id →
  **Single-card mode.** Run **only that one card**, and do it **inline in the
  current session** (no subagent fan-out) so its execution is visible here.
  Extract the card's `shortLink` from the argument (the segment after `/c/` in a
  URL, or the value itself) and load it with `get_card`. Follow
  "## Procedure — single card".

Single-card mode is what lets you dedicate a **separate session to each card**:
open a new session and run `/trello-feat <card-url>` — that session works the
one card end-to-end in its own worktree, so you can watch each card's execution
independently.

## Fixed Trello targets

This command always operates on these exact ids (resolve by name via
`list_workspaces` / `list_boards` / `get_lists` if an id ever changes):

- **Workspace:** `Personal Assistant` — `6a54dd566d0f1d87fc3d9c54`
- **Board:** `Task Management` — `6a54dd8eecaab3bd510528ba`
- **Source list (read cards here):** `Todo` — `6a54dda5bd020d9d6740ade7`
- **In-progress list (move card here BEFORE you start the work):** `In Progress`
  — `6a54dda869fc7862c4139b49`. A card enters `In Progress` the moment its work
  begins — before the branch or PR exists — not after the PR is opened.
- **Test list (`Test` — `6a5c1f75a5a7d68de5e833d7`):** cards land here when their
  PR is **merged into `staging`**. The `/merge-all` command does that, **not**
  this command — after you open the PR, leave the card in `In Progress`.
- **Done list (`Done` — `6a54ddadf123f4f7f7955c95`):** cards land here when the
  work is **released to `main`**. The `/release` command does that. This command
  only ever moves cards into `In Progress`; it never moves them to `Test` or
  `Done`.

## Expected card shape

Each Todo card is a single feature and carries:

- **Title** — the feature name (card name).
- **Description** — what to build and any context.
- **Acceptance criteria** — the checklist the implementation must satisfy
  (in the description or as a Trello checklist). Treat these as the definition
  of done for that card's PR.

If a card is missing a description or acceptance criteria, do **not** guess the
feature — skip it and report it as skipped-for-missing-detail. **Do not dispatch
an agent for a skipped card.**

## Procedure — all cards

Use this when the argument is empty or `all`.

1. **Select the workspace, then the board.** `set_active_workspace` with
   `6a54dd566d0f1d87fc3d9c54` (the "Personal Assistant" workspace), then
   `set_active_board` with `6a54dd8eecaab3bd510528ba` (its "Task Management"
   board).

2. **Read every card in Todo.** `get_cards_by_list_id` for
   `6a54dda5bd020d9d6740ade7`, then `get_card` on each to pull the full
   description, checklists (acceptance criteria), attachments, **and the card's
   `shortUrl` / `url` — this link must go into the PR**. Print a numbered plan of
   the cards you're about to work through, marking any you're skipping for
   missing detail.

3. **Preflight the repo once.** `staging` is the base for every card's branch:

   ```
   git fetch origin --prune
   git rev-parse --verify origin/staging   # must exist; if not, stop and report
   ```

   You do **not** need a clean working tree here — each agent works in its own
   worktree, not this checkout — but confirm `origin/staging` resolves before
   dispatching anyone.

4. **Move every valid card to `In Progress` before doing any work.** For each
   card you are about to implement (i.e. every card that has a description and
   acceptance criteria and is not being skipped), `move_card` it into
   `In Progress` (`6a54dda869fc7862c4139b49`) **now**, before dispatching its
   agent — the card enters `In Progress` the moment its work starts, before the
   branch or PR exists. Do **not** move cards you are skipping for missing
   detail.

5. **Dispatch one subagent per valid card, in parallel.** For each card that has
   a description and acceptance criteria, launch an `Agent` with
   `isolation: "worktree"` so it gets its own git worktree. Send all the
   card-handling agents **in a single message** (multiple `Agent` tool calls in
   one turn) so they run concurrently. Give each agent everything it needs — it
   cannot see the Trello board itself:

   - The card **title**, full **description**, and **every acceptance criterion**
     (verbatim).
   - The card **`shortUrl`** (for the PR body and commit trailer).
   - The branch slug: `feat/<slug>`, where `<slug>` is a short kebab-case slug
     derived from the card title.

   Each agent's prompt must instruct it to run this full isolated cycle **inside
   its own worktree** and then **return a structured result** (branch name, PR
   number + URL, and status: `opened` / `failed-verification`):

   a. **Create its branch off fresh `staging`, inside its worktree:**

      ```
      git fetch origin
      git checkout -b feat/<slug> origin/staging
      ```

      (The worktree isolates this checkout, so no other agent or the user's tree
      is affected.)

   b. **Implement the feature** to satisfy the card's description and *every*
      acceptance-criterion. Follow all of CLAUDE.md — especially the theming
      rule (every UI change must work in **both** light and dark theme) and the
      frontend stack conventions. Keep the change focused on that one card.

   c. **Verify before opening a PR** — do not open a PR on a broken build:

      ```
      make build      # and make lint / make test as relevant to the change
      ```

      For UI changes, verify both themes per CLAUDE.md. If verification can't be
      made to pass, stop, return status `failed-verification`, and do **not**
      open a PR.

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

      Return the PR number and URL.

   The agent does **not** touch Trello — you handle the Trello updates in the
   next step once it reports back.

6. **After each agent returns, comment its PR link on the card (you, the
   orchestrator).** For every agent that reported an opened PR: **comment the PR
   link back on the card** (`add_comment`, e.g. `Implemented in PR: <url>`). The
   card is already in `In Progress` from step 4 — **leave it there.** Do **not**
   move it to `Test`; that happens when the PR is merged into `staging` (via
   `/merge-all`). Do not comment on cards whose agent reported
   `failed-verification`.

7. **Do not merge** any PR — leave them as drafts for the user to review. Never
   commit to `main`, never force-push, never target `main`.

8. **Report** a table of every card: card title → branch → PR number/URL →
   status (opened / skipped-for-missing-detail / failed-verification). The agents'
   worktrees are auto-cleaned; leave your own checkout as you found it.

## Procedure — single card

Use this when the argument is a single Trello card (URL, `shortLink`, or id). You
do **not** fan out a subagent here — you implement the one card **inline in the
current session**, isolated in its own git worktree, so its execution is visible
in this session.

1. **Select the workspace, then the board.** `set_active_workspace` with
   `6a54dd566d0f1d87fc3d9c54`, then `set_active_board` with
   `6a54dd8eecaab3bd510528ba`.

2. **Resolve the one card.** Derive its `shortLink` from the argument (the
   segment after `/c/` in a URL, or the value itself) and call `get_card` on it
   to pull the full **description**, **acceptance criteria** (a Trello checklist
   or an "Acceptance Criteria" section in the description), and the card's
   **`shortUrl`**. If the card is missing a description or acceptance criteria,
   do **not** guess — report it as `skipped-for-missing-detail` and stop without
   creating a branch or PR.

3. **Preflight the repo.** `staging` is the base for the branch:

   ```
   git fetch origin --prune
   git rev-parse --verify origin/staging   # must exist; if not, stop and report
   ```

4. **Move the card to `In Progress` before starting.** `move_card` the resolved
   card into `In Progress` (`6a54dda869fc7862c4139b49`) **now** — a card enters
   `In Progress` the moment its work begins, before the branch or PR exists.

5. **Isolate this session's work in its own git worktree, then implement.** Use
   the `EnterWorktree` tool so your work never disturbs the user's checkout (if
   the session is already running inside a worktree, you are already isolated —
   skip this). Then run the full cycle inside that worktree:

   a. **Create the branch off fresh `staging`:**

      ```
      git fetch origin
      git checkout -b feat/<slug> origin/staging
      ```

      where `<slug>` is a short kebab-case slug derived from the card title.

   b. **Implement the feature** to satisfy the card's description and *every*
      acceptance-criterion. Follow all of CLAUDE.md — especially the theming
      rule (every UI change must work in **both** light and dark theme) and the
      frontend stack conventions. Keep the change focused on this one card.

   c. **Verify before opening a PR** — do not open a PR on a broken build:

      ```
      make build      # and make lint / make test as relevant to the change
      ```

      For UI changes, verify both themes per CLAUDE.md. If verification can't be
      made to pass, stop, report status `failed-verification`, and do **not**
      open a PR.

   d. **Commit and push** with the Trello card link as a trailer:

      ```
      git add -A
      git commit -m "[trello] feat: <card title>" -m "Trello: <card shortUrl>"
      git push -u origin feat/<slug>
      ```

   e. **Open a draft PR into `staging`** (never `main`), title prefixed
      `[trello]`: `[trello] feat: <card title>`. The body must follow CLAUDE.md →
      Pull requests (What & why / Before vs. after / Why it matters / Scope &
      notes), include the acceptance criteria as a checklist, and attach the
      Trello card link on its own line (`**Trello card:** <card shortUrl>`).
      Then label it:

      ```
      gh pr create --draft --base staging \
        --title "[trello] feat: <card title>" --body "<see CLAUDE.md rules; include the Trello card link>"
      gh label create feature --description "New feature" --color 0E8A16 2>/dev/null || true
      gh pr edit <number> --add-label feature
      ```

6. **Comment the PR link on the card (only if a PR was opened).** `add_comment`
   on the card with the PR link (e.g. `Implemented in PR: <url>`). The card is
   already in `In Progress` from step 4 — **leave it there.** Do **not** move it
   to `Test`; that happens when the PR is merged into `staging` (via
   `/merge-all`). Do not comment if the status was `failed-verification`. (A card
   skipped for missing detail was never moved to `In Progress` and gets no
   comment.)

7. **Do not merge.** Leave the PR as a draft. Never commit to `main`, never
   force-push, never target `main`.

8. **Report** the one card: card title → branch → PR number/URL → status
   (opened / skipped-for-missing-detail / failed-verification).

## Rules

- **Always the Todo list of the Task Management board.** The argument only
  selects **all cards** (empty / `all`) vs **one specific card** (its URL /
  `shortLink` / id) — it never points at a different board or list.
- **1 card = 1 worktree = 1 branch = 1 PR.** Never combine cards. In all-cards
  mode that means one subagent per card; in single-card mode you run the one
  card inline in the current session's own worktree — never have one agent or
  session handle two cards.
- **All-cards mode fans out in parallel.** Dispatch all valid cards' agents in
  one turn so they run concurrently; each is isolated in its own worktree.
  **Single-card mode does not fan out** — it implements the one card inline in
  the current session (still isolated in its own worktree).
- **Card lifecycle:** a card moves to `In Progress` when its work **starts**
  (this command, before the branch/PR exists), to `Test` when its PR is **merged
  into `staging`** (`/merge-all`), and to `Done` when it is **released to `main`**
  (`/release`). This command only ever moves cards into `In Progress`.
- **Every PR:** title prefixed `[trello]`, Trello card link in the body, base
  `staging` (never `main`), opened as a draft. This command never merges.
- Skip — do not fabricate, and do not dispatch an agent for — any card missing a
  description or acceptance criteria, and report it.
- If an agent's verification fails and it can't make it pass, it returns
  `failed-verification` without opening a PR; you record it and move on to the
  other results rather than forcing a broken change through.
- Respect both light and dark theme for any UI, and follow the repo's frontend
  and PR conventions throughout.
