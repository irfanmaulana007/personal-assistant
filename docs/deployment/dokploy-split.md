# Deploying the split stack on Dokploy

This guide runs the split deployment (from
[PR #226](https://github.com/irfanmaulana007/personal-assistant/pull/226)) on
[Dokploy](https://dokploy.com), where **Dokploy itself builds each service from
the repo** using that service's Dockerfile and **only redeploys the service whose
files changed** (Dokploy *Watch Paths*).

## Repo layout

Every service lives under `app/`, with its Dockerfile beside its source:

```
app/
  api/      Go core API          → app/api/Dockerfile
  web/      React/Vite web app   → app/web/Dockerfile
  mobile/   React Native app*    (reserved) → no web service (EAS/CI)
packages/
  shared/   @personal-assistant/shared — types, utils, API client (consumed by web + mobile)
```

\* Mobile is a **reserved slot** — see [Step 6](#step-6--mobile-reserved-slot).

| Service   | Source dir | Dockerfile           | Dokploy build type | Port | Public? |
| --------- | ---------- | -------------------- | ------------------ | ---- | ------- |
| **api**   | `app/api`  | `app/api/Dockerfile` | Dockerfile         | 8090 | no (internal) |
| **web**   | `app/web`  | `app/web/Dockerfile` | Dockerfile         | 80   | yes     |
| **mobile**| `app/mobile` | — (EAS/Fastlane)   | n/a (not a web service) | — | — |

Plus two datastores the **api** needs: **PostgreSQL** (main data + the
WhatsApp/whatsmeow session) and **MongoDB** (logs).

> **Dockerfiles build from the repo _root_, not the service dir.** The web build
> needs the root `pnpm-lock.yaml`/`pnpm-workspace.yaml` and `packages/shared`
> (workspace build), and the api build needs the root `go.mod`/`go.sum`. So in
> Dokploy the **Docker Context Path is `.`** for both, while the **Docker File**
> points into the service dir (`app/api/Dockerfile`, `app/web/Dockerfile`).

---

## Deployment model: Dokploy builds from Git, per-service Watch Paths

- **Source:** connect the GitHub repo to Dokploy (GitHub App). On every push to
  `main`, Dokploy receives a webhook.
- **Build:** each app uses **Build Type = Dockerfile** and builds its own image
  from its Dockerfile — no GHCR, no GitHub Actions push-to-host.
- **"Only run if the service changed":** each app sets **Watch Paths** (glob
  patterns). Dokploy compares the pushed commit's changed files against them and
  **skips the build unless something under that service changed** — so an
  api-only change never rebuilds web, and vice-versa. This replaces the
  path-filtered GitHub Actions and is the Dokploy-native way to get independent
  deploys.

> Prefer to keep building images in GitHub Actions and have Dokploy just pull
> them? See [Appendix: prebuilt GHCR images](#appendix-prebuilt-ghcr-images).

### Target architecture (same-origin)

```
                       ┌─────────── Dokploy project: personal-assistant ───────────┐
   Internet ──HTTPS──► │  web  (Application, app/web/Dockerfile, :80)               │
   app.example.com     │    │  nginx serves the SPA and proxies /api ──► api:8090   │
                       │    ▼                                                        │
                       │  api  (Application, app/api/Dockerfile, :8090, internal)    │
                       │    ├──► postgres  (Dokploy database)                        │
                       │    └──► mongo     (Dokploy database)                        │
                       └────────────────────────────────────────────────────────────┘
```

**Why web is the only public service:** in the default *same-origin* model the
web image ships an nginx that proxies `/api` to the api service over the internal
Docker network (`BACKEND_URL`). The browser only ever talks to the web origin, so
the api needs **no public domain** and no CORS. WhatsApp QR pairing and login all
happen through the web UI, which reaches the api through that proxy.

> **Cross-origin alternative** (browser calls the api on its own domain): the API
> base URL is **baked into the web bundle at build time** (`VITE_API_BASE_URL`),
> so it's a **Docker build arg** on the web app — see
> [Appendix: cross-origin mode](#appendix-cross-origin-mode). The steps below
> assume same-origin.

---

## Prerequisites

- A running Dokploy server (Traefik + Let's Encrypt, installed by default).
- A DNS `A` record for the web app (e.g. `app.example.com`) → the Dokploy host IP.
- The repo connected to Dokploy via its **GitHub App** integration (Settings →
  Git → GitHub) so Dokploy can clone private code and receive push webhooks with
  changed-file info for Watch Paths.

---

## Step 1 — Create the project

**Projects → Create Project** → `personal-assistant`. Every resource below lives
in this project so they share the internal `dokploy-network` and reach each other
by service name.

---

## Step 2 — Provision the databases

**+ Create Service → Database** for each:

### PostgreSQL
- **Type:** PostgreSQL `17`
- **Database:** `assistant` · **User:** `assistant` · **Password:** generate & save
- Deploy, then copy its **internal host** (e.g. `personal-assistant-postgres-xxxx`).

### MongoDB
- **Type:** MongoDB `7`
- **Root user:** `root` · **Password:** generate & save
- Deploy, then copy its **internal host** (e.g. `personal-assistant-mongo-xxxx`).

Keep both **internal** — only the api connects to them.

---

## Step 3 — API application

**+ Create Service → Application**, name it `api`.

**Provider / Source**
- **GitHub** → this repo, branch **`main`**.

**Build Type → Dockerfile** — the "Docker" fields, exactly:

| Field | Value |
| --- | --- |
| **Docker File** (path) | `app/api/Dockerfile` |
| **Docker Context Path** | `.` |
| **Docker Build Stage** | *(leave empty — the final stage is the runtime image)* |

**Watch Paths** (auto-deploy filter — api has **no** dependency on `packages/shared`, it's Go):
```
app/api/**
go.mod
go.sum
app/api/Dockerfile
```

**Environment** — DSN/URI use the **internal DB hosts** from Step 2 (not `localhost`):
```env
APP_ENV=production
ENCRYPTION_KEY=<openssl rand -base64 32>
WEB_PASSWORD=<password you'll use to log in to the web UI>
OWNER_JID=6281234567890@s.whatsapp.net
LOG_LEVEL=info
POSTGRES_DSN=postgres://assistant:<PG_PASSWORD>@<pg-internal-host>:5432/assistant?sslmode=disable
MONGO_URI=mongodb://root:<MONGO_PASSWORD>@<mongo-internal-host>:27017/?authSource=admin
MONGO_DB=assistant_logs
```
- `ENCRYPTION_KEY` **must** be 32 bytes base64 (`openssl rand -base64 32`).
- `OWNER_JID` — your WhatsApp JID; comma-separate several to allow more numbers
  (the first also gets reminders/recap).
- The **LLM provider/key/model are not env vars** — set them later in the app's
  **Settings** page (stored encrypted in Postgres).

**Ports / Networking**
- Container listens on **8090**. Leave it **internal — no domain**.
- Note the api's assigned **internal service name** (shown in the app settings,
  e.g. `personal-assistant-api-xxxx`) — the web app needs it for `BACKEND_URL`.

**Deploy** and check the logs: it should connect to Postgres/Mongo and serve `:8090`.

---

## Step 4 — Web application

**+ Create Service → Application**, name it `web`.

**Provider / Source**
- **GitHub** → this repo, branch **`main`**.

**Build Type → Dockerfile** — the "Docker" fields:

| Field | Value |
| --- | --- |
| **Docker File** (path) | `app/web/Dockerfile` |
| **Docker Context Path** | `.` |
| **Docker Build Stage** | *(leave empty)* |

**Build Args** — the web bundle's API origin is baked at build time. For
same-origin, leave it **empty**:
```
VITE_API_BASE_URL=
```
(Set it only for the [cross-origin model](#appendix-cross-origin-mode).)

**Watch Paths** (web embeds the shared package, so it watches `packages/shared` too):
```
app/web/**
packages/shared/**
package.json
pnpm-lock.yaml
pnpm-workspace.yaml
app/web/Dockerfile
app/web/nginx.conf.template
```

**Environment** — one runtime variable, pointing nginx's `/api` proxy at the api
service (use the api's internal service name from Step 3):
```env
BACKEND_URL=http://<api-internal-service-name>:8090
```
(The compose default is `http://api:8090` once you rename the service; substitute
Dokploy's actual assigned name.)

**Ports / Domain**
- Container port **80**.
- Add a **Domain**: `app.example.com`, container port `80`, enable **HTTPS**
  (Let's Encrypt). Dokploy issues the cert via Traefik.

**Deploy**, then open `https://app.example.com` — you should get the login page.

---

## Step 5 — First boot

1. Log in at `https://app.example.com` with `WEB_PASSWORD`.
2. **Integrations → WhatsApp** → scan the QR to pair. The whatsmeow session lives
   in Postgres, so it survives redeploys (containers are stateless — all state is
   in the databases).
3. **Settings** → configure the LLM provider, API key, and model.

---

## Step 6 — Mobile (reserved slot)

`app/mobile` is a **React Native** app: it ships to the App Store / Play Store via
**EAS or Fastlane in CI — it is not a long-running web service, so there is no
Dokploy application and no "Docker" field to fill for it.** The build pipeline is
`.github/workflows/deploy-mobile.yml`, path-filtered on `app/mobile/**` (and
`packages/shared/**`).

The only case where mobile gets a Dokploy service is if you serve an **Expo web
export** (a static web build). If so, set it up exactly like the web app:

| Field | Value |
| --- | --- |
| **Build Type** | Dockerfile |
| **Docker File** | `app/mobile/Dockerfile` *(a small nginx-serves-`dist` image, like web)* |
| **Docker Context Path** | `.` |
| **Watch Paths** | `app/mobile/**`, `packages/shared/**` |
| **Domain** | e.g. `m.example.com`, port `80`, HTTPS |

Otherwise, leave mobile out of Dokploy entirely.

---

## Deploy & redeploy order

- **First time:** databases → api → web (api must be up before web can proxy to
  it; web still starts if api is briefly down and recovers once reachable).
- **Ongoing:** each app rebuilds **only when its Watch Paths match** the pushed
  commit — so services deploy independently. Redeploys are safe any time: the
  containers are stateless, no migration step, all state is in Postgres/Mongo.

---

## Verifying / troubleshooting

| Symptom | Likely cause / fix |
| --- | --- |
| Push didn't trigger a build | Changed files don't match that app's **Watch Paths**, or the repo isn't connected via the GitHub App (no webhook). Check the app's Deployments tab. |
| Build fails on `COPY` / lockfile not found | **Docker Context Path** isn't `.` (repo root), or the Dockerfile's internal `COPY` paths weren't updated to the `app/` layout. |
| Login page loads but API calls 502/504 | `BACKEND_URL` points at the wrong service name, or api isn't up. Confirm the api's internal name and that it listens on 8090. |
| api crashes on boot | Bad `POSTGRES_DSN` / `MONGO_URI` (wrong internal host or password) or `ENCRYPTION_KEY` not 32-byte base64. Check api logs. |
| Chat responses don't stream | The `/api` proxy disables buffering for SSE (`proxy_buffering off`). If you front web with another proxy, don't re-buffer `/api`. |
| TLS not issued | DNS `A` record not pointing at the Dokploy host yet, or ports 80/443 blocked. |

---

## Appendix: cross-origin mode

To have the browser call the api on its own public origin (e.g. `api.example.com`)
instead of proxying through web:

1. Give the **api** app a public **domain** (`api.example.com`, port 8090, HTTPS)
   and allow CORS from the web origin in the api.
2. On the **web** app, set the **Build Arg** `VITE_API_BASE_URL=https://api.example.com`
   and **redeploy web** (it's baked into the bundle at build time — a rebuild is
   required; it is not a runtime env var).
3. The web app's nginx `/api` proxy (`BACKEND_URL`) is then unused; leaving it set
   does no harm.

Same-origin (the main guide) is simpler — one public domain, no CORS — and is the
recommended default.

---

## Appendix: prebuilt GHCR images

If you'd rather keep building images in **GitHub Actions** (the original PR #226
model) and have Dokploy only pull them:

1. Keep `deploy-backend.yml` / `deploy-web.yml` building & pushing to GHCR — but
   **update their path filters** to the new layout (`app/api/**` + `go.mod`/`go.sum`
   for backend; `app/web/**` + `packages/**` for web).
2. In each Dokploy app, set **Provider → Docker Image**
   (`ghcr.io/<owner>/personal-assistant/{api,web}:latest`) instead of GitHub +
   Dockerfile, and add a **GHCR registry credential** (a PAT with `read:packages`,
   since the images are private).
3. Complete each workflow's deploy-to-host TODO by `curl`-ing the app's Dokploy
   **redeploy webhook** after the push:
   ```yaml
       - name: Trigger Dokploy redeploy
         run: curl -fsSL -X POST "${{ secrets.DOKPLOY_API_WEBHOOK }}"
   ```

Trade-off: here the per-service filtering lives in GitHub Actions rather than
Dokploy Watch Paths. The main guide (Dokploy builds from Git) keeps everything in
one place and needs no GHCR or webhook plumbing.
