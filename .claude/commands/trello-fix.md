---
description: Fix every card in the Bug list of the "Issue" Trello board, each as its own branch + PR to staging (1 card = 1 branch = 1 PR)
---

Fix **every card in the `Bug` list of the "Issue" Trello board**. Each card is
one bug; every card becomes **its own branch and its own pull request into
`staging`** — never batch multiple bugs into one branch or PR.

Read Trello through the globally-configured `trello` MCP server (tools
`set_active_board`, `get_lists`, `get_cards_by_list_id`, `get_card`,
`add_comment`, `move_card`, …). If those MCP tools aren't loaded in the current
session, tell the user to restart Claude Code so the `trello` server's tools
load.

## Fixed Trello targets

This command always operates on these exact ids (resolve by name via
`list_boards` / `get_lists` if an id ever changes):

- **Board:** `Issue` — `6a54edaae21957ab935c81f6`
- **Source list (read cards here):** `Bug` — `6a54edaae21957ab935c820f`
- **In-flight list (move card here once its PR is open):** `Progress` —
  `6a54edaae21957ab935c8210`
- Do **not** move cards to `Done` (`6a54edaae21957ab935c8211`) — the `/release`
  command does that when the fix actually ships.

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

1. **Select the board.** `set_active_board` with `6a54edaae21957ab935c81f6`
   (the "Issue" board).

2. **Read every card in Bug.** `get_cards_by_list_id` for
   `6a54edaae21957ab935c820f`, then `get_card` on each to pull the full
   description, actual result, expected result, checklists, attachments, **and
   the card's `shortUrl` / `url` — you must attach this link to the PR**. Print
   a numbered plan of the bugs you're about to work through.

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
      git checkout -b fix/<slug>
      ```

      `<slug>` is a short kebab-case slug derived from the bug title.

   b. **Reproduce, then fix.** First confirm the **actual result** on the card
      (reproduce the bug), then make the change so the behaviour becomes the
      **expected result**. Address the root cause, not the symptom. Follow all
      of CLAUDE.md — especially the theming rule (every UI change must work in
      **both** light and dark theme). Keep the change focused on that one bug.

   c. **Verify the fix before opening a PR.** Confirm the expected result now
      holds and the buggy behaviour is gone, and that checks pass — do not open
      a PR on a broken build:

      ```
      make build      # and make lint / make test as relevant to the change
      ```

      For UI changes, verify both themes per CLAUDE.md.

   d. **Commit and push.** Include the Trello card link as a trailer so
      `/release` can find it later:

      ```
      git add -A
      git commit -m "[trello] fix: <card title>" -m "Trello: <card shortUrl>"
      git push -u origin fix/<slug>
      ```

   e. **Open a draft PR into `staging`** (never `main`). The **title must be
      prefixed with `[trello]`**: `[trello] fix: <card title>`. The body must
      follow CLAUDE.md → Pull requests (What & why / Before vs. after / Why it
      matters / Scope & notes), state the **actual → expected** result
      explicitly and how the fix closes the gap, and **must attach the Trello
      card link** on its own line, e.g. `**Trello card:** <card shortUrl>`. Then
      label it:

      ```
      gh pr create --draft --base staging \
        --title "[trello] fix: <card title>" --body "<see CLAUDE.md rules; include the Trello card link>"
      gh label create fix --description "Bug fix" --color D73A4A 2>/dev/null || true
      gh pr edit <number> --add-label fix
      ```

   f. **Comment the PR link back on the card** (`add_comment`, e.g.
      `Fixed in PR: <url>`) and **move the card to `Progress`** (`move_card` →
      `6a54edaae21957ab935c8210`).

5. **Do not merge** any PR — leave them as drafts for the user to review. Never
   commit to `main`, never force-push, never target `main`.

6. **Report** a table of every card: bug title → branch → PR number/URL → status
   (opened / skipped-for-missing-detail / could-not-reproduce /
   failed-verification). Leave the repo checked out on `staging` and clean.

## Rules

- **Always the Bug list of the Issue board.** No board argument.
- **1 card = 1 branch = 1 PR.** Never combine bugs.
- **Every PR:** title prefixed `[trello]`, Trello card link in the body, base
  `staging` (never `main`), opened as a draft. This command never merges.
- Skip — do not fabricate — any card missing the actual or expected result, and
  report it.
- If a bug can't be reproduced or the fix can't be verified, stop work on that
  card, leave its branch/PR as-is (or close the PR), report it, and continue to
  the next card rather than forcing an unverified fix through.
- Respect both light and dark theme for any UI, and follow the repo's frontend
  and PR conventions throughout.
