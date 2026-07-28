# PRD — General Default Project, Per-User WhatsApp Numbers &amp; Self-Registration

**Status:** Draft · **Trello:** _n/a_ · **Branch:** `worktree-prd-general-project-wa-users` · **Target:** staging

> One coherent change to how WhatsApp traffic is attributed: a stable **General** project as the default agent scope, **projects that map to WhatsApp JIDs** so the agent knows which project to act in, **users that own multiple WhatsApp numbers** (so mappings are picked from a dropdown, not typed), and **self-registration** of accounts for numbers we have never seen — so every conversation, on WhatsApp or later on the web, is anchored to a real user and project.

---

## 1. Problem &amp; Goal

Today the agent's choice of project for an incoming WhatsApp message is fragile and its association to a user is thin:

- **No stable default project.** CLAUDE.md declares "project `id = 1` is the default," but that is a convention, not a guarantee. In code the fallback is "the owner's *first* project" (`app/api/cmd/assistant/main.go:534-541`, `ListProjectsForUser(...)[0]`) — order-dependent and not something anyone chose. The historical fallback wrote to the unscoped **project 0**, which is orphaned from every per-project view and leaks reads across all projects via the `($N = 0 OR project_id = $N)` predicate (`app/api/internal/store/postgres_reminders.go:56`, comments at `main.go:328` and `internal/routine/routine.go:296`).
- **WhatsApp numbers are typed by hand and float free of users.** The mapping UI (`app/web/src/components/settings/WhatsAppMappingsSettings.tsx`) takes a raw JID string and, for personal chats, a **raw integer `user_id`** (`WhatsAppMappingsSettings.tsx:381-397`). There is no list of "this user's numbers," no validation that the number belongs to the user, and no way to add a second number for the same person.
- **Unknown numbers have no identity.** When a number with no mapping messages in, the run is attributed to the platform owner and the owner's first project (`main.go:527-541`). The real sender gets no account, so there is nowhere to accumulate their context — and if that person later opens the web app, nothing links them to their WhatsApp history.

**Goal:** Make project selection deterministic and user attribution first-class:

1. A single, explicitly-designated **General** project is the agent's default scope for any WhatsApp chat (personal or group) that has no more specific mapping — changeable from the web UI.
2. Every project can be **mapped to one or more WhatsApp JIDs**; a matching mapping overrides the General default.
3. Every user can **register multiple WhatsApp numbers**, so a personal mapping is built by picking a **user → number** from dropdowns instead of typing a JID and a user id.
4. A message from an **unregistered number self-registers a user account** keyed on that number, so per-user context is captured from first contact and is ready if that number later signs in on the web.

---

## 2. Goals / Non-Goals

### Goals
- Introduce a durable "default project" concept (the **General** project) and make WhatsApp/routine fallbacks resolve to it instead of `first-project` / project 0.
- Add a `user_whatsapp_numbers` table: one user → many verified numbers, uniquely owned.
- Rework the WhatsApp mapping create/edit flow to select an **existing user and one of their numbers** for personal mappings (replacing the raw-JID + raw-`user_id` inputs).
- Add self-service number management on the user **Profile** page and in the admin user editor.
- Self-register a user (and their number) on first inbound message from an unknown personal JID, attribute the conversation to that user, and route it to the General project by default.
- Normalize JIDs (LID → phone) at mapping-lookup time so the "agent knows which project" reliably, closing the project-0 leak path.

### Non-Goals
- No change to the multi-project RBAC model itself (roles, membership, `project_members`) — see `docs/prd-multi-project-rbac.md`.
- No change to group-chat mention/allowlist gating (`whatsapp.go:419-431`) beyond routing.
- No public web signup form. Self-registration is initiated **only** by an inbound WhatsApp message, not by an anonymous web visitor.
- No phone-number-based web login/OTP in this phase (a self-registered user still needs a password set before they can log in — see §7 and Open Questions).
- No migration of the `OWNER_JID` allowlist into the database (kept as-is; interaction is discussed in §11).

---

## 3. Current State (grounded)

