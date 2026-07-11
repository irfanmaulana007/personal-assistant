# Deploying to Dokploy

This guide walks through deploying the whole personal-assistant to your own
[Dokploy](https://dokploy.com) server, step by step.

## What gets deployed

The app is a **single Docker container** (see `Dockerfile`):

- A multi-stage build compiles the **React client** (Vite → static files) and the
  **Go server** (with CGO + the `sqlite_fts5` build tag for SQLite full-text search).
- At runtime the Go server serves **both the JSON API and the static client** on
  **one port (`8090`)** — there is no separate frontend service.

### Storage backends

The server supports two backends, selected by the `DB_DRIVER` env var:

- **`sqlite`** (default) — all state lives in **SQLite files inside one volume**
  (`/app/data`): `assistant.db` (users, settings, traces, skills, notes, …) and
  `whatsmeow.db` (the WhatsApp pairing session). **One Application, one volume, no
  external database service.** This is the simplest path — start here.
- **`hybrid`** — application data lives in **PostgreSQL** and logs/analytics in
  **MongoDB**, with only the WhatsApp session left in the `/app/data` volume.
  Requires two extra database services. See
  [Option B](#option-b--hybrid-postgresql--mongodb-backend) below.

The rest of this guide covers the default **SQLite** deployment (Option A); the
hybrid backend is an add-on documented at the end.

## Prerequisites

- A server with **Dokploy already installed** (VPS with Docker; see the Dokploy docs
  for `curl -sSL https://dokploy.com/install.sh | sh`).
- A **domain** (or subdomain) pointed at the server's IP (an `A` record), e.g.
  `assistant.example.com` — needed for HTTPS.
- This repository pushed to **GitHub** (or GitLab/Gitea/Bitbucket), which Dokploy
  will build from.

## Environment variables

Set these on the Application (values are read at container start):

| Variable          | Required | Description                                                                 |
| ----------------- | -------- | --------------------------------------------------------------------------- |
| `ENCRYPTION_KEY`  | **yes**  | 32 bytes, base64. Encrypts stored secrets. **Generate once and never change it** — rotating it makes previously stored secrets unreadable. Generate with `openssl rand -base64 32`. |
| `WEB_PASSWORD`    | **yes**  | Password gate for the web app (required because `web.enabled` is true).      |
| `OWNER_JID`       | **yes**  | WhatsApp number(s) allowed to talk to the assistant, as JIDs, e.g. `6281234567890@s.whatsapp.net`. **Comma-separate several** to allow more than one (e.g. your personal + work numbers). The assistant runs on the account you pair via the Integrations QR and answers these numbers; the **first** one also receives reminders and the daily recap. |
| `LOG_LEVEL`       | no       | `debug` \| `info` \| `warn` \| `error` (default `info`).                     |

> The **LLM provider, API key, model, and base URL are NOT env vars** — they are set
> from the **Settings page** after first boot and stored encrypted in the database.
> The same is true for the **Composio API key** (used for Gmail / Google Calendar /
> GitHub / Sentry integrations): it's entered on the **Integrations page**, not via env.

> `DB_PATH` and the WhatsApp DB default to the `/app/data` volume — leave them alone so
> your data persists. Only `OWNER_JID` needs a real value even if you don't use WhatsApp
> (config validation requires it).

For the **hybrid backend** (Option B) also set these — otherwise leave them unset and
the app uses SQLite:

| Variable         | Required for hybrid | Description                                                              |
| ---------------- | ------------------- | ------------------------------------------------------------------------ |
| `DB_DRIVER`      | **yes**             | Set to `hybrid` to use PostgreSQL + MongoDB. Unset/`sqlite` keeps the default single-file backend. |
| `POSTGRES_DSN`   | **yes**             | PostgreSQL connection string, e.g. `postgres://assistant:PASS@assistant-postgres:5432/assistant?sslmode=disable`. |
| `MONGO_URI`      | **yes**             | MongoDB connection string, e.g. `mongodb://root:PASS@assistant-mongo:27017/?authSource=admin`. |
| `MONGO_DB`       | no                  | Mongo database name for logs (default `assistant_logs`).                  |

## Step 1 — Create the project and application

1. Open your Dokploy dashboard → **Create Project** (e.g. `assistant`).
2. Inside the project → **Create Service → Application**. Name it `assistant`.

## Step 2 — Connect the Git source

1. In the Application → **General / Source** tab, choose your Git provider and select
   this repository and the **`main`** branch.
   - If it's a private repo, connect the provider (GitHub App / deploy key) first.
2. Set **Build Type = `Dockerfile`** and the Dockerfile path to `./Dockerfile`
   (the repo root). Dokploy will build the multi-stage image on deploy.

## Step 3 — Set environment variables

Open the **Environment** tab and add the variables from the table above:

```
ENCRYPTION_KEY=<output of: openssl rand -base64 32>
WEB_PASSWORD=<a strong password>
OWNER_JID=6281234567890@s.whatsapp.net
LOG_LEVEL=info
```

## Step 4 — Add the persistent volume

This is critical — without it, your database and WhatsApp session are lost on every
redeploy.

1. Go to **Advanced → Volumes / Mounts → Add Mount**.
2. Type **Volume** (a Docker named volume managed by Dokploy).
3. **Volume Name:** `assistant-data`
4. **Mount Path (in container):** `/app/data`
5. Save.

## Step 5 — Expose it on your domain

1. Go to the **Domains** tab → **Add Domain**.
2. **Host:** your domain (e.g. `assistant.example.com`).
3. **Container Port:** `8090`.
4. Enable **HTTPS** (Let's Encrypt) and set the certificate provider to Let's Encrypt.
5. Save — Traefik will route the domain to the container and issue a certificate.

## Step 6 — Deploy

Click **Deploy**. Dokploy builds the image (first build takes a few minutes — it
compiles the client and the Go/CGO server) and starts the container. Watch **Logs**;
a healthy boot ends with the HTTP server starting on `:8090`.

Verify: `https://assistant.example.com/api/health` should return `{"status":"ok"}`.

## Step 7 — First-run setup (in the browser)

1. Open `https://assistant.example.com`. You'll be asked for the **web password**
   (`WEB_PASSWORD`), then to **create the first admin account** (this is the initial
   setup — the first user becomes admin).
2. Go to **Settings → Model** and add your **LLM provider + API key** (e.g. DeepSeek
   or OpenAI). Use **Test connection** to confirm.
3. Optional: **Settings → Display** to pick your timezone (UTC / GMT+7) and currency
   (USD / IDR).
4. Optional: **Skills** — enable the skills you want.
5. Optional: **WhatsApp** — go to **Integrations** and scan the QR (WhatsApp → Linked
   Devices) with the account the **assistant should run as**. A dedicated assistant number
   works well: pair that number, then set `OWNER_JID` to the number(s) that should be able
   to chat with it (your personal, and/or work — comma-separated). The assistant answers
   those numbers and replies to whoever wrote; the first `OWNER_JID` number gets reminders
   and the daily recap. (If you leave `OWNER_JID` as the paired number itself, it works in
   "Message Yourself" mode instead.) The session persists in `/app/data`, so you pair once.
6. Optional: **Integrations (Composio)** — to use Gmail, **Google Calendar**, GitHub, or
   Sentry, first paste your **Composio API key** on the Integrations page, then click
   **Connect** on each app and complete the hosted OAuth. For Google Calendar you can
   **Add account** more than once to connect several Google accounts — one-time events
   are created there and all connected calendars are merged into your schedule/recap.
   (One-time events fall back to a reminder if no calendar is connected.)

## Updating

Push to `main` and click **Deploy** again (or enable **Auto Deploy / webhooks** in the
Application settings so Dokploy redeploys on push). The `/app/data` volume is
preserved across deploys, so your data and WhatsApp pairing survive updates — as long
as `ENCRYPTION_KEY` stays the same.

## Backups

Everything is in the `assistant-data` volume. To back up, snapshot that volume, or
from the server:

```bash
# adjust the volume name to what Dokploy created (often <project>_assistant-data)
docker run --rm -v assistant-data:/data -v "$PWD":/backup alpine \
  tar czf /backup/assistant-data-$(date +%F).tar.gz -C /data .
```

Restore by extracting the archive back into the volume before starting the container.

**On the hybrid backend (Option B),** the `assistant-data` volume only holds the
WhatsApp session — your application data and logs live in the PostgreSQL and MongoDB
services. Back those up with their own tools (`pg_dump` for Postgres, `mongodump` for
Mongo), e.g. via Dokploy's database backup features.

## Option B — Hybrid PostgreSQL + MongoDB backend

By default the app runs on SQLite (Option A) and needs no external database. The
**hybrid** backend instead stores application data in **PostgreSQL** and
logs/analytics in **MongoDB**. It is entirely opt-in via `DB_DRIVER=hybrid`.

You only need this if you specifically want Postgres/Mongo (e.g. to share the data
with other tools, or to scale beyond a single instance). Otherwise stay on Option A.

### B1 — Create the database services in Dokploy

In the same project:

1. **Create Service → Database → PostgreSQL.** Name it `assistant-postgres`. Set a
   database name (`assistant`), user (`assistant`), and a strong password. Deploy it.
2. **Create Service → Database → MongoDB.** Name it `assistant-mongo`. Set the root
   user (`root`) and a strong password. Deploy it.

Dokploy runs these on the project's internal Docker network, so the Application
reaches them by **service name** as hostname (`assistant-postgres`, `assistant-mongo`)
— no public exposure needed.

### B2 — Point the Application at them

On the `assistant` Application's **Environment** tab, add (in addition to the Option A
vars):

```
DB_DRIVER=hybrid
POSTGRES_DSN=postgres://assistant:<pg-password>@assistant-postgres:5432/assistant?sslmode=disable
MONGO_URI=mongodb://root:<mongo-password>@assistant-mongo:27017/?authSource=admin
MONGO_DB=assistant_logs
```

Adjust the hostnames if you named the services differently. Redeploy the Application.
On boot the server **creates the PostgreSQL schema automatically** (embedded
migrations) and **ensures the MongoDB indexes** — there is no manual schema step.
The `/app/data` volume is still required (the WhatsApp session lives there).

### B3 — (Optional) Migrate existing SQLite data

If you are switching an existing SQLite deployment to hybrid and want to keep your
data, the image ships a `migrate-db` ETL binary. With the databases reachable, run it
once against your `assistant.db` (e.g. from the Application's terminal, or a one-off
container):

```bash
/app/migrate-db --config config/config.yaml \
  --sqlite data/assistant.db --truncate --verify
```

`--verify` compares per-table source/destination row counts and exits non-zero on any
mismatch; original ids are preserved so references stay valid.

## Alternative: docker-compose

The repo also ships a `docker-compose.yml`. It is now a **full hybrid stack** — the
`assistant` service plus bundled `postgres:17` and `mongo:7` services (with
healthchecks and named volumes) and `DB_DRIVER=hybrid` preset. If you prefer Dokploy's
**Compose** service type, point it at that file and set the environment variables.
For a plain VPS without Dokploy:

```bash
cp .env.example .env   # fill ENCRYPTION_KEY, WEB_PASSWORD, OWNER_JID,
                       # plus POSTGRES_PASSWORD and MONGO_PASSWORD
docker compose up -d --build
```

To run the compose stack on **SQLite** instead of the bundled databases, set
`DB_DRIVER=sqlite` on the `assistant` service (or remove the postgres/mongo services).

## Troubleshooting

- **Container exits immediately / `config validation failed`** — a required env var is
  missing. Ensure `ENCRYPTION_KEY`, `WEB_PASSWORD`, and `OWNER_JID` are all set.
- **"web.password is required"** — `WEB_PASSWORD` isn't set on the Application.
- **Data resets after redeploy** — the `/app/data` volume mount is missing or the mount
  path is wrong; it must be exactly `/app/data`.
- **Can't log in after a redeploy / secrets look corrupted** — `ENCRYPTION_KEY` changed.
  Restore the original key.
- **Blank page but `/api/health` works** — a build issue with the static client; check
  the build logs for the `client-builder` stage.
- **WhatsApp keeps asking to re-pair** — the volume isn't persisting `whatsmeow.db`;
  confirm the `/app/data` mount.
