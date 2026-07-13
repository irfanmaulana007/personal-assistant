---
name: create-card
description: Create a Trello card on the "Personal Assistant" workspace — either a task on the Backlog list (Task Management board) or a bug on the Bug list (Issue board). Routes to the right board automatically, enriches the fields from the user's ask, and always writes the card in English. Use when the user wants to add/log/file a task, feature, backlog item, bug, defect, or issue (e.g. "add a task to…", "log a bug where…", "put this on my backlog", "file an issue that…").
---

# Create card

Turn a free-form user ask into a well-structured Trello card on the **Personal
Assistant** workspace. The skill decides which board the card belongs on, drafts
the required fields, and creates the card. **Cards are always written in
English**, even when the user asks in another language — translate as needed.

## Fixed IDs (this workspace)

Use these directly. Only re-list via `mcp__trello__get_lists` if a `listId`
returns an error.

| Target | boardId | listId |
| --- | --- | --- |
| **Backlog** (Task Management) | `6a54dd8eecaab3bd510528ba` | `6a54dda0cf5b49c7fb6f8b15` |
| **Bug** (Issue) | `6a54edaae21957ab935c81f6` | `6a54edaae21957ab935c820f` |

**Backlog card labels** (Task Management board) — attach exactly one:
| Label | labelId |
| --- | --- |
| Feature | `6a54dd8eecaab3bd510528d4` |
| Improvement | `6a54dd8eecaab3bd510528d7` |
| Chore | `6a54dd8eecaab3bd510528d6` |
| Refactor | `6a54dd8eecaab3bd510528d5` |

Only re-list via `mcp__trello__get_board_labels` if a `labelId` returns an error.

## Step 1 — Choose the board

Decide **task vs. bug** from the intent of the ask, not just keywords:

- **Bug list (Issue board)** — the user is reporting that something *already
  built* behaves wrong: it's broken, erroring, crashing, showing the wrong
  result, regressed, or not doing what it should. Signals: "bug", "broken",
  "error", "crash", "doesn't work", "wrong", "should show X but shows Y",
  steps-to-reproduce, an observed-vs-expected mismatch.

- **Backlog list (Task Management board)** — the user wants *new work*: a
  feature, enhancement, chore, refactor, investigation, or any to-do that
  isn't a defect in existing behavior. Signals: "add", "build", "implement",
  "create", "improve", "we should", "need to", "task", "feature".

If the ask is genuinely ambiguous (could be either), ask the user one short
clarifying question before creating the card. Otherwise pick the best fit and
state which board you chose.

## Step 2 — Draft the card (enrich from the ask)

Infer and flesh out each field from what the user said. Keep it concise but
complete; don't invent facts the user didn't imply, but do turn a terse ask into
a clear, well-formed card. **All fields in English.**

### If Backlog (Task Management)

- **Title** (card name) — a short imperative summary of the work, e.g. "Add
  dark-mode toggle to settings page".
- **Description** — 1–3 sentences of context: what needs to be done and why /
  the problem it solves.
- **Acceptance criteria** — a checklist of the concrete, verifiable conditions
  that must be true for the task to be considered done. Write 2–5 items, each a
  testable statement (prefer "Given/When/Then" or "User can…" phrasing).
- **Label** — pick **exactly one** from the Backlog-label table above, by the
  nature of the work:
  - **Feature** — net-new user-facing capability or behavior that didn't exist.
  - **Improvement** — enhances something that already exists (better UX, more
    performance, expanded scope of an existing feature).
  - **Chore** — maintenance / housekeeping with no user-facing behavior change
    (deps, config, tooling, docs, cleanup, releases).
  - **Refactor** — restructuring existing code without changing its behavior.

  When two fit, prefer the most specific: new capability → Feature; behavior-
  preserving code restructure → Refactor over Chore; user-visible enhancement →
  Improvement over Feature.

Compose the **description field** as:

```
<description paragraph>

## Acceptance Criteria
- [ ] <criterion 1>
- [ ] <criterion 2>
```

### If Bug (Issue)

- **Title** (card name) — a short summary of the defect, e.g. "Login button
  unresponsive on mobile Safari".
- **Description** — context and, when derivable, the steps to reproduce.
- **Actual result (bug)** — what currently happens (the wrong behavior).
- **Expected result** — what should happen instead.

Compose the **description field** as:

```
<description / steps to reproduce>

## Actual Result
<what happens now — the bug>

## Expected Result
<what should happen instead>
```

## Step 3 — Confirm, then create

1. Show the drafted card to the user (board + all fields) so they can eyeball it.
   For a clear, unambiguous ask you may create it directly and show the result —
   don't over-block on confirmation, but never guess a board when it's a toss-up.

2. Create the card with `mcp__trello__add_card_to_list`, passing `boardId`,
   `listId`, `name` (title), and `description` (the composed markdown from
   step 2). **For a Backlog card, also pass `labels: ["<labelId>"]`** with the
   single label id chosen in step 2. (Bug cards take no label.)

3. **Backlog only — add the acceptance criteria as a real checklist** so it's
   trackable in Trello, not just prose:
   - `mcp__trello__create_checklist` with `name: "Acceptance Criteria"` and the
     new `cardId`.
   - one `mcp__trello__add_checklist_item` per criterion (`checkListName:
     "Acceptance Criteria"`, `cardId`, `text`).

4. **Report back** with the card name, which board/list it landed on, the label
   (for Backlog cards), and the card URL returned by the create call.

## Notes

- Never create the card on any list other than Backlog or Bug — those are the
  only two intake lists. Other lists (Todo, In Progress, Progress, Done) are for
  moving cards later, not for filing new ones.
- To view existing cards instead of creating one, use the `review-tasks` skill.
