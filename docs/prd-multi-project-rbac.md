# PRD — Multi-Project Support with RBAC

**Status:** Draft for review · **Trello:** https://trello.com/c/FtZYzz7F ·
**Branch:** `feat/multi-project-rbac` · **Target:** `staging`

> ⚠️ **This supersedes a standing non-goal.** `docs/PRD.md` lists *"Multi-user /
> multi-tenant support"* as an explicit **non-goal** ("this is a personal
> tool, not a multi-tenant service"). This feature intentionally reverses that:
> the app grows from a single flat `user_id` workspace into a multi-project,
> role-based system. Treat this document as the authoritative scope for that
> shift.

---

## 1. Problem & Goal

Today every row in the database is scoped by a single flat `user_id`
(`app/api/internal/store/postgres_*.go`), and authorization is a single global
`role` string on the user — `"admin"` or `"member"` — enforced by one
`requireAdmin` middleware (`app/api/internal/api/middleware.go:50`). There is no
concept of a project, workspace, or tenant anywhere in the codebase, and skills
are toggled per **user** (`user_skills`), not per anything larger.

We need to support **multiple projects per user**, with **Role-Based Access
Control** so that:

- **Superadmins** have unrestricted access to every project and all platform
  settings.
- **Admins** manage the membership and skill/feature configuration of the
  specific projects they are assigned to.
- **Members** can only access projects they were invited to, and can only use
  the skills that are enabled there.

Additionally, each project gets a **feature-management layer**: navigation
menus / modules are modelled as *features*, each feature owns zero or more
skills, and **disabling a feature cascades to disable all of its skills** for
that project.

## 2. Goals / Non-Goals

### Goals (maps 1:1 to the card's 12 acceptance criteria)

1. Users can create and manage multiple projects from a single dashboard.
2. Global skills are active across all projects by default.
3. Admins can enable/disable specific skills per project.
4. Superadmin role has unrestricted access to all projects and settings.
5. Admin role can manage team membership and skill config for assigned projects.
6. Member role can only access projects they are invited to and use enabled skills.
7. RBAC enforcement on **all** API endpoints **and** skill execution.
8. Audit log tracks all project-level actions (create, invite, skill toggle).
9. Each project supports feature management; menus/nav items are features.
10. Each feature holds zero or more skills.
11. Disabling a feature auto-disables all skills under it.
12. Existing users are migrated to a single personal project with admin role.

### Non-Goals (explicitly out of scope for this card)

- Billing / plans / per-seat pricing.
- Cross-project data sharing — every domain record lives in exactly one project;
  there is no moving/sharing a record between projects.
- Email-based invitation delivery (invites add an **existing** user by email;
  no outbound email / signup flow).
- SSO / external identity providers.
- Changing the hand-rolled JWT into a session/refresh-token system.

## 3. Current State (grounded)

| Concern | Today | File |
| --- | --- | --- |
| Routing | stdlib `net/http.ServeMux`, all routes in one place | `api/server.go:92-166` |
| AuthZ | `protect` (any user) / `admin` (`requireAdmin`, `role=="admin"`) | `api/middleware.go:50` |
| Identity | HS256 JWT, claims `{Sub,Email,Role}` | `api/auth.go:26` |
| User role | free-text `role` col, `'admin'`/`'member'` | `store/migrations/postgres/000001_init.up.sql:15` |
| DB access | raw SQL via `pgx/v5`, no ORM, no FKs | `store/postgres_*.go` |
| Migrations | golang-migrate, embedded, `NNNNNN_name.{up,down}.sql`, next = **000006** | `store/migrations/postgres/` |
| Skills | catalog `skills` + per-user `user_skills`, effective = `COALESCE(us.enabled, s.default_enabled)` | `store/postgres_skills.go:92` |
| Skill exec | agent reads `authctx.UserID`, `skills.Enabled(ctx,uid)`, gates tools | `agent/agent.go:210,226` |
| Logs/audit | logs live in **MongoDB** (`LogStore`); **no** user-action audit trail | `store/mongo.go:27` |
| Client auth | `useAuth()` → `isAdmin = role==='admin'`, threaded via props (no context) | `hooks/useAuth.ts:82` |
| Client nav | data-driven `navItems[]`, `adminOnly` filter | `components/Layout.tsx:54,219` |
| Client skills | `Skills.tsx` toggles via `setSkillEnabled`, returns full list | `components/Skills.tsx:180` |

