---
name: review-tasks
description: Review every task and bug on the "Personal Assistant" Trello workspace. Lists all cards across the "Task Management" board (Backlog / Todo / In Progress / Done) and the "Issue" board (Bug / Progress / Done), grouped by board and list. Use when the user wants to see, check, summarize, or audit their open tasks, backlog, or bugs (e.g. "what's on my board", "list my tasks", "any open bugs", "review the backlog").
---

# Review tasks

Fetch and summarize every card across both boards of the **Personal Assistant**
Trello workspace, so the user gets a single at-a-glance view of their tasks and
bugs without opening Trello.

## Fixed IDs (this workspace)

These IDs are stable — use them directly, do not re-discover them each run. Only
re-list via `mcp__trello__get_lists` if a `listId` below returns an error (a list
may have been renamed or archived).

**Workspace "Personal Assistant"** — `6a54dd566d0f1d87fc3d9c54`

**Board "Task Management"** — `6a54dd8eecaab3bd510528ba`
| List | listId |
| --- | --- |
| Backlog | `6a54dda0cf5b49c7fb6f8b15` |
| Todo | `6a54dda5bd020d9d6740ade7` |
| In Progress | `6a54dda869fc7862c4139b49` |
| Done | `6a54ddadf123f4f7f7955c95` |

**Board "Issue"** — `6a54edaae21957ab935c81f6`
| List | listId |
| --- | --- |
| Bug | `6a54edaae21957ab935c820f` |
| Progress | `6a54edaae21957ab935c8210` |
| Done | `6a54edaae21957ab935c8211` |

## Procedure

1. **Scope the request.** If the user asked only about tasks, only about bugs,
   or only about a specific list (e.g. "what's in progress"), fetch just those
   lists. Otherwise fetch every list on both boards.

2. **Fetch cards per list** with `mcp__trello__get_cards_by_list_id`, passing the
   `boardId` and `listId` from the tables above. Run the independent fetches in
   parallel (one tool call per list, in a single message).

3. **Present the results** grouped by board, then by list, in the board/list
   order shown above. For each list show a heading with the list name and card
   count, then each card as a bullet with its name. Keep descriptions collapsed
   unless the user asked for detail — if they did, include the description and,
   for Task Management cards, the acceptance-criteria checklist
   (`mcp__trello__get_acceptance_criteria` with the `cardId`).

4. **Summarize.** End with a one-line rollup: total open tasks (Task Management,
   excluding Done), total open bugs (Issue, excluding Done), and anything In
   Progress right now. Skip empty lists in the summary but note if a whole board
   is empty.

## Notes

- This skill is **read-only** — it never creates, moves, or edits cards. To add a
  card, use the `create-card` skill instead.
- Cards may be written in any language; present them as-is (do not translate the
  existing card text).
