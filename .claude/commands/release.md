---
description: Bump the app version (semver) on staging, then promote staging → main via PR and tag the release
argument-hint: major | minor | patch | X.Y.Z (default patch)
---

Cut a new release of **personal-assistant**. This is the *only* command that is
allowed to move code onto `main` and tag it. Everyday work flows into `staging`
(see CLAUDE.md → Pull requests); running this command is the deliberate act of
promoting whatever is on `staging` to `main` under a new version number.

The requested bump is `$ARGUMENTS` (one of `major`, `minor`, `patch`, or an
explicit `X.Y.Z`). If it is empty, treat it as `patch`.

The single source of truth for the app version is the root `package.json`
`version` field.

## Procedure

1. **Preflight — never release from a dirty or stale tree.**

   ```
   git status --porcelain        # must be empty; if not, stop and report
   git fetch origin --prune --tags
   ```

   If the working tree is dirty, stop and report it — do not stash or discard.

2. **Make sure a `staging` branch exists and is checked out, up to date.**

   ```
   git rev-parse --verify origin/staging   # does the remote branch exist?
   ```

   - If `origin/staging` exists: `git checkout staging && git pull origin staging`.
   - If it does **not** exist yet: create it from `main` so the first release has
     a base to promote from:

     ```
     git checkout main && git pull origin main
     git checkout -b staging
     git push -u origin staging
     ```

3. **Compute the new version.** Read the current `version` from the root
   `package.json`. Then:

   - `major` → bump `X`, reset minor and patch to 0 (`X+1.0.0`)
   - `minor` → bump `Y`, reset patch to 0 (`X.Y+1.0`)
   - `patch` → bump `Z` (`X.Y.Z+1`)
   - an explicit `X.Y.Z` → use it verbatim (it MUST be strictly greater than the
     current version; if not, stop and report)

   State the old → new version explicitly before changing anything, e.g.
   `1.0.0 → 1.1.0`.

4. **Update the version field** in the root `package.json` to the new version.
   Do not touch any other file. (`client/package.json` tracks the client bundle
   independently and is out of scope for the app release.)

5. **Verify the app still builds** before promoting it:

   ```
   make build
   ```

   If the build fails, stop and report — do not open a release PR for a broken
   build.

6. **Commit the bump directly to `staging`** (this is the one place a direct
   commit — no PR — is correct) and push:

   ```
   git add package.json
   git commit -m "chore(release): v<NEW_VERSION>"
   git push origin staging
   ```

7. **Open the release PR from `staging` into `main`** and label it:

   ```
   gh pr create --base main --head staging \
     --title "Release v<NEW_VERSION>" \
     --body "<see below>"
   gh label create release --description "Release / version promotion" --color 5319E7 2>/dev/null || true
   gh pr edit <number> --add-label release
   ```

   The PR body must follow the repo's PR rules (What & why / Before vs. after /
   Why it matters / Scope & notes). For a release PR, summarize **what is being
   promoted from `staging` to `main`** in this version — list the notable
   changes that have landed on `staging` since the last release tag
   (`git log <last-tag>..staging --oneline` is a good starting point) — and state
   the old → new version.

8. **Merge the PR into `main`** (no review wait — these are self-authored):

   ```
   gh pr merge <number> --merge
   ```

   Do not delete the `staging` branch — it is long-lived and keeps receiving the
   next cycle's work.

9. **Tag the release on `main`.**

   ```
   git checkout main
   git pull origin main
   git tag -a v<NEW_VERSION> -m "Release v<NEW_VERSION>"
   git push origin v<NEW_VERSION>
   ```

10. **Move released Trello cards to `Done`.** The `/trello-feat` and
    `/trello-fix` commands stamp each PR's commit with a `Trello: <card url>`
    trailer and put the card link in the PR body. Any of those cards that are
    part of this release must now be marked done on their board.

    a. **Collect the released card links** from the commits being promoted.
       Using the previous release tag as the lower bound:

       ```
       PREV=$(git describe --tags --abbrev=0 v<NEW_VERSION>^ 2>/dev/null)
       git log ${PREV:+$PREV..}v<NEW_VERSION> --pretty=%B \
         | grep -oiE 'https://trello\.com/c/[A-Za-z0-9]+' | sort -u
       ```

       If there are no matches, there are no Trello tasks in this release —
       skip the rest of this step and note that in the report.

    b. **For each unique card link, move it to `Done`.** Extract the card's
       shortLink (the segment after `/c/`) and call the `trello` MCP server's
       `get_card` with it to read the card's `idBoard`, then `move_card` it into
       the matching board's `Done` list:

       - Board **Task Management** (`6a54dd8eecaab3bd510528ba`) → Done list
         `6a54ddadf123f4f7f7955c95`
       - Board **Issue** (`6a54edaae21957ab935c81f6`) → Done list
         `6a54edaae21957ab935c8211`

       A card may sit on either board (features vs. bugs) — always move it to
       the `Done` list **of its own board**, never across boards. Optionally
       `add_comment` on the card, e.g. `Released in v<NEW_VERSION>`.

       If the `trello` MCP tools aren't loaded in the session, report which
       cards should be moved to `Done` (with their links) so the user can do it
       manually, rather than failing the release.

11. **Report** the released version, the PR number/URL, the pushed tag, and the
    Trello cards moved to `Done` (or a note that the release contained no Trello
    tasks). Leave `main` checked out and clean.

## Rules

- Only this command promotes to `main` and creates version tags. All other work
  targets `staging` via PR.
- Never force-push, never rewrite history, never tag a version that already
  exists — check `git tag` first and stop if the tag is present.
- The version bump commit is the single sanctioned direct commit to a branch;
  everything else goes through the `staging → main` PR.
- If any step fails (dirty tree, failing build, non-monotonic version, existing
  tag), stop and report rather than forcing the release through.
- Moving Trello cards to `Done` is the last step and must never block the
  release: if the Trello sync fails or its tools are unavailable, the release is
  still complete — just report which cards need moving by hand.
