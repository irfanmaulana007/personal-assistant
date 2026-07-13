---
description: Pull bug cards from a Trello "Bug" list and ship a fix for each one as its own branch + PR to staging (1 card = 1 branch = 1 PR)
argument-hint: [board name or id] (default — ask which board)
---

Fix the bug cards sitting in a Trello **Bug** list. Each card describes one
bug; every card becomes **its own branch and its own pull request into
`staging`** — never batch multiple bugs into one branch or PR.

This command reads Trello through the globally-configured `trello` MCP server
(tools named `list_boards`, `set_active_board`, `get_lists`,
`get_cards_by_list_id`, `get_card`, `add_comment`, `move_card`, …). If those
MCP tools are not loaded in the current session, tell the user to restart Claude
Code so the `trello` server's tools load, and fall back to the Trello REST API
(`https://api.trello.com/1/…`) using the `TRELLO_API_KEY` / `TRELLO_TOKEN` from
the server config for read-only steps only.

The optional argument `$ARGUMENTS` names the board to work on.

## Expected card shape

Each Bug card is a single bug and carries:

- **Title** — a short name for the bug (card name).
- **Description** — context: where it happens, how to reproduce, scope.
- **Actual result** — the buggy behaviour observed today.
- **Expected result** — the behaviour it *should* have.

The fix is done when the **actual result** is made to match the **expected
result**. If a card is missing the actual or expected result, do **not** guess —
skip it and report it as skipped-for-missing-detail.

## Procedure

1. **Pick the board.**
   - If `$ARGUMENTS` is non-empty, call `list_boards`, resolve it to a board by
     name (case-insensitive) or id, and `set_active_board`.
   - If `$ARGUMENTS` is empty, call `list_boards` and ask the user which board
     to use. Do not assume.

2. **Find the Bug list.** Call `get_lists` for the active board and pick the
   list whose name matches `bug` / `bugs` (case-insensitive). If no such list
   exists, show the available list names and ask which one holds the bug cards.
   Note whether a downstream list like `Doing` / `In Progress` / `In Review` /
   `Fixed` / `Done` exists — you'll move finished cards there in step 8.

3. **Read every card.** Call `get_cards_by_list_id` for the Bug list, then
   `get_card` on each to pull the full description, actual result, expected
   result, checklists, and attachments. Print a numbered list of the bugs you're
   about to work through so the user can see the plan.

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
      git checkout -b fix/<slug>
      ```

      `<slug>` is a short kebab-case slug derived from the bug title.

   b. **Reproduce, then fix.** First confirm the **actual result** described on
      the card (reproduce the bug), then make the change so the behaviour
      becomes the **expected result**. Address the root cause, not just the
      symptom. Follow all of CLAUDE.md — especially the theming rule (every UI
      change must work in **both** light and dark theme). Keep the change
      focused on that one bug.

   c. **Verify the fix before opening a PR.** Confirm the expected result now
      holds and the actual (buggy) behaviour is gone, and that checks pass — do
      not open a PR on a broken build:

      ```
      make build      # and make lint / make test as relevant to the change
      ```

      For UI changes, verify both themes per CLAUDE.md.

   d. **Commit and push:**

      ```
      git add -A
      git commit -m "fix: <card title>"   # clear, concise, per CLAUDE.md
      git push -u origin fix/<slug>
      ```

   e. **Open a draft PR into `staging`** (never `main`) with a full description
      per CLAUDE.md → Pull requests (What & why / Before vs. after / Why it
      matters / Scope & notes). State the **actual → expected** result explicitly
      and how the fix closes the gap, and link the Trello card URL. Then label
      it:

      ```
      gh pr create --draft --base staging \
        --title "fix: <card title>" --body "<see CLAUDE.md rules>"
      gh label create fix --description "Bug fix" --color D73A4A 2>/dev/null || true
      gh pr edit <number> --add-label fix
      ```

6. **Do not merge** any PR — leave them as drafts for the user to review. Never
   commit to `main`, never force-push, never target `main`.

7. **Link the PR back to Trello.** For each card, `add_comment` with the PR URL
   (e.g. `Fixed in PR: <url>`) so the board tracks the work.

8. **Move the card forward (only if a suitable list exists).** If step 2 found a
   `Doing` / `In Progress` / `In Review` list, `move_card` there. Do **not**
   move a card to `Fixed` / `Done` — the PR is still open and unreviewed. If no
   such list exists, leave the card in place and note it.

9. **Report** a table of every card: bug title → branch → PR number/URL → status
   (opened / skipped-for-missing-detail / could-not-reproduce /
   failed-verification). Leave the repo checked out on `staging` and clean.

## Rules

- **1 card = 1 branch = 1 PR.** Never combine bugs.
- **Every PR targets `staging`, never `main`** (CLAUDE.md). PRs open as drafts;
  this command never merges.
- Skip — do not fabricate — any card missing the actual or expected result, and
  report it.
- If a bug can't be reproduced or the fix can't be verified, stop work on that
  card, leave its branch/PR as-is (or close the PR), report it, and continue to
  the next card rather than forcing an unverified fix through.
- Respect both light and dark theme for any UI, and follow the repo's frontend
  and PR conventions throughout.
