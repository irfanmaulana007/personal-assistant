---
description: Force-merge all open PRs one by one, resolving conflicts on the branch as they come up
---

Merge every open pull request in this repository, one at a time, making sure each
merge is clean before moving to the next. Resolve any conflicts on the PR's own
branch, then merge. Repeat until there are no open PRs left.

**No human review is required. Force-merge every open PR** — this repo's PRs are
self-authored, so do not wait for approvals or treat a missing review as a
blocker. Merge drafts too: mark a draft ready with `gh pr ready <number>` right
before merging it. The only legitimate reasons to skip a PR are an unresolvable
conflict or a genuinely failing required check (not a missing review).

## Procedure

1. **List open PRs**, oldest first (so earlier work merges before later work that
   may build on it):

   ```
   gh pr list --state open --json number,title,headRefName,mergeable,mergeStateStatus,isDraft --limit 100
   ```

   If there are none, report that everything is already merged and stop.

2. **For each PR, in order**, do the following:

   a. Refresh the PR's mergeability, since merging a previous PR can turn a
      clean PR into a conflicting one:

      ```
      gh pr view <number> --json number,title,headRefName,mergeable,mergeStateStatus,isDraft
      ```

   b. **If `mergeable` is `MERGEABLE`** (no conflicts) — merge it. If it is a
      draft (`isDraft` is `true`), mark it ready first, then merge:

      ```
      gh pr ready <number>   # only if the PR is a draft
      gh pr merge <number> --merge --delete-branch
      ```

      Prefer `--merge`. Only use `--squash` if the repo convention clearly calls
      for it. Do not use `--admin` to bypass required checks.

      Note: `--delete-branch` may fail to delete the *local* branch with
      "cannot delete branch ... used by worktree" — that is harmless. The PR is
      merged and the remote branch is deleted; only the local checkout in a
      worktree is left in place. Ignore that specific error and continue.

   c. **If `mergeable` is `CONFLICTING`** — resolve on the PR's branch, do NOT
      touch `main`:

      - Fetch and check out the branch:
        ```
        git fetch origin
        git checkout <headRefName>
        git merge origin/main
        ```
      - Resolve every conflict. Read both sides, understand the intent of each
        change, and produce a correct merged result — never blindly pick one
        side or delete conflicting code to make it build. Preserve the behavior
        both branches intended.
      - After resolving:
        ```
        git add -A
        git commit --no-edit
        make build && make test   # verify the merge result compiles and passes
        git push origin <headRefName>
        ```
      - Wait for GitHub to recompute mergeability, then merge (marking ready
        first if it is a draft):
        ```
        gh pr ready <number>   # only if the PR is a draft
        gh pr merge <number> --merge --delete-branch
        ```

   d. **If `mergeable` is `UNKNOWN`** — GitHub is still computing it. Wait a few
      seconds and re-run step (a) for this PR before deciding.

   e. If a PR has required checks that are failing (not just conflicts), stop and
      report it rather than force-merging. A missing or absent review is NOT a
      reason to stop — force-merge regardless of review state.

3. **After each successful merge**, return to `main` locally and pull so the next
   conflict resolution is based on the latest code:

   ```
   git checkout main
   git pull origin main
   ```

4. **Repeat** from step 1 until `gh pr list --state open` returns nothing.

## Rules

- Force-merge every open PR — no human review, no approval wait. Mark drafts
  ready and merge them.
- Never force-push, never push to `main` directly, never merge with `--admin` to
  skip failing required checks.
- Merge PRs in ascending PR-number order unless told otherwise, so dependent work
  lands after what it depends on.
- Re-check mergeability right before every merge — a clean PR can become
  conflicting after an earlier merge lands.
- When resolving conflicts, build and test before pushing. If the build or tests
  fail after a resolution you cannot fix confidently, stop and report the PR and
  the failure instead of merging.
- Report a running summary: which PRs merged cleanly, which needed conflict
  resolution, and any that were skipped and why.