**Projects** — `store.Project` (`app/api/internal/store/store.go:96-102`: `ID, Name, Slug, OwnerUserID, CreatedAt`); table in `migrations/postgres/000006_multi_project_rbac.up.sql:13-19`; `slug` added by `000009_project_slug.up.sql`. "Default" is resolved dynamically as the caller's first project (`api/middleware.go:126`, `defaultProject`). The only literal `1` is the Mongo log backfill sentinel (`store/mongo_migrations.go:23`).

**WhatsApp routing** — `resolveWhatsAppScope` (`app/api/cmd/assistant/main.go:502-543`), called per message at `main.go:230`:
- lookup key = `msg.Chat` for groups, else `msg.From` (`main.go:504-507`);
- `db.GetWhatsAppMapping(jid)` — exact match, `WHERE jid = $1` (`store/postgres_projects.go:455-459`);
- on hit: scope to `m.ProjectID` / `m.Role`; personal chat with `m.UserID != 0` attributes that user (`main.go:518-519`); groups never confer superadmin (`main.go:515-516`);
- on miss: fall back to owner (`store.FirstAdmin`) + owner's first project (`main.go:527-541`).

**Mapping table** — `store.WhatsAppMapping` (`store.go:143-152`: `ID, JID, Kind('group'|'personal'), ProjectID, Role, UserID, Label, CreatedAt`); table `whatsapp_mappings` with `jid TEXT UNIQUE` (`000006_...up.sql:70-80`); admin-only CRUD API `api/whatsapp_mappings.go` routed at `api/server.go:231-234`; UI `WhatsAppMappingsSettings.tsx` mounted in `IntegrationsWhatsApp.tsx:38`.

**Users** — `store.User` (`store.go:10-18`: `ID, Email, Name, PasswordHash, Role, CreatedAt`); table `000001_init.up.sql:10-17`. **No phone/JID column anywhere on the user.** Creation paths: first-run `POST /api/auth/setup` (superadmin, no personal project — `auth.go:76,107-110`) and admin-only `POST /api/users` (auto-provisions a personal project — `users.go:107-111`, `provisionPersonalProject`). No self-registration endpoint. Self-service is name/email/password only (`Profile.tsx`, `auth.go` `handleUpdateProfile`/`handleChangePassword`).

**Users ↔ projects** — `project_members(project_id, user_id, role)` join (`000006_...up.sql:22-28`); `projects.owner_user_id` is the creator stamp. Superadmins are members of no project by design.

**LID vs phone** — WhatsApp may address a sender by LID (`…@lid`). The handler resolves LID→phone **only for the `OWNER_JID` allowlist permit check** (`transport/whatsapp/whatsapp.go:381-412`), *not* for the mapping lookup — so a chat arriving in LID form misses its phone-form mapping row and falls through to the fallback. This is the mechanism behind the project-0 orphan/leak.

---

## 4. Concepts &amp; Terminology

| Term | Meaning |
|------|---------|
| **General project** | The single project marked as the platform default. The agent acts here for any WhatsApp chat with no more specific mapping. Exactly one project is General at a time; it is reassignable from the web UI. |
| **WhatsApp mapping** | A row in `whatsapp_mappings` binding one normalized JID → project (+ role, + attributed user for personal chats). A matching mapping **overrides** the General default. |
| **User number** | A verified WhatsApp phone number owned by a user (`user_whatsapp_numbers`). A user may own many; a number belongs to at most one user. |
| **Self-registered user** | A user account created automatically on first inbound message from an unknown personal number, seeded from the WhatsApp push name and the number. |
| **Normalized JID** | Canonical phone-form key for a chat identity, after LID→phone resolution and `ToNonAD()` stripping, used for all mapping lookups and number uniqueness. |

---

## 5. Data Model Changes

### 5.1 `projects` — designate the default

Add a nullable/boolean marker plus a uniqueness guarantee. Preferred: a boolean with a partial unique index so exactly one project can be default.

```sql
ALTER TABLE projects ADD COLUMN is_default BOOLEAN NOT NULL DEFAULT false;
CREATE UNIQUE INDEX uniq_projects_single_default ON projects(is_default) WHERE is_default;
```

