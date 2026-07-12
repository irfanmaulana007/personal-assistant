---
description: Debug a conversation run from the "RUN DETAIL" text copied out of the Logs screen
---

Debug a single conversation run using the plain-text **RUN DETAIL** block that a
maintainer copies from the Logs screen (the "Copy debug detail" button on a
trace, produced by `buildDebugText` in `client/src/components/Logs.tsx`).

The user will paste a block that looks like this:

```
=== RUN DETAIL ===
ID: 43
Status: ok
Created: 2026-07-11T03:07:28Z
User: Irfan (#1)
Channel: whatsapp
Model: deepseek-v4-flash
Tokens: 13913 (prompt 13456 / completion 457)
Latency (ms): 9587
Est. cost (USD): 0.0020118000000000002
Skills: bucket_list, ask_about_contact, travel_control, activity_summary, food_calories, hiking_tracker, english_tutor

--- Input ---
Minta braket piala dunia dong

--- LLM calls (1) ---
#1 deepseek-v4-flash · 13913 tok (13456/457) · 7493ms · $0.0020118000000000002 · stop

--- Output ---
Haha sayangnya aku nggak bisa ngasih bracket Piala Dunia...
```

If the user invoked this command without pasting a block, ask them to paste the
copied RUN DETAIL text first, then proceed.

## What the format is for

It is a **self-contained snapshot of one conversation turn** (one "trace") — the
user's message, which model and skills handled it, every LLM call and tool call
made along the way, the optional LLM-judge quality score, and the final reply.
It exists so a run can be pasted straight into a chat or issue and debugged
without database access. Sections are emitted only when they have data, so a
given block may omit some of the ones below.

## How to read each field

**Header (metadata)**
- `ID` — the trace's database id. Use it to look the run up again in the Logs UI
  or the store.
- `Status` — `ok` or an error status. Anything other than `ok` means the turn
  did not complete cleanly; pair it with the `--- Error ---` section.
- `Created` — UTC timestamp of the turn.
- `User` — display name and `#<user_id>`.
- `Channel` — where the message came from (`whatsapp`, `web`, …).
- `Model` — the top-level model chosen for the turn.
- `Tokens` — `total (prompt / completion)`. A large prompt count usually means a
  big system prompt, long history, or many skills injected.
- `Latency (ms)` — wall-clock latency for the whole turn.
- `Est. cost (USD)` — estimated cost of the turn.
- `Skills` — the skills that were made available / selected for this turn. A
  wrong or missing skill here is a common root cause of bad answers.

**`--- Quality score ---`** (only if the run was judged)
- `Overall X / 5`, plus `Accuracy` / `Helpfulness` / `Safety` sub-scores, the
  `Judge model`, and a `Rationale`. A low score with a rationale is the fastest
  pointer to what went wrong.

**`--- Input ---`** — the exact user message that triggered the turn.

**`--- Error ---`** (only on failures) — the error string. Read this first when
`Status` is not `ok`.

**`--- Tool calls (N) ---`** (only if tools ran) — each tool in order:
`[i] name (latency)`, its `arguments`, and its `result` (both pretty-printed
JSON). Check whether the right tool was called with sane arguments and whether
its result actually supports the final answer.

**`--- LLM calls (N) ---`** — one line per model step:
`#step model · total tok (prompt/completion) · latency · $cost · finish`.
The trailing field is the **finish reason** — either a normal stop reason
(`stop`, `length`, …) or the list of tool calls that step requested. Multiple
steps usually mean a tool-use loop.

**`--- Output ---`** — the final reply sent to the user.

## How to check / debug — step by step

1. **Confirm what "wrong" means.** Restate the input and the output in your own
   words and identify the actual complaint (wrong answer, refusal, missing tool
   call, too slow, too expensive, low judge score). Do not guess.
2. **Triage on `Status` + `Error`.** If `Status` isn't `ok`, read the `Error`
   section first — the rest of the flow may be a side effect of that failure.
3. **Check skill selection.** Compare `Skills` against what the input asked for.
   If the skill that should have handled the request isn't listed (or an
   irrelevant one is), the routing is the bug, not the model. Trace skill
   selection in the server (`server/`).
4. **Walk the LLM calls.** Read `--- LLM calls ---` top to bottom. A single
   `stop` step is a plain answer. Multiple steps with tool-call finish reasons
   mean a tool loop — line them up with the `--- Tool calls ---` section.
5. **Verify each tool call.** For every tool: was it the right tool? Are the
   `arguments` correct given the input? Does the `result` actually support the
   final `Output`? A good tool result contradicted by the output is a
   prompt/model problem; a bad tool result is a tool/integration problem.
6. **Read the Output against the Input.** Decide if the reply is correct,
   grounded in the tool results, and on-channel (e.g. WhatsApp-appropriate).
7. **Cross-check the judge score** if present — a low sub-score (accuracy vs.
   safety vs. helpfulness) plus its rationale narrows the category of failure.
8. **Sanity-check cost & latency.** Unusually high `Tokens`/`Latency`/cost point
   at bloated context, an over-large model, or an unnecessary tool loop even
   when the answer itself is fine.
9. **Locate the code and propose a fix.** Map the conclusion to a layer —
   skill routing, tool implementation, prompt/model config (`server/`), or the
   Logs rendering itself (`client/src/components/Logs.tsx`) — and, if the user
   wants, dig in and fix it. To pull the same run fresh, use its `ID`.

Finish with a short diagnosis: the most likely root cause, the evidence in the
block that supports it, and the concrete next step.
