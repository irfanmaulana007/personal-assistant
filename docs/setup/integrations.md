# Integrations (Composio)

The **Integrations** page (admin only) connects third-party apps —
**Gmail, Google Calendar, GitHub, Sentry** — through
[Composio](https://composio.dev), which manages the OAuth flows for you.

Once an app is **connected**, the assistant can use it in Chat: a curated set of
each connected app's actions is exposed to the LLM agent as tools (e.g. Gmail:
send / fetch / draft; Calendar: create / find / list; GitHub: create issue /
list / search; Sentry: list issues / projects). The agent calls them through
Composio on your behalf. Apps you haven't connected contribute no tools.

## 1. Get a Composio API key

1. Create an account at <https://app.composio.dev>.
2. Copy your **API key** from the dashboard.

## 2. Configure it

1. Sign in to the app as an **admin** and open **Integrations**.
2. Paste your Composio API key into the **Composio API key** field and **Save**.
   The key is stored **encrypted** in the database (never in config/env).

## 3. Connect an app

1. Click **Connect** on a toolkit (e.g. Gmail).
2. A Composio-hosted authorization page opens in a new tab — approve access.
3. Back in the app, click **Refresh**; the toolkit shows **Connected**.

Connections are **per user** (the signed-in admin), keyed by the app's user id
in Composio. **Disconnect** removes the connection.

## WhatsApp (personal number)

Separately from Composio, you can link your **personal WhatsApp** so you can chat
with the assistant from WhatsApp (via whatsmeow — a WhatsApp Web linked device):

1. Set `whatsapp.enabled: true` in `app/api/config/config.yaml` and start the
   server (it no longer blocks on WhatsApp — the web app comes up immediately).
2. Open **Integrations** as an admin → the **WhatsApp** card → **Connect**.
3. A QR code appears in the browser. On your phone: **WhatsApp → Settings →
   Linked Devices → Link a Device**, and scan it. The card flips to
   **Connected** automatically.

The owner number is derived from the paired device (no `whatsapp_jid` config
needed), and reminders are delivered to that account. **Disconnect** unlinks it.

> whatsmeow is an unofficial client — it links your real number like WhatsApp
> Web. There's a small, non-zero risk of a WhatsApp ban for automated use.

## Notes & caveats

- Some toolkits need an **auth config** in your Composio project. The app tries
  to reuse an existing one and otherwise creates a **Composio-managed** OAuth
  config automatically. If a toolkit has no managed OAuth (or needs custom
  credentials — e.g. Sentry in some setups), create the auth config in the
  Composio dashboard first, then Connect.
- Composio's REST API has shifted across versions. This integration targets the
  documented **v3** shapes (`/api/v3/auth_configs`, `/api/v3/connected_accounts`).
  If a request fails, the exact error from Composio is surfaced in the UI — if an
  endpoint/field name has changed on your account, adjust
  `app/api/internal/composio/composio.go` accordingly.
