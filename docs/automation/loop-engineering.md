# Loop engineering: autonomous fix/improve loop on the VPS

A self-driving [Claude Code](https://docs.claude.com/en/docs/claude-code) agent
that runs **on the VPS on a schedule**, reads production signals from **Sentry**
(errors) and **MongoDB** (application logs), and turns them into fixes and
latency improvements. Each pass opens a **PR against `staging`**, and — only when
the fix is unambiguously safe — **auto-merges it to `staging`**, which Dokploy
then rebuilds and deploys. Everything risky is left as an open PR for you to
review.

This is "loop engineering" (a.k.a. the Ralph loop): give one agent a tight,
well-guarded prompt and run it over and over. The intelligence is in the
**guardrails**, not the cleverness of any single run.

> **TL;DR of the safety model:** the loop can *ship* only trivially-safe changes
> to `staging` on its own. `main` is never touched (only `/release` promotes
> `staging → main`). Anything non-obvious becomes a normal PR that waits for a
> human. There is a one-file kill switch.

---

## Table of contents

1. [How it works](#1-how-it-works)
2. [Prerequisites](#2-prerequisites)
3. [MCP servers the loop needs](#3-mcp-servers-the-loop-needs)
4. [The operating prompt](#4-the-operating-prompt-the-heart-of-the-loop)
5. [What counts as "obvious" (auto-merge policy)](#5-what-counts-as-obvious-auto-merge-policy)
6. [Wiring it up on the VPS](#6-wiring-it-up-on-the-vps)
7. [The runner script](#7-the-runner-script)
8. [Scheduling](#8-scheduling-cron-or-systemd-timer)
9. [Observing the loop itself](#9-observing-the-loop-itself)
10. [Cost, rate, and safety controls](#10-cost-rate-and-safety-controls)
11. [Kill switch & rollback](#11-kill-switch--rollback)
12. [Rollout plan](#12-rollout-plan-crawl--walk--run)
13. [FAQ / failure modes](#13-faq--failure-modes)

---

## 1. How it works

```
┌──────────────────────────── VPS (Dokploy host) ────────────────────────────┐
│                                                                             │
│   cron / systemd timer  ──►  loop-runner.sh  ──►  claude -p  (headless)      │
│                                                     │                        │
│        ┌────────────────────────────────────────────┼─────────────────┐     │
│        ▼                     ▼                        ▼                 │     │
│   Sentry MCP           mongo-prod MCP           git worktree            │     │
│   (open issues,        (error/latency logs      (clean clone of         │     │
│    stack traces,        from the `logs` DB)      staging to edit)        │     │
│    frequency)                                                            │     │
│        │                     │                        │                 │     │
│        └─────────► triage ───┴──► fix in worktree ────┘                 │     │
│                       │                                                  │     │
│                       ▼                                                  │     │
│                 make build / make test / make lint                       │     │
│                       │                                                  │     │
│                       ▼                                                  │     │
│              gh pr create --base staging  ──►  label + body              │     │
│                       │                                                  │     │
│                 obvious? ──yes──► gh pr merge --squash (auto)            │     │
│                       │                                                  │     │
│                       └──no──► leave open for human review              │     │
└─────────────────────────────────────────────────────────────────────────────┘
                                       │
                          Dokploy webhook on push to staging
                                       │
                                       ▼
                        rebuild + redeploy changed service
```

Each **iteration** is a single headless Claude Code invocation (`claude -p`) with
a fixed prompt. The prompt tells the agent to:

1. **Pull signals** — query Sentry for the top unresolved issues and mongo-prod
   for recent error/slow-request log lines.
2. **Pick one** — the single highest-value item this pass (one issue = one
   branch = one PR). Never batch unrelated fixes.
3. **Reproduce & understand** — read the stack trace, find the code, form a
   root-cause hypothesis. If it can't, it writes up findings and stops (no blind
   edits).
4. **Fix in a worktree** — make the smallest correct change.
5. **Verify** — `make build`, `make test`, `make lint` must all pass.
6. **Ship** — open a PR against `staging` with a full description; auto-merge
   only if the change clears the "obvious" bar (§5).
7. **Annotate Sentry/Mongo** — comment on the Sentry issue with the PR link; if
   auto-merged, mark it resolved-in-next-release.

Because it runs **one item per pass** and re-reads live signals every time, the
loop naturally converges: fixed issues stop showing up, so the next pass moves to
the next-worst thing.

---

## 2. Prerequisites

On the VPS (the Dokploy host, `72.61.141.226` per the deployment notes — run the
loop as a **non-root** dedicated user, e.g. `loopbot`):

| Requirement | Notes |
| --- | --- |
| **Claude Code CLI** | `npm i -g @anthropic-ai/claude-code` (or the native installer). Verify `claude --version`. |
| **Auth** | Set `ANTHROPIC_API_KEY` for the `loopbot` user (headless can't do interactive OAuth). Use a **dedicated key** so you can meter and revoke it independently. |
| **`gh` CLI** | Authenticated as a **bot GitHub account / fine-grained PAT** with `repo` + `pull_request` write on `personal-assistant` only. Do **not** reuse your personal token. |
| **git** | The runner clones/worktrees a fresh checkout each pass so the loop never edits a live working copy. |
| **Go 1.25+ / Node + pnpm** | Needed so the agent can actually run `make build`, `make test`, `make lint`. Same toolchain the Dockerfiles use. |
| **Network egress** | To `api.anthropic.com`, Sentry, MongoDB (prod), and GitHub. |

> **Why a dedicated `loopbot` identity everywhere** (OS user, GitHub account,
> Anthropic key, Sentry token): so every automated action is attributable,
> independently rate-limitable, and revocable with a single credential rotation
> if the loop ever misbehaves.

---

## 3. MCP servers the loop needs

The loop reuses the **same MCP servers already configured for this project** —
`sentry`, `mongo-prod`, and `github` — plus optionally `postgres-prod` for
read-only latency diagnosis. Put them in an `.mcp.json` that the runner points
at (keep this file **outside** the repo checkout, in the `loopbot` home dir, so
secrets never land in a commit):

```jsonc
// ~/loopbot/.mcp.json  — read-mostly production observability
{
  "mcpServers": {
    "sentry": {
      "type": "http",
      "url": "https://mcp.sentry.dev/mcp",
      "headers": { "Authorization": "Bearer ${SENTRY_AUTH_TOKEN}" }
    },
    "mongo-prod": {
      "command": "npx",
      "args": ["-y", "mongodb-mcp-server", "--readOnly"],
      "env": { "MDB_MCP_CONNECTION_STRING": "${MONGO_PROD_URI_READONLY}" }
    },
    "github": {
      "type": "http",
      "url": "https://api.githubcopilot.com/mcp/",
      "headers": { "Authorization": "Bearer ${GH_LOOPBOT_TOKEN}" }
    }
  }
}
```

**Least privilege is the whole point here:**

- **`mongo-prod` → read-only.** The loop must *read* logs, never mutate prod
  data. Use a MongoDB user scoped to `read` on the `logs` DB, and pass
  `--readOnly` so even a hallucinated write is refused at the server.
- **`postgres-prod`** (optional, for `EXPLAIN`/slow-query analysis) → a
  **read-only** DB role. Never give the loop a mutation-capable Postgres MCP.
- **`sentry`** → a token with issue read + comment/resolve scopes, nothing more.
- **`github`** → PR create/merge on this one repo. **No admin, no branch-protection
  bypass** (see §5 — branch protection is what makes auto-merge safe).

Prefer the CLI equivalents (`claude mcp add ...`) if you'd rather not hand-write
JSON; the shapes above match how `sentry` and `mongo-prod` are already registered
for local use.

---

## 4. The operating prompt (the heart of the loop)

Save this as `~/loopbot/PROMPT.md`. It is passed verbatim to `claude -p` each
pass. Everything the loop is and isn't allowed to do lives here — treat it like
production code and change it deliberately.

````markdown
You are the autonomous reliability loop for the **personal-assistant** project,
running headless on the VPS. Your job each run: turn ONE production signal into
ONE safe improvement, shipped as a PR against `staging`.

## Non-negotiable rules
- **Never touch `main`.** Only PRs into `staging`. `main` is advanced solely by
  the human `/release` process.
- **One item per run.** One issue → one branch → one focused PR. Never bundle
  unrelated changes.
- **No blind edits.** If you cannot form a concrete root-cause hypothesis from
  the stack trace + code + logs, do NOT edit anything. Write your findings to the
  Sentry issue as a comment and stop.
- **Read-only in production.** Mongo/Postgres/Sentry access is for diagnosis. You
  may comment on / resolve Sentry issues; you may not mutate app data.
- **Verify before shipping.** `make build`, `make test`, and `make lint` must all
  pass. If any fails and you can't fix it cleanly within this run, open the PR as
  a normal (non-auto-merge) PR and say so.
- Respect CLAUDE.md: theme rules, PR description rules, labels, conventions.

## Procedure
1. **Gather signals.**
   - Sentry: list top UNRESOLVED issues for this project, newest/most-frequent
     first. Note error type, frequency, first/last seen, stack trace.
   - mongo-prod (`logs` DB, read-only): recent ERROR-level lines and slow
     requests (high latency). Correlate with the Sentry issues.
2. **Choose the single highest-value item.** Prefer: crash/500s affecting many
   users > data-integrity bugs > clear latency regressions > log noise. If
   nothing actionable, exit 0 with "no actionable signal this pass".
3. **Reproduce & locate.** Map the trace to code. Confirm the root cause. If
   unsure, comment findings on the Sentry issue and STOP.
4. **Fix.** Smallest correct change. Add/adjust a test that would have caught it.
5. **Verify:** `make build && make test && make lint`.
6. **Open the PR** against `staging`:
   - Branch: `loop/<kebab-summary>`.
   - Title: `fix: <summary>` or `perf: <summary>`.
   - Body MUST follow CLAUDE.md PR rules (What & why / Before vs after / Why it
     matters / Scope & notes) AND include: the Sentry issue link, the log
     evidence, and a `Loop-run: <timestamp>` trailer.
   - Add a type label (`fix`, `improvement`, `perf`, ...).
7. **Decide auto-merge** using the "obvious" checklist below. If it qualifies,
   squash-merge into `staging`. Otherwise leave it open and prefix the title with
   `[review] `.
8. **Annotate Sentry:** comment on the issue with the PR URL. If auto-merged,
   note it will resolve on next `staging` deploy.
9. **Report** a one-paragraph summary (what you shipped or why you didn't) as
   your final message.

## "Obvious → auto-merge" checklist (ALL must be true)
- Change is confined to a bug's direct cause; diff is small (rule of thumb:
  ≤ ~40 changed lines, ≤ 3 files).
- No schema/migration change, no dependency bump, no config/secret/env change,
  no auth/permission/RBAC logic, no money/billing logic, no data backfill.
- No public API contract change and no user-visible UX/behaviour change beyond
  fixing the reported defect.
- A test now covers the fixed path and the full suite passes.
- You are confident the fix is correct, not merely plausible.
If ANY item is false → open as `[review]` and do NOT merge.
````

Tune the numeric thresholds to your taste — they're deliberately conservative.

---

## 5. What counts as "obvious" (auto-merge policy)

The prompt's checklist is the agent's self-check, but **do not rely on the model
alone** to enforce it. Back it with mechanical guardrails so a bad self-judgement
can't ship anything dangerous:

1. **Branch protection on `staging`** — require CI (`ci.yml`: build + lint +
   typecheck) to pass before merge. The loop's `gh pr merge` then *cannot*
   succeed until green, no matter what the model thinks. This is your hard floor.
2. **A `staging`-only CODEOWNERS / path guard for sensitive files** — require a
   human review when the diff touches migrations, `go.mod`/`go.sum`,
   `package.json`, auth, RBAC, or deploy config. Even if the loop tries to
   auto-merge, GitHub blocks it and it falls back to an open PR.
3. **`--squash` merges only** — one revertable commit per fix on `staging`.
4. **Post-merge smoke** — after auto-merge, the runner can poll the Sentry issue
   for N minutes; if error volume rises after deploy, it opens a revert PR.

**Auto-mergeable examples:** nil-pointer guard on an already-tested handler; an
off-by-one in pagination; a missing `ctx` cancellation; adding a DB index hint /
`.limit()` to a proven-slow read-only query; a wrong error message.

**Never auto-merge (always `[review]`):** anything with a migration, a dependency
change, auth/RBAC/permission logic, money/quota logic, cross-project data
scoping (see the LID/project-scoping incidents in past work), a new env var, or
any user-visible behaviour change.

---

## 6. Wiring it up on the VPS

```sh
# as loopbot
mkdir -p ~/loopbot
cd ~/loopbot

# 1. Credentials (0600, never committed). Load these in the runner, not here.
cat > ~/loopbot/.env <<'EOF'
ANTHROPIC_API_KEY=sk-ant-...
SENTRY_AUTH_TOKEN=...
MONGO_PROD_URI_READONLY=mongodb://readonly:...@host:27017/logs
GH_LOOPBOT_TOKEN=github_pat_...
EOF
chmod 600 ~/loopbot/.env

# 2. MCP + prompt (from §3 and §4)
#    ~/loopbot/.mcp.json
#    ~/loopbot/PROMPT.md

# 3. gh auth for the bot identity
GH_TOKEN=$GH_LOOPBOT_TOKEN gh auth status
```

The runner always operates on a **fresh checkout of `staging`** in a throwaway
worktree, so the loop never collides with your local work or a previous pass.

---

## 7. The runner script

`~/loopbot/loop-runner.sh` — one pass per invocation. Keep the loop *outside*
Claude (cron re-invokes it); each run is a clean, bounded, headless process.

```bash
#!/usr/bin/env bash
set -euo pipefail

BASE=~/loopbot
REPO="$BASE/repo"                     # long-lived clone
WORK="$BASE/work/$(date +%s)"         # throwaway worktree for this pass
LOG="$BASE/logs/$(date +%Y%m%d-%H%M%S).log"
mkdir -p "$BASE/logs" "$BASE/work"

# ── Kill switch ──────────────────────────────────────────────────────────────
if [[ -f "$BASE/PAUSED" ]]; then echo "loop paused; exiting" | tee -a "$LOG"; exit 0; fi

# ── Load secrets ─────────────────────────────────────────────────────────────
set -a; source "$BASE/.env"; set +a
export GH_TOKEN="$GH_LOOPBOT_TOKEN"

# ── Fresh staging checkout ───────────────────────────────────────────────────
[[ -d "$REPO" ]] || git clone https://github.com/irfanmaulana007/personal-assistant "$REPO"
git -C "$REPO" fetch origin --prune --quiet
git -C "$REPO" worktree add --force -B loop-pass "$WORK" origin/staging
trap 'git -C "$REPO" worktree remove --force "$WORK" 2>/dev/null || true' EXIT

cd "$WORK"

# ── One headless pass ────────────────────────────────────────────────────────
# --print: headless. --permission-mode: no interactive prompts. Allowed tools are
# scoped so the loop can build/test/PR but not run arbitrary destructive shell.
timeout 30m claude \
  --print \
  --mcp-config "$BASE/.mcp.json" \
  --permission-mode acceptEdits \
  --allowedTools "Read,Edit,Write,Bash(make build),Bash(make test),Bash(make lint),Bash(git *),Bash(gh pr *),mcp__sentry__*,mcp__mongo-prod__*,mcp__github__*" \
  --append-system-prompt "$(cat "$BASE/PROMPT.md")" \
  "Run one reliability-loop pass now. Follow PROMPT.md exactly." \
  2>&1 | tee -a "$LOG"

echo "pass complete: $(date -Is)" | tee -a "$LOG"
```

Notes:
- **`timeout 30m`** bounds every pass — a stuck run can't hang the VPS or burn
  unbounded tokens.
- **`--allowedTools`** is an allowlist: build/test/lint/git/gh + the three MCP
  servers, and nothing else. No `rm`, no arbitrary curl, no prod writes. Adjust to
  the exact tool names your Claude Code version reports.
- Keep the flag names in step with your installed CLI (`claude --help`); headless
  = `-p`/`--print`, and the permission/allowlist flags evolve — verify once.

---

## 8. Scheduling: cron or systemd timer

**Cron** (simplest):

```cron
# ~loopbot crontab — one pass every 30 min during the day, on the hour overnight.
*/30 8-23 * * *  /home/loopbot/loopbot/loop-runner.sh >> /home/loopbot/loopbot/cron.log 2>&1
0    0-7  * * *  /home/loopbot/loopbot/loop-runner.sh >> /home/loopbot/loopbot/cron.log 2>&1
```

**systemd timer** (better logging + no overlap):

```ini
# /etc/systemd/system/loopbot.service
[Service]
Type=oneshot
User=loopbot
ExecStart=/home/loopbot/loopbot/loop-runner.sh
# hard resource caps
MemoryMax=4G
CPUQuota=200%
TimeoutStartSec=35min
```

```ini
# /etc/systemd/system/loopbot.timer
[Timer]
OnBootSec=10min
OnUnitActiveSec=30min      # gap AFTER the previous run finishes → no overlap
Persistent=true
[Install]
WantedBy=timers.target
```

```sh
sudo systemctl enable --now loopbot.timer
systemctl list-timers loopbot.timer
```

Prefer the systemd timer: `OnUnitActiveSec` measures the gap *after* completion,
so passes never overlap (cron would fire on a fixed wall-clock schedule even if
the previous pass is still running).

> There is also a project `/loop` skill and a `/schedule` (cloud cron) skill.
> Those drive an **interactive/cloud** Claude Code session and are great for
> ad-hoc "watch this for me" use. For an always-on, headless, VPS-resident loop,
> the cron/systemd + `claude -p` approach above is the durable one — it survives
> your laptop being closed and doesn't depend on a live session.

---

## 9. Observing the loop itself

Treat the loop as a service with its own telemetry:

- **Per-pass logs** — `~/loopbot/logs/<timestamp>.log` (the full transcript) plus
  `cron.log`. Rotate with `logrotate` (keep ~14 days).
- **A run ledger** — append one JSON line per pass (issue picked, PR opened,
  merged y/n, tokens, duration) to `~/loopbot/ledger.jsonl`. Cheap to grep, easy
  to chart later.
- **Dogfood it** — write those pass summaries into the same MongoDB `logs` DB the
  app uses (a `loop_runs` collection), so the loop's own activity shows up in your
  existing log tooling.
- **PR trail** — every action is already a PR with a `Loop-run:` trailer and a
  `loop/*` branch, so `gh pr list --label fix --search "loop/"` is your audit log.
- **Alerts** — if K consecutive passes error (build failures, auth expiry, no
  network), have the runner ping you (WhatsApp/email — the app already sends
  both).

---

## 10. Cost, rate, and safety controls

| Control | Mechanism |
| --- | --- |
| **Token budget per pass** | `timeout 30m` + a scoped prompt; optionally set a `--max-turns` cap so one pass can't spiral. |
| **Cadence** | 30-min gap via systemd `OnUnitActiveSec`; back off overnight. Start slower (hourly) and speed up once trusted. |
| **Blast radius** | One item per pass; `--allowedTools` allowlist; read-only prod DB roles; branch protection on `staging`. |
| **No `main` access** | The bot GitHub token has no path to `main`; only `/release` promotes. Enforce with branch protection requiring a human. |
| **Dedicated key** | Separate `ANTHROPIC_API_KEY` so you can watch spend and revoke without touching your own. |
| **Idempotence** | Fresh `origin/staging` worktree each pass; nothing carries over, so a crashed pass leaves no half-state. |
| **Duplicate-work guard** | Before opening a PR, the agent checks for an existing open `loop/*` PR for the same Sentry issue and skips if found. |

---

## 11. Kill switch & rollback

- **Pause instantly:** `touch ~/loopbot/PAUSED`. The runner exits at the top of
  the next pass without doing anything. Remove the file to resume.
- **Stop scheduling:** `sudo systemctl disable --now loopbot.timer` (or comment
  the crontab).
- **Revoke access:** rotate `GH_LOOPBOT_TOKEN` / `ANTHROPIC_API_KEY` — kills all
  future actions immediately.
- **Undo a bad auto-merge:** every merge is a single squash commit on `staging`;
  `gh pr revert` or `git revert <sha>` + push. Because auto-merges are trivial by
  policy, reverts are trivial too. `main` is unaffected regardless.

---

## 12. Rollout plan (crawl → walk → run)

1. **Shadow (read-only).** Run the loop with auto-merge disabled — it only opens
   `[review]` PRs. Review them for a week: are its picks and fixes good? Tune
   `PROMPT.md`.
2. **Assisted auto-merge.** Enable auto-merge but keep branch protection strict
   and watch the ledger daily. Confirm it never auto-merges anything from the
   "never" list.
3. **Steady state.** Widen cadence, let it run. Periodically audit `loop/*` PRs
   and the revert rate. Feed recurring false-picks back into the prompt.

Never skip step 1. The whole system is only as trustworthy as the week you spent
watching it open PRs you didn't merge.

---

## 13. FAQ / failure modes

**Q: What if it opens a wrong-but-plausible fix and auto-merges it?**
Branch protection requires CI green, the diff is tiny by policy, and it lands as
one squash commit on `staging` (never `main`). Revert is one command, and the
post-merge Sentry check (§5.4) can auto-open the revert. `staging` is the
designated place for exactly this risk.

**Q: Can it make prod worse directly?**
No. It has read-only prod DB roles and no deploy access. It can only *propose
code*; Dokploy deploys `staging` on merge, which is your pre-prod branch, not
`main`.

**Q: What if Sentry/Mongo are quiet?**
The pass exits 0 with "no actionable signal" and costs almost nothing. Quiet is
the goal state.

**Q: Two passes overlapping?**
systemd `OnUnitActiveSec` prevents it; each pass also uses a unique worktree, so
even a manual double-run won't corrupt state.

**Q: How is this different from the `/trello-fix` skill?**
`/trello-fix` fixes *human-filed* Bug cards. This loop is *signal-driven* — it
discovers issues from Sentry/Mongo before anyone files them. They're
complementary: the loop can even file a Trello Bug card for anything it judges
non-obvious, feeding your existing human workflow.

---

## See also

- [`docs/deployment/dokploy-split.md`](../deployment/dokploy-split.md) — how
  `staging`/`main` map to Dokploy builds and Watch Paths.
- [`.claude/commands/release.md`](../../.claude/commands/release.md) — the only
  sanctioned path from `staging` to `main`.
- `CLAUDE.md` → **Pull requests** — the PR/label/target-branch rules the loop
  must obey.
- [Claude Code headless / SDK docs](https://docs.claude.com/en/docs/claude-code/sdk)
  — `claude -p`, `--allowedTools`, `--mcp-config`, permission modes.
