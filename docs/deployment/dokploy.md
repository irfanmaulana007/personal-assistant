# Deploying to Dokploy

This guide walks through deploying the whole personal-assistant to your own
[Dokploy](https://dokploy.com) server, step by step.

## What gets deployed

The app is a **single Docker container** (see `Dockerfile`):

- A multi-stage build compiles the **React client** (Vite → static files) and the
  **Go server** (with CGO + the `sqlite_fts5` build tag for SQLite full-text search).
- At runtime the Go server serves **both the JSON API and the static client** on
  **one port (`8090`)** — there is no separate frontend service.
- All state lives in **SQLite files inside one volume** (`/app/data`):
  - `assistant.db` — users, settings (encrypted LLM key), traces, skills, notes, etc.
  - `whatsmeow.db` — the WhatsApp (whatsmeow) pairing session.

So on Dokploy you deploy **one Application** with **one persistent volume** and a few
environment variables. No external database service is required.

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
| `OWNER_JID`       | **yes**  | Your WhatsApp JID, e.g. `6281234567890@s.whatsapp.net` (config validation requires it; used to match your inbound messages). |
| `LOG_LEVEL`       | no       | `debug` \| `info` \| `warn` \| `error` (default `info`).                     |

> The **LLM provider, API key, model, and base URL are NOT env vars** — they are set
> from the **Settings page** after first boot and stored encrypted in the database.
> The same is true for the **Composio API key** (used for Gmail / Google Calendar /
> GitHub / Sentry integrations): it's entered on the **Integrations page**, not via env.

> `DB_PATH` and the WhatsApp DB default to the `/app/data` volume — leave them alone so
> your data persists. Only `OWNER_JID` needs a real value even if you don't use WhatsApp
> (config validation requires it).

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
5. Optional: **WhatsApp** — go to **Integrations** and link WhatsApp by scanning the QR
   with your phone (WhatsApp → Linked Devices). The session persists in the `/app/data`
   volume, so you only pair once. Reminders and the daily schedule recap are delivered to
   this paired account.
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

## Alternative: docker-compose

The repo also ships a `docker-compose.yml` (single service + named volume). If you
prefer Dokploy's **Compose** service type, point it at that file and set the same
environment variables. For a plain VPS without Dokploy:

```bash
cp .env.example .env   # then fill ENCRYPTION_KEY, WEB_PASSWORD, OWNER_JID
docker compose up -d --build
```

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
