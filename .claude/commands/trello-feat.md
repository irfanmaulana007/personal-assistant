---
description: Pull feature cards from a Trello "Todo" list and ship each one as its own branch + PR to staging (1 card = 1 branch = 1 PR)
argument-hint: [board name or id] (default — ask which board)
---

Implement the feature cards sitting in a Trello **Todo** list. Each card
describes one feature; every card becomes **its own branch and its own pull
request into `staging`** — never batch multiple cards into one branch or PR.

This command reads Trello through the globally-configured `trello` MCP server
(tools named `list_boards`, `set_active_board`, `get_lists`,
`get_cards_by_list_id`, `get_card`, `add_comment`, `move_card`, …). If those
MCP tools are not loaded in the current session, tell the user to restart Claude
Code so the `trello` server's tools load, and fall back to the Trello REST API
(`https://api.trello.com/1/…`) using the `TRELLO_API_KEY` / `TRELLO_TOKEN` from
the server config for read-only steps only.

The optional argument `$ARGUMENTS` names the board to work on.

## Expected card shape

Each Todo card is a single feature and carries:

- **Title** — the feature name (card name).
- **Description** — what to build and any context.
- **Acceptance criteria** — the checklist the implementation must satisfy
  (often in the card description or as a Trello checklist). Treat these as the
  definition of done for that card's PR.

If a card is missing a description or acceptance criteria, do **not** guess the
feature — skip it, and report it in the summary as skipped-for-missing-detail.

## Procedure

1. **Pick the board.**
   - If `$ARGUMENTS` is non-empty, call `list_boards`, resolve it to a board by
     name (case-insensitive) or id, and `set_active_board`.
   - If `$ARGUMENTS` is empty, call `list_boards` and ask the user which board
     to use. Do not assume.

2. **Find the Todo list.** Call `get_lists` for the active board and pick the
   list whose name matches `todo` / `to do` / `to-do` (case-insensitive). If no
   such list exists, show the available list names and ask which one holds the
   feature cards. Note whether a downstream list like `Doing` / `In Progress` /
   `In Review` / `Done` exists — you'll move finished cards there in step 8.

3. **Read every card.** Call `get_cards_by_list_id` for the Todo list, then
   `get_card` on each to pull the full description, checklists (acceptance
   criteria), and attachments. Print a numbered list of the cards you're about
   to work through so the user can see the plan.

4. **Preflight the repo once.** Make sure `staging` exists and is current — the
   base for every card's branch:

   ```
   git fetch origin --prune
   git rev-parse --verify origin/staging   # must exist; if not, stop and report
   ```

   The working tree must be clean before starting. If it is dirty, stop and
   report — do not stash or discard the user's work.

5. **For each card, in order, do a full isolated cycle:**

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

   c. **Verify before opening a PR.** Run the relevant checks and make them
      pass — do not open a PR on a broken build:

      ```
      make build      # and make lint / make test as relevant to the change
      ```

      For UI changes, verify both themes per CLAUDE.md.

   d. **Commit and push:**

      ```
      git add -A
      git commit -m "feat: <card title>"   # clear, concise, per CLAUDE.md
      git push -u origin feat/<slug>
      ```

   e. **Open a draft PR into `staging`** (never `main`) with a full description
      per CLAUDE.md → Pull requests (What & why / Before vs. after / Why it
      matters / Scope & notes). Include the acceptance criteria as a checklist
      and link the Trello card URL. Then label it:

      ```
      gh pr create --draft --base staging \
        --title "feat: <card title>" --body "<see CLAUDE.md rules>"
      gh label create feature --description "New feature" --color 0E8A16 2>/dev/null || true
      gh pr edit <number> --add-label feature
      ```

6. **Do not merge** any PR — leave them as drafts for the user to review. Never
   commit to `main`, never force-push, never target `main`.

7. **Link the PR back to Trello.** For each card, `add_comment` with the PR URL
   (e.g. `Implemented in PR: <url>`) so the board tracks the work.

8. **Move the card forward (only if a suitable list exists).** If step 2 found a
   `Doing` / `In Progress` / `In Review` list, `move_card` there. Do **not**
   move a card to `Done` — the PR is still open and unreviewed. If no such list
   exists, leave the card in place and note it.

9. **Report** a table of every card: card title → branch → PR number/URL →
   status (opened / skipped-for-missing-detail / failed-verification). Leave the
   repo checked out on `staging` and clean.

## Rules

- **1 card = 1 branch = 1 PR.** Never combine cards.
- **Every PR targets `staging`, never `main`** (CLAUDE.md). PRs open as drafts;
  this command never merges.
- Skip — do not fabricate — any card missing a description or acceptance
  criteria, and report it.
- If a card's verification fails and you can't make it pass, stop work on that
  card, leave its branch/PR as-is (or close the PR), report it, and continue to
  the next card rather than forcing a broken change through.
- Respect both light and dark theme for any UI, and follow the repo's frontend
  and PR conventions throughout.