## 4. Roles & Permission Model

Two role *scopes*:

- **Global role** (`users.role`) — platform-wide. Values become
  **`superadmin`** | **`member`**. `superadmin` = unrestricted (all projects +
  all platform settings). This replaces today's global `"admin"`.
- **Project role** (`project_members.role`) — per-project. Values
  **`admin`** | **`member`**.

> **Terminology shift:** the words *admin* and *member* now denote
> **project-scoped** roles. The old global god-role is renamed **superadmin**.
> Existing global `admin` users migrate to `superadmin`; existing platform
> admin-gated endpoints (settings, users, pricing, logs, integrations,
> whatsapp, routines) become **superadmin-only**.

### Permission matrix

| Action | superadmin | project admin | project member | non-member |
| --- | :--: | :--: | :--: | :--: |
| Access platform settings / user mgmt / pricing / logs | ✅ | ❌ | ❌ | ❌ |
| List projects | all | assigned | assigned | — |
| **Create a project** | ✅ | ❌ | ❌ | ❌ |
| **Appoint a project `admin`** (add admin / promote to admin) | ✅ | ❌ | ❌ | ❌ |
| View a project's data | any | own | own | ❌ |
| Rename / delete a project | any | own | ❌ | ❌ |
| Add / remove **members** (role `member`) | any | own | ❌ | ❌ |
| Toggle a project's skills | any | own | ❌ | ❌ |
| Toggle a project's features | any | own | ❌ | ❌ |
| Use enabled skills (chat / execution) | any | own | own | ❌ |
| View a project's audit log | any | own | ❌ | ❌ |

**Creation & admin appointment are superadmin-only.** Only a superadmin can
create a project and only a superadmin can grant the project-`admin` role
(when creating the project they name its admin, and thereafter they are the
only one who can add another admin or promote a member to admin). A project
**admin** manages the project day-to-day — add/remove **members**, toggle
skills & features — but **cannot** create projects or appoint/other admins.

