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
  `server/internal/composio/composio.go` accordingly.