Seed a **General** project (owned by the platform superadmin from `FirstAdmin`) and mark it default (see §10). `store.Project` gains `IsDefault bool`; add `store.GetDefaultProject(ctx)` and `store.SetDefaultProject(ctx, id)`.

> Alternative considered: a `settings(key,value)` row `default_project_id`. The boolean-on-projects approach keeps the flag next to the entity and lets the partial index enforce "exactly one" — recommended. (Open question 14.2.)

### 5.2 New table `user_whatsapp_numbers`

```sql
CREATE TABLE user_whatsapp_numbers (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    phone_e164  TEXT   NOT NULL,           -- normalized, digits + leading country code
    jid         TEXT   NOT NULL,           -- normalized phone-form JID used for lookups
    label       TEXT   NOT NULL DEFAULT '',
    is_primary  BOOLEAN NOT NULL DEFAULT false,
    verified_at TIMESTAMPTZ,               -- set when the number is proven (inbound WA msg)
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (jid)                           -- a number belongs to at most one user
);
CREATE INDEX idx_user_wa_numbers_user ON user_whatsapp_numbers(user_id);
```

`store.UserWhatsAppNumber` struct + CRUD: `ListUserNumbers(userID)`, `GetUserByNumberJID(jid)`, `AddUserNumber`, `SetPrimaryNumber`, `DeleteUserNumber`.

### 5.3 `whatsapp_mappings` — derive personal mappings from user numbers

Keep the table. For **personal** mappings, the `jid` and `user_id` are now derived from a selected `user_whatsapp_numbers` row rather than typed. Optionally add `source_number_id BIGINT REFERENCES user_whatsapp_numbers(id)` to record the link (nullable for group mappings and legacy rows). JIDs are normalized before insert/lookup.

---

## 6. Routing / Resolution Logic

Rework `resolveWhatsAppScope` (`main.go:502-543`) to be deterministic and identity-aware:

1. **Normalize** the lookup JID (LID→phone via `client.Store.LIDs.GetPNForLID`, then `ToNonAD()`) — reuse the resolution already present at `whatsapp.go:381-396`, but apply it to the **mapping lookup**, not only the allowlist. Group key stays `msg.Chat`; personal key becomes the normalized sender JID.
2. **Explicit mapping wins.** `GetWhatsAppMapping(normalizedJID)` → if found, scope to its project/role/user (existing behavior, unchanged semantics).
3. **Known user, no mapping.** For a personal chat, `GetUserByNumberJID(normalizedJID)` → if the number is owned by a user, attribute that `user_id` and route to the **General** project (`GetDefaultProject`).
4. **Unknown number.** For a personal chat, **self-register** (§7): create user + number, attribute it, route to General.
5. **Group / final fallback.** Route to the **General** project (`GetDefaultProject`) instead of `summaries[0]`/project 0. Groups never confer superadmin (unchanged).

The routine fallback (`internal/routine/routine.go:296-304`) is repointed the same way: `GetDefaultProject` instead of first-project.

**Net effect:** the agent "knows which project" from an explicit mapping when one exists, and otherwise acts as an assistant in the **General** project — matching the requested behavior ("General by default … unless it has changed on the web UI," where "changing it" = creating/editing a mapping or reassigning the default project).

---

## 7. Self-Registration Flow

On step 6.4 (unknown personal number, self-registration enabled — see §12 flag):

1. Normalize the sender JID → `phone_e164` + `jid`.
2. Create a `users` row: `name` = WhatsApp push name (`evt.Info.PushName`, fallback to the phone number), `email` = `NULL` or a synthesized placeholder (Open question 14.3), `role = member`, `password_hash` = an **unusable** sentinel (no login until claimed).
3. Auto-provision the user's personal project (reuse `provisionPersonalProject`, `users.go`) so their data has an isolated home if/when a mapping later points there.
4. Insert `user_whatsapp_numbers` (`verified_at = now()` — an inbound message proves ownership, `is_primary = true`).
5. Attribute the current run to the new `user_id`; route to the **General** project by default (per §6.3). No `whatsapp_mappings` row is created automatically — the number stays on the General default until an admin maps it (keeps "unless changed on the web UI" the single override surface).
6. Emit an audit/log event (`self_registered_user`) with the number and new user id.

