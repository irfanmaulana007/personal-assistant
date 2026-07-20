# Deploying the split stack on Dokploy

This guide maps the split deployment introduced in
[PR #226](https://github.com/irfanmaulana007/personal-assistant/pull/226) onto
[Dokploy](https://dokploy.com). After PR #226 the repo builds **two independent
images** and pushes them to GHCR:

| Service     | Image                                              | Port | Public? |
| ----------- | -------------------------------------------------- | ---- | ------- |
| **Backend** | `ghcr.io/<owner>/personal-assistant/backend:latest` | 8090 | no (internal only) |
| **Web**     | `ghcr.io/<owner>/personal-assistant/web:latest`     | 80   | yes     |

Plus two datastores the backend needs: **PostgreSQL** (main data + the
WhatsApp/whatsmeow session) and **MongoDB** (logs).

The GitHub deploy workflows (`deploy-backend.yml`, `deploy-web.yml`) already
**build and push** the per-service images to GHCR. Their final "deploy-to-host"
step is a marked TODO — **Dokploy is that host.** This guide sets up the four
Dokploy resources and then wires the workflows to trigger a redeploy.

---

## Target architecture on Dokploy

```
                       ┌─────────────────────── Dokploy project: personal-assistant ──┐
   Internet ──HTTPS──► │  web  (Application, GHCR web image, :80)                      │
   app.example.com     │    │  nginx serves the SPA and proxies /api ──► backend:8090 │
                       │    ▼                                                          │
                       │  backend  (Application, GHCR backend image, :8090, internal)  │
                       │    ├──► postgres  (Dokploy database)                          │
                       │    └──► mongo     (Dokploy database)                          │
                       └──────────────────────────────────────────────────────────────┘
```

**Why the web app is the only public service:** in the default *same-origin*
model the web image ships an nginx that proxies `/api` to the backend over the
internal Docker network (`BACKEND_URL`). The browser only ever talks to the web
origin, so the backend needs **no public domain** and no CORS config. The
WhatsApp QR pairing and login all happen through the web app's UI, which calls
the API through that proxy — so nothing else needs to be exposed.

> **Cross-origin alternative.** If you would rather have the browser call the
> backend directly, the API base URL must be **baked into the web bundle at build
> time** (`VITE_API_BASE_URL`), which happens in GitHub Actions, *not* in
> Dokploy — see [Cross-origin mode](#appendix-cross-origin-mode) at the end. The
> steps below assume the recommended same-origin model.

---

## Prerequisites

- A running Dokploy server (with Traefik + Let's Encrypt, which Dokploy installs
  by default).
- A DNS `A` record for the web app (e.g. `app.example.com`) pointing at the
  Dokploy server's public IP.
- The GHCR images have been pushed at least once — merge to `main` (or run the
  workflows via **workflow_dispatch**) so `.../backend:latest` and `.../web:latest`
  exist.
- A GitHub **Personal Access Token** with `read:packages` scope to let Dokploy
  pull the images (GHCR images are private by default).

---

## Step 1 — Create the project

In Dokploy: **Projects → Create Project**, name it `personal-assistant`. Every
resource below lives inside this project so they share the internal
`dokploy-network` and can reach each other by service name.

---

## Step 2 — Provision the databases

Create both as Dokploy-managed databases (**+ Create Service → Database**):

### PostgreSQL
- **Type:** PostgreSQL (use `17` to match the compose files).
- **Database name:** `assistant`
- **User:** `assistant`
- **Password:** generate a strong one — save it.
- Deploy it, then open the database and copy its **internal connection host**
  (Dokploy shows an internal host such as `personal-assistant-postgres-xxxx`).

### MongoDB
- **Type:** MongoDB (`7`).
- **Root user:** `root`
- **Password:** generate a strong one — save it.
- Deploy it and copy the **internal host** (e.g. `personal-assistant-mongo-xxxx`).

> You do **not** need to expose either database to the internet. Keep them
> internal; only the backend connects to them.

---

## Step 3 — Add GHCR registry credentials

So Dokploy can pull the private images: **Settings → Registry → Add Registry**
(or the "Docker Registry" section):

- **Registry URL:** `ghcr.io`
- **Username:** your GitHub username
- **Password:** the `read:packages` PAT from prerequisites.

---

## Step 4 — Backend application

**+ Create Service → Application**, name it `backend`.

1. **Source → Docker Image**
   - Image: `ghcr.io/<owner>/personal-assistant/backend:latest`
   - Select the GHCR registry credential from Step 3.
2. **Environment** — add these (values from Steps 2 and your secrets). Note the
   DSN/URI use the **internal database hosts** from Step 2, not `localhost`:

   ```env
   APP_ENV=production
   ENCRYPTION_KEY=<openssl rand -base64 32>
   WEB_PASSWORD=<the password you will use to log in to the web UI>
   OWNER_JID=6281234567890@s.whatsapp.net
   LOG_LEVEL=info
   POSTGRES_DSN=postgres://assistant:<PG_PASSWORD>@<pg-internal-host>:5432/assistant?sslmode=disable
   MONGO_URI=mongodb://root:<MONGO_PASSWORD>@<mongo-internal-host>:27017/?authSource=admin
   MONGO_DB=assistant_logs
   ```

   - `ENCRYPTION_KEY` **must** be 32 bytes base64 (`openssl rand -base64 32`).
   - `OWNER_JID` — your WhatsApp JID; comma-separate several to allow more numbers
     (the first also gets reminders/recap).
   - The LLM provider/key/model are **not** env vars — they're configured later in
     the app's **Settings** page and stored encrypted in Postgres.
3. **Ports / Networking**
   - The container listens on **8090**. Leave it internal — **do not add a domain**.
     Dokploy keeps it reachable inside the project network by its service name.
   - Note the backend's internal **service name / app name** (Dokploy shows it in
     the app's settings, e.g. `personal-assistant-backend-xxxx`) — you need it for
     the web app's `BACKEND_URL` in the next step.
4. **Deploy.** Check the logs — it should connect to Postgres/Mongo and start
   serving on `:8090`.

---

## Step 5 — Web application

**+ Create Service → Application**, name it `web`.

1. **Source → Docker Image**
   - Image: `ghcr.io/<owner>/personal-assistant/web:latest`
   - Same GHCR registry credential.
2. **Environment** — one variable, pointing the nginx `/api` proxy at the backend:

   ```env
   BACKEND_URL=http://<backend-internal-service-name>:8090
   ```

   Use the backend's internal service name from Step 4 (the default in the compose
   files is `http://backend:8090`; on Dokploy substitute the actual assigned
   name). This is a **runtime** env var consumed by nginx `envsubst` on start.
3. **Ports / Domain**
   - Container port: **80**.
   - Add a **Domain**: `app.example.com`, container port `80`, enable **HTTPS**
     (Let's Encrypt). Dokploy provisions the certificate via Traefik.
4. **Deploy.** Visit `https://app.example.com` — you should get the login page.

> **`VITE_API_BASE_URL` is not set here.** In the same-origin model it's left
> empty at build time (the workflow only passes it when the `WEB_API_BASE_URL`
> repo variable is set). The web app requests `/api/...` and nginx proxies to
> `BACKEND_URL`. See the appendix if you want the cross-origin model instead.

---

## Step 6 — First boot

1. Open `https://app.example.com` and log in with `WEB_PASSWORD`.
2. Go to **Integrations → WhatsApp** and scan the QR code to pair the account.
   The whatsmeow session is stored in Postgres, so it survives redeploys (the
   containers are stateless — all state is in the databases).
3. Go to **Settings** and configure the LLM provider, API key, and model.

---

## Step 7 — Auto-deploy from GitHub Actions (complete the workflow TODO)

Each Dokploy application exposes a **redeploy webhook**. Wiring the GitHub
workflows to call it turns the "deploy-to-host TODO" into a real, independent
per-service rollout.

1. In Dokploy, open the **backend** app → **Deployments / Webhooks** and copy its
   **Webhook URL**. Do the same for the **web** app.
2. Add them as GitHub **repository secrets**:
   `DOKPLOY_BACKEND_WEBHOOK` and `DOKPLOY_WEB_WEBHOOK`.
3. Add a deploy step at the end of each workflow's `image` job, after the image is
   pushed. For `.github/workflows/deploy-backend.yml`:

   ```yaml
       - name: Trigger Dokploy redeploy
         run: curl -fsSL -X POST "${{ secrets.DOKPLOY_BACKEND_WEBHOOK }}"
   ```

   And the equivalent in `deploy-web.yml` with `DOKPLOY_WEB_WEBHOOK`.

Because the two workflows are **path-filtered** (backend on `app/api/**`,
`go.mod`, …; web on `app/web/**`, `packages/**`, …), a backend-only change
redeploys only the backend app on Dokploy, and a web/shared change redeploys only
the web app — which is the whole point of the split.

> Dokploy can also **auto-pull `:latest` on a schedule** or watch the registry
> instead of using the webhook, if you prefer not to edit the workflows. The
> webhook is the most immediate and precise option.

---

## Deploy & redeploy order

- **First time:** databases → backend → web (backend must be up before web can
  proxy to it; the web app still starts if the backend is briefly down and
  recovers once it's reachable).
- **Ongoing:** each service redeploys on its own via its webhook. Redeploys are
  safe to run any time — the containers are stateless, so there's no data
  migration step; all persistent state lives in Postgres/Mongo.

---

## Verifying / troubleshooting

| Symptom | Likely cause / fix |
| --- | --- |
| Dokploy can't pull the image | GHCR credential missing or PAT lacks `read:packages`; or the image tag was never pushed (run the deploy workflow once). |
| Login page loads but API calls 502/504 | `BACKEND_URL` points at the wrong service name, or the backend isn't up. Confirm the backend app's internal name and that it's listening on 8090. |
| Backend crashes on boot | Bad `POSTGRES_DSN` / `MONGO_URI` (wrong internal host or password), or `ENCRYPTION_KEY` not 32-byte base64. Check backend logs. |
| Chat responses don't stream | The `/api` proxy disables buffering for SSE (`proxy_buffering off`) — if you front the web app with another proxy, make sure it doesn't re-buffer `/api`. |
| TLS not issued | DNS `A` record for the domain isn't pointing at the Dokploy host yet, or port 80/443 blocked. |

---

## Appendix: cross-origin mode

If you want the browser to call the backend on its own public origin (e.g.
`api.example.com`) instead of proxying through the web app:

1. Give the **backend** app a public **domain** (`api.example.com`, port 8090,
   HTTPS) in Dokploy, and make sure the backend allows CORS from the web origin.
2. Set the GitHub repo **variable** `WEB_API_BASE_URL=https://api.example.com`
   (Settings → Secrets and variables → Actions → Variables). The `deploy-web`
   workflow passes it as the `VITE_API_BASE_URL` **build arg**, baking it into the
   bundle. You must **re-run `deploy-web`** for this to take effect — it's a
   build-time value, not a Dokploy runtime env var.
3. In this mode the web app's nginx `/api` proxy (`BACKEND_URL`) is unused, but
   leaving it set does no harm.

The same-origin model in the main guide is simpler (one public domain, no CORS)
and is the recommended default.

---

## Alternative: single Compose service

Instead of two Applications, Dokploy can deploy `deploy/docker-compose.split.yml`
as one **Compose** service (**+ Create Service → Compose**, point it at that file
and supply the `.env` values). This is closer to local parity and includes its
own Postgres/Mongo, but it **builds from source** (not the GHCR images) and
redeploys the whole stack together — so it gives up the independent per-service
rollout that PR #226 was built for. Prefer the two-Application setup above unless
you specifically want an all-in-one stack.