> **Re criterion 1** ("Users can create and manage multiple projects from a
> single dashboard"): with this rule, *creation* is a superadmin action from
> the dashboard; project admins/members *manage* the projects they belong to.
> The dashboard is shared; the create affordance is superadmin-gated.

**Creator rule:** a project is created by a superadmin, who assigns the initial
project `admin` at creation time (that user is inserted into `project_members`
with role `admin`).

### Provisioning & global-role rules (resolved)

- **Granting `superadmin` is superadmin-only.** The global role
  (`users.role ∈ {superadmin, member}`) is edited on the existing Account /
  user-management screen, which now offers `superadmin`/`member` and is
  superadmin-gated (it already flips to superadmin-only per §4). Only a
  superadmin can mint another superadmin.
- **New users get a personal project automatically.** When a superadmin creates
  a user (Account screen / `POST /api/users`), the server also creates a
  personal project owned by that user and inserts them as its project `admin` —
  mirroring the migration backfill so a freshly-created user is never stranded
  with zero projects. A `member` still only sees projects they own or are
  invited to.
- **Deleting a project** removes its `project_members`, `project_skills`,
  `project_features`, `whatsapp_mappings`, and all domain rows scoped to it
  (hard delete). A user's last/personal project cannot be deleted out from under
  them by a non-superadmin (project admins can delete only projects they admin;
  the guard against orphaning is that domain rows are removed with the project).

## 5. Data Model

New Postgres tables (migration `000006`). IDs follow the house style
(`BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY`); no FK constraints, indexed
scoping columns — mirroring the existing schema.

```
projects
  id, name, owner_user_id, created_at
  idx(owner_user_id)

project_members                     -- who can access a project, and as what
  project_id, user_id, role('admin'|'member'), created_at
  PK(project_id, user_id)
  idx(user_id)

project_skills                      -- per-project skill override (like user_skills)
  project_id, skill_id, enabled
  PK(project_id, skill_id)

features                            -- catalog of nav/menu modules (code-seeded, like skills)
  id, key(unique), name, description, sort_order, default_enabled, created_at

feature_skills                      -- which skills belong to a feature (0..n)
  feature_id, skill_id
  PK(feature_id, skill_id)

project_features                    -- per-project feature override
  project_id, feature_id, enabled
  PK(project_id, feature_id)
```

Audit lives in **MongoDB** (respecting "logs live in Mongo"): a new
`audit_log` collection via `LogStore`, documents
`{ id, project_id, actor_user_id, actor_email, action, target, metadata, created_at }`.

### Domain-data isolation (full per-project scoping)

Beyond the tables above, **every existing user-owned domain table gains a
`project_id`** and all reads/writes re-scope from `WHERE user_id = $1` to
`WHERE project_id = $1` (retaining `user_id` as the *creator* stamp). Tables
touched:

`contacts`, `bucket_list_items` (formerly `life_goals`), `trips`,
`trip_expenses`, `hike_mountains`, `hike_tracks`, `hike_participants`, `hikes`
(`hike_hikers` inherits scope via its parent hike), `reminders`, `memories`,
`notes`, and the Mongo `activities` collection (gains a `project_id` field).

**Intentional exception:** `user_personas` stays **user-level** — it is the
assistant's style preference for a person, not project data, so it is not
re-scoped.

Convention: each PG table gets `project_id BIGINT NOT NULL DEFAULT 0` +
`CREATE INDEX idx_<t>_project ON <t>(project_id)`; store reads/writes scope by
the active project from `authctx.ProjectID(ctx)` (threaded via context, like
the existing `authctx.UserID`) so method signatures are unchanged. Background
paths (the reminder scheduler) carry no active project and remain
owner-scoped. Records live in **exactly one** project.

### Entity relationships

```
users ──< project_members >── projects ──< project_skills >── skills
                                  │                              │
                                  └──< project_features >── features ──< feature_skills >── skills
```

### Effective-skill resolution (the core query)

For a given project, a skill is **enabled** iff:

```
effective_enabled(project, skill) =
    feature_enabled(project, skill.feature)          -- feature gate (cascade)
    AND COALESCE(project_skills.enabled, skills.default_enabled)   -- per-project override, else global default
```

where a skill with **no** feature is only gated by the second line, and

```
feature_enabled(project, feature) = COALESCE(project_features.enabled, features.default_enabled /* true */)
```

This satisfies criteria 2 (global default flows to all projects), 3/5 (admins
override per project), 11 (feature off ⇒ its skills off).

## 6. API Surface

All new endpoints are project-scoped and RBAC-guarded by new middleware
(§8). Existing platform routes flip from `admin` (any global admin) to
**superadmin**.

| Method & path | Guard | Purpose |
| --- | --- | --- |
| `GET /api/projects` | authed | List projects visible to caller |
| `POST /api/projects` | **superadmin** | Create project + name its initial admin |
| `GET /api/projects/{id}` | member+ | Project detail |
| `PATCH /api/projects/{id}` | proj-admin | Rename |
| `DELETE /api/projects/{id}` | proj-admin | Delete project + its scoped rows |
| `GET /api/projects/{id}/members` | member+ | List members |
| `POST /api/projects/{id}/members` | proj-admin (role=member) / **superadmin** (role=admin) | Add existing user by email; a project admin may only add **members**, appointing an `admin` requires superadmin |
| `PATCH /api/projects/{id}/members/{userId}` | proj-admin (to/from member) / **superadmin** (any change to/from admin) | Change member role — promoting to or demoting from `admin` is superadmin-only |
| `DELETE /api/projects/{id}/members/{userId}` | proj-admin | Remove member (a project admin cannot remove another admin) |
| `GET /api/projects/{id}/skills` | member+ | Effective skills for project |
| `PUT /api/projects/{id}/skills/{skillId}` | proj-admin | Enable/disable a skill |
| `GET /api/projects/{id}/features` | member+ | Features + enabled state + attached skills |
| `PUT /api/projects/{id}/features/{featureId}` | proj-admin | Toggle feature (cascades to skills) |
| `GET /api/projects/{id}/audit` | proj-admin | Project audit log |
| `GET /api/metrics/usage` | member+ | Usage metrics **for the active project** (existing endpoint, now project-scoped) |
| `GET /api/admin/overview` | **superadmin** | Cross-project usage & metrics overview; optional `?projectId=` filter |

**Current project selection.** The client sends the active project via an
`X-Project-Id` header. Middleware resolves it once, looks up the caller's
membership/role, and stores `projectID` + `projectRole` in context
(`authctx`). The existing `/api/skills` list/toggle endpoints are repointed to
operate on the **current project** (from the header), so the existing Skills
page keeps working but is now project-scoped and mutation-gated to project
admins. Chat (`POST /api/chat`) also reads `X-Project-Id` so skill execution is
evaluated against the active project, and per-project data (contacts, notes,
reminders, …) is served for the active project.

**Cross-project metrics.** Logs/traces in Mongo (`traces`, `message_log`,
`tool_usage`) gain a `project_id` field written at record time. The existing
`GET /api/metrics/usage` filters by the active project; the new superadmin
`GET /api/admin/overview` aggregates across **all** projects (per-project
breakdown table + totals) and accepts an optional `projectId` filter.

## 6a. WhatsApp → Project / Role mapping

The agent runs on WhatsApp as well as the web. **Which project (and role) the
agent acts as is decided by where the message came from**, not by a logged-in
session:

- **Group chats are mapped to a project.** A WhatsApp **group JID** is mapped to
  a `project_id`; every message in that group runs the agent scoped to that
  project — its enabled skills/features and its data. Group1 → Project A,
  Group2 → Project B. A group message **never** confers superadmin, regardless
  of who sent it (role is clamped to at most `admin`/`member` of the mapped
  project).
- **Personal (1:1) chats are mapped to a project + role.** A **personal phone
  number / JID** maps to a `project_id` and a role. If that role is
  **`superadmin`**, the agent acts with full access to **all** skills in the 1:1
  chat — this is the owner's private control channel. Superadmin is honoured
  **only** in personal chat.
- **Unmapped chats:** fall back to a configured default project (the owner's
  personal project) with `member` scope, or are ignored per the existing
  owner-number gate — resolved once the inbound seam is confirmed.

**Today (grounded):** the WhatsApp path attributes **every** message to a single
owner user — `db.FirstAdmin(ctx)` → `authctx.WithUserID(uctx, owner.ID)` at
`cmd/assistant/main.go:218-224` — and there is no project/role concept. Inbound
events already expose everything we need: `msg.Chat` (group JID), `msg.From`
(sender phone/JID), and `msg.IsGroup` (built in
`transport/whatsapp/whatsapp.go:455-465`). A group-mention gate + allow-list
already decide whether to respond (`whatsapp.go:395-431`).

**Storage:** `whatsapp_mappings(jid, kind['group'|'personal'], project_id, role,
user_id, label)` (migration `000006`). (An alternative that needs no table is the
existing per-chat **settings-key** pattern — `translate_group_<jid>` in
`settings.go:471-541` — but a dedicated table is cleaner for the management UI
and superadmin listing.)

**Seam:** in the WhatsApp handler closure in `cmd/assistant/main.go`, right after
`uctx := authctx.WithUserID(ctx, userID)` (`main.go:224`) — where `msg.Chat`,
`msg.From`, and `msg.IsGroup` are all in scope — resolve the mapping and wrap the
ctx:
- `msg.IsGroup` → look up `whatsapp_mappings` by **group JID** (`msg.Chat`),
  `kind='group'`; set `WithProjectID(mapping.project_id)` and
  `WithProjectRole` **clamped to `member`/`admin`** (never superadmin from a
  group).
- DM → look up by **sender JID** (`msg.From`), `kind='personal'`; set
  `WithProjectID` + `WithProjectRole` (may be `superadmin`), and, if the mapping
  names a `user_id`, `WithUserID(mapping.user_id)` so the DM is attributed to
  that user instead of `FirstAdmin`.
- Unmapped **group** → not responded to (a group must be explicitly mapped to
  become active), preserving today's default-quiet behaviour in groups.
- Unmapped **personal** chat from an allow-listed/owner number → falls back to
  the **owner's personal project** with the owner's role, so the owner's 1:1
  chat keeps working exactly as today. Non-allow-listed senders are dropped by
  the existing gate.

The updated ctx then flows into `groupTranslator.Handle` (`main.go:231`) and
`assistant.Run` (`main.go:266`) unchanged; the agent's skill-resolution and data
access scope to that project automatically because they read
`authctx.ProjectID`.

**Management UI:** a superadmin screen (Settings → WhatsApp, next to the existing
`WhatsAppSettings`) to list/create/edit mappings — pick a group or enter a
number, choose the project, and (personal only) the role. Mirrors the existing
`Account.tsx`/`WhatsAppSettings` patterns.

**APIs:** `GET/POST/PATCH/DELETE /api/whatsapp/mappings` (superadmin-only),
alongside the existing `/api/whatsapp*` admin routes.

## 7. Migration & Backfill (criterion 12)

Migration `000006_multi_project_rbac.up.sql`:

1. Create the six tables above.
2. **Global-role rename:** `UPDATE users SET role='superadmin' WHERE role='admin';`
3. **Personal project per user** (backfilled in pure SQL):
   ```sql
   INSERT INTO projects (name, owner_user_id)
     SELECT COALESCE(NULLIF(name,''), email) || ' — Personal', id FROM users;
   INSERT INTO project_members (project_id, user_id, role)
     SELECT p.id, p.owner_user_id, 'admin' FROM projects p;
   ```
4. **Carry over existing skill config:** migrate each user's `user_skills`
   overrides into `project_skills` on their new personal project, so nobody's
   current skill setup regresses (per the repo's *migrate-data-on-schema-change*
   rule).
   ```sql
   INSERT INTO project_skills (project_id, skill_id, enabled)
     SELECT p.id, us.skill_id, us.enabled
       FROM user_skills us JOIN projects p ON p.owner_user_id = us.user_id;
   ```
5. **Domain-data backfill:** add `project_id` to every domain table (§5) and
   point each existing row at its owner's personal project:
   ```sql
   ALTER TABLE contacts ADD COLUMN project_id BIGINT NOT NULL DEFAULT 0;
   UPDATE contacts c SET project_id = p.id
     FROM projects p WHERE p.owner_user_id = c.user_id;
   CREATE INDEX idx_contacts_project ON contacts(project_id);
   -- …repeated for life_goals, trips, trip_expenses, hike_*, reminders,
   --    memories, notes, bucket_list, user_personas
   ```
   Because a single migration runs top-to-bottom, the `projects` rows from
   step 3 already exist when these `UPDATE`s run.

`features` + `feature_skills` are **code-seeded on boot** (a `seedFeatures`
mirroring `seedSkills` in `store/seed.go` / `postgres_skills.go`), so the
feature catalog + skill attachments version with the code, not the migration.
`down.sql` drops the new tables, drops the `project_id` columns, and reverts
`superadmin → admin`.

## 8. Enforcement

- **New middleware** in `api/middleware.go`:
  - `requireSuperadmin` — replaces/extends `requireAdmin`; global `role == 'superadmin'`.
  - `withProject` — resolves `X-Project-Id`, verifies caller is a member (or
    superadmin), injects `projectID`/`projectRole` into context; `403`/`404`
    otherwise.
  - `requireProjectAdmin` — layered on `withProject`; requires project role
    `admin` (superadmin always passes). Used for rename/delete, member
    add/remove, skill & feature toggles.
  - **Admin-appointment guard** — creating a project, and any member mutation
    that grants or revokes the project `admin` role, additionally requires
    global `superadmin`. A project admin passing `role=admin` gets `403`.
- **`authctx`** gains `WithProjectID` / `ProjectID` / `WithProjectRole` /
  `ProjectRole` so non-`api` layers (agent, capabilities) stay project-aware
  without importing `api`.
- **Skill execution:** `agent.go` resolves the active project from context and
  computes enabled skills via the effective-skill query (§5) instead of
  `user_skills`; a non-member gets no skills / is rejected. This closes
  criterion 7's "…and skill execution".
- **Audit hooks:** project create, member invite/remove/role-change, and skill
  toggle each `LogStore.RecordAudit(...)`.

## 9. Frontend

- **Project context:** a new `ProjectContext` (modelled on
  `contexts/PreferencesContext.tsx`) holds the list of the user's projects, the
  **active project**, and a setter that persists the choice to `localStorage`
  and sets the `X-Project-Id` header default in `api/client.ts`. Every page
  reads the active project from this context, so switching re-scopes the whole
  app.
- **Project switcher pinned at the top of the navigation** (Dokploy-style) —
  the first element in the sidebar (`Layout.tsx`), above the menu, always
  visible on every page. A Radix `Popover` (matching `UserMenu.tsx`) shows the
  active project, lets the user switch projects, and — for superadmins — offers
  **＋ New project**. Switching updates `ProjectContext` (and the
  `X-Project-Id` header) app-wide. Light+dark styled to match the `slate-*`
  sidebar.
- **Projects dashboard** (`components/Projects.tsx`, new nav entry) — list for
  everyone; the **Create project** affordance (and naming its initial admin) is
  **superadmin-only**; rename/delete for project admins. Mirrors
  `Reminders.tsx`/`Account.tsx` patterns (Modal forms, Toggle, skeletons, both
  light+dark per CLAUDE.md).
- **Members tab** — add existing user by email. A **project admin** sees only
  the **member** role option; the **admin** role option in the `<select>` is
  superadmin-only. Change-role / remove follow the same gating (mirrors
  `Account.tsx`'s `UsersCard`).
- **Per-project skills** — the existing `Skills.tsx` page, now scoped to the
  active project; toggles gated to project admins (`disabled={!canManage}`).
- **Feature management** — a section listing features with a Toggle each;
  toggling a feature off visibly disables (and greys) its child skills.
- **Audit log view** — a simple table for project admins.
- **Superadmin cross-project dashboard** — a new **All Projects** overview
  (superadmin-only, under the existing Dashboard nav group / `/dashboard`
  tree). Shows per-project usage & metrics (messages, tokens, cost, active
  skills) as a breakdown table + summary cards + Recharts visualizations (colors
  from `useChartTheme()` per CLAUDE.md), with a **project filter** dropdown to
  drill into one project. Backed by `GET /api/admin/overview`.
- **Role plumbing:** `useAuth`/props expand from binary `isAdmin` to expose the
  effective capability in the active project (`canManageProject`) plus the
  global `isSuperadmin`.

## 10. Phasing (single branch / single PR, ordered commits)

Per the Trello workflow this is **one card → one branch → one PR**. The work is
sequenced so the diff is reviewable commit-by-commit:

1. **Schema & migration** — `000006`, structs in `store.go`, `DataStore`
   interface methods, `postgres_projects.go`, feature seed, and the
   domain-table `project_id` backfill. + store round-trip & cross-project
   isolation tests (mirror `bucketlist_test.go`).
2. **RBAC middleware & authctx** — superadmin rename, `withProject`,
   `requireProjectAdmin`, admin-appointment guard; flip platform routes to
   superadmin.
3. **Project/member/skill/feature/audit APIs** — handlers + routes + DTOs.
4. **Domain re-scoping** — repoint every `postgres_*.go` + `api/*.go` +
   `capability/*.go` from `user_id` to the active `project_id`
   (`authctx.ProjectID`).
5. **Agent skill-execution scoping** + audit hooks + `project_id` on Mongo
   traces/logs.
6. **Frontend** — context, top-of-nav switcher, projects dashboard, members,
   per-project skills, feature mgmt, audit view, and the superadmin
   cross-project overview dashboard (light+dark verified).
7. **Docs** — this PRD + update `docs/PRD.md` non-goal note.

## 11. Testing

- **Store unit/integration** (`-tags integration`, testcontainers): project CRUD
  round-trip; membership; effective-skill resolution incl. feature cascade;
  backfill migration produces one admin-membership per existing user; skill
  config carry-over.
- **Cross-project isolation** test (the `bucketlist_test.go:116` pattern):
  member of project A cannot read/mutate project B.
- **Pure handler-logic tests** for role checks / DTO validation (matches the
  repo's existing pure-logic handler tests).
- **Build/lint gates:** `make build`, `make lint`, `make test` green before PR.

## 12. Risks & Open Questions

- **Blast radius.** Renaming the global role and flipping every platform route
  to superadmin touches auth broadly; a wrong turn locks out admins. Mitigated
  by the migration `admin→superadmin` + keeping superadmin a strict superset.
- **Full data isolation is the largest risk (decided in scope).** Every domain
  table gets `project_id` and every store/handler/capability re-scopes to the
  active project. Mitigations: `project_id BIGINT NOT NULL DEFAULT 0` +
  deterministic backfill so no row is orphaned; cross-project isolation tests on
  every domain; the active project always resolved server-side from
  `X-Project-Id` (never trusted from the row's `user_id`).
- **One large PR (decided).** Delivered as a single reviewable PR, sequenced by
  the §10 phases so it reads commit-by-commit. `make build`/`lint`/`test` gate
  the PR.
- **Chat/agent must always carry a project.** Every chat + capability call now
  needs an active project; the client always sends `X-Project-Id`, and the
  server falls back to the caller's default/personal project if the header is
  absent so no request is left project-less.
- **JWT staleness.** Project roles are resolved per-request from the DB (not
  baked into the token), so role changes take effect immediately — no token
  refresh needed. Intentional.