**Web hand-off.** Because the account already exists, a self-registered number that later opens the web app can be *claimed*: an admin sets a password (existing `forgot-password` email flow needs an email; a WhatsApp-delivered claim code is the cleaner path — deferred, Open question 14.4). Until claimed, the account is WhatsApp-only; its context (memories/notes attributed by `user_id`) is already accumulating.

---

## 8. Web UI / UX

All theme-aware (light + dark), following the existing palette per CLAUDE.md.

- **Set the General project.** A control to mark which project is the default — recommended in **global** settings (superadmin), e.g. alongside `Account.tsx`, or a "Default project" toggle on each project's settings page. Selecting one clears the flag on the previous default (enforced by the partial unique index + `SetDefaultProject`). Show a "General / Default" badge in `ProjectSwitcher.tsx` / `Projects.tsx`.
- **User numbers — self-service.** New "WhatsApp numbers" section on `Profile.tsx`: list the user's numbers (label, primary badge, verified state), add/remove, set primary. A newly added number is `verified_at = NULL` until proven (Open question 14.4 on verification).
- **User numbers — admin.** Same list embedded in the global user editor (`dashboard/global/GlobalUsers.tsx` / `Users.tsx`) so an admin can manage any user's numbers.
- **Mapping create/edit modal** (`WhatsAppMappingsSettings.tsx`): for **kind = personal**, replace the free-text JID field and the raw numeric "Attribute to user id" field with two dropdowns — **User** (searchable) → **Number** (that user's registered numbers). The JID and `user_id` are derived from the selection. **kind = group** keeps the JID field (groups aren't user-owned). Project and role dropdowns unchanged.

---

## 9. API Surface

New / changed endpoints (auth guarding consistent with existing patterns):

| Method &amp; path | Guard | Purpose |
|----------------|-------|---------|
| `GET /api/users/{id}/whatsapp-numbers` | self or superadmin | List a user's numbers |
| `POST /api/users/{id}/whatsapp-numbers` | self or superadmin | Add a number |
| `PATCH /api/users/{id}/whatsapp-numbers/{numId}` | self or superadmin | Set primary / label |
| `DELETE /api/users/{id}/whatsapp-numbers/{numId}` | self or superadmin | Remove a number |
| `GET /api/auth/me/whatsapp-numbers` (+ POST/PATCH/DELETE) | self | Self-service convenience aliases |
| `PATCH /api/projects/{id}` (extend) | superadmin | Accept `is_default: true` to reassign the General project |
| `GET /api/projects/default` | authed | Resolve the current default project |

`whatsapp_mappings` create/update (`api/whatsapp_mappings.go`) accepts a `source_number_id` for personal mappings and derives `jid`/`user_id` server-side; the raw-`user_id` path is deprecated but tolerated for backfill. Shared TS types/client (`packages/shared/src/types.ts`, `packages/shared/src/api/client.ts`) gain `UserWhatsAppNumber` + the calls above.

---

## 10. Migration &amp; Backfill

1. **New migration** (`0000NN_general_project_and_user_numbers.up.sql`):
   - `ALTER TABLE projects ADD is_default` + partial unique index.
   - Create `user_whatsapp_numbers`.
   - (optional) `ALTER TABLE whatsapp_mappings ADD source_number_id`.
   - **Seed / choose the General project:** if a project literally named "General" exists, mark it default; else create one owned by `FirstAdmin` and mark it default. (Fallback: if the deployment relied on `id = 1`, mark `id = 1` default to preserve today's convention.)
   - **Backfill user numbers:** for every existing `whatsapp_mappings` row with `kind='personal'` and `user_id <> 0`, insert a `user_whatsapp_numbers` row (`jid` = normalized mapping JID, `verified_at = now()`), and set `source_number_id`. Skip rows whose `user_id = 0`.
2. **Down migration** drops the additions (numbers table, columns, index). Do not delete the General project on down (data-preserving) — just drop `is_default`.
3. **No data stranded** (per the "migrate data on schema change" rule): existing personal mappings become first-class user numbers; no mapping loses its attribution.

---

## 11. Security, Abuse &amp; Privacy

- **Reply gating vs. self-registration.** Today the agent only *replies* to senders in the `OWNER_JID` allowlist (`whatsapp.go:357-415`). Self-registration means the agent would create accounts for — and potentially reply to — strangers. This is the single biggest policy decision (Open question 14.1). **Recommended default:** self-registration is behind a feature flag (§12) and, when enabled, does **not** automatically widen the reply allowlist — registration captures context silently, while an admin still promotes a number before the agent engages freely. Alternatively, enabling the flag treats "registered users" as allowed senders (public-assistant mode).
- **Abuse controls** (required if replies are opened): per-number rate limiting on registration, a block/deny list, and a cap on new self-registered accounts per hour. Log every `self_registered_user` event.
- **Number ownership integrity.** `jid` is `UNIQUE` in `user_whatsapp_numbers`, so a number can't be claimed by two users. Numbers added via the UI start `unverified`; only an inbound WhatsApp message (or an explicit verification step) sets `verified_at`.
- **Privacy.** Self-registered accounts hold real phone numbers and accumulated context; deleting the user cascades their numbers. Group chats never attribute a personal `user_id` (unchanged) and never confer superadmin.
- **Superadmin surface.** Default-project reassignment and any-user number management are superadmin-only; self-service is scoped to the caller's own account.

---

## 12. Rollout / Feature Flags

- `WA_SELF_REGISTER` (default **off**): gates step 6.4. When off, unknown numbers use the pre-existing fallback (now the General project instead of project 0) with no account created.
- Ship in slices so each is independently reviewable and revertible:
  1. **General project** — schema + `GetDefaultProject` + repoint fallbacks (`main.go`, `routine.go`) + web control. Immediately fixes the project-0 leak.
  2. **User numbers** — table, API, Profile + admin UI, and the mapping-modal dropdowns + backfill.
  3. **Self-registration** — flagged flow + audit + abuse controls.
- Each slice is a separate PR to **staging** with `feature`/`improvement` labels (per CLAUDE.md).

---

## 13. Success Metrics

- **0** runs land on project 0 after slice 1 (trace attribution, `main.go:328`).
- **100%** of inbound personal messages resolve to a real `user_id` (mapped, known-number, or self-registered) once slice 3 is on.
- Mapping creation no longer requires typing a JID or a numeric user id for personal chats (UI audit).
- No duplicate-number rows (enforced) and no cross-user number collisions.
- Time-to-context for a new number: a self-registered user's first message is attributed on message #1, not after manual admin setup.

---

## 14. Risks &amp; Open Questions

1. **Does self-registration open the agent to replying to strangers?** (§11) — the core policy call. Options: (a) silent capture, admin promotes before replies [recommended default]; (b) registered = allowed (public assistant) with abuse controls; (c) registration only for numbers already on the `OWNER_JID`/allowed list. **Needs product decision.**
2. **Default-project representation:** boolean-on-projects + partial unique index (recommended) vs. a `settings.default_project_id` row. Also: is "General" a single global project, or should each owner have their own default? (This app is effectively single-owner via `FirstAdmin`, so global is proposed.)
3. **Email for self-registered users:** `NULL` email (requires making `users.email` nullable / dropping the `UNIQUE(email)` assumption for placeholders) vs. a synthesized placeholder like `<phone>@wa.local`. Affects the `email UNIQUE NOT NULL` constraint (`000001_init.up.sql:11`).
4. **Web claim / verification path:** how a self-registered (password-less) user first logs in — admin-set password, emailed password (needs a real email), or a WhatsApp-delivered claim/OTP code (cleaner, deferred). Also how UI-added numbers get `verified`.
5. **LID normalization coverage:** `GetPNForLID` may not resolve every LID (contact not yet synced). Fallback behavior when a personal JID can't be normalized to a phone — route to General but skip number-based attribution?
6. **Owner's own chats:** the platform superadmin has no personal project by design; confirm the owner's 1:1 chats should land in **General** (proposed) rather than a dedicated owner project.

---

_Related: `docs/prd-multi-project-rbac.md` (roles &amp; membership), `docs/PRD.md` (master product PRD)._
