# Setting up the LLM Agent

The assistant is powered by an **LLM tool-calling agent**. Instead of matching
your messages against fixed patterns, it sends them to a Large Language Model
which decides — on its own — when to call the built-in tools (calendar, email,
reminders, notes) and how to reply.

It is **provider-agnostic**: DeepSeek, OpenAI, OpenRouter, Groq, Mistral, or any
other OpenAI-compatible endpoint. You pick the provider from the **Settings**
page. All configuration lives in the database — there are no LLM environment
variables or YAML settings.

---

## 1. Get an API key

Pick a provider and create an API key:

| Provider   | Where to get a key                     | Default model               |
| ---------- | -------------------------------------- | --------------------------- |
| DeepSeek   | <https://platform.deepseek.com>        | `deepseek-chat`             |
| OpenAI     | <https://platform.openai.com/api-keys> | `gpt-4o-mini`               |
| OpenRouter | <https://openrouter.ai/keys>           | `openai/gpt-4o-mini`        |
| Groq       | <https://console.groq.com/keys>        | `llama-3.3-70b-versatile`   |
| Mistral    | <https://console.mistral.ai>           | `mistral-small-latest`      |
| Custom     | any OpenAI-compatible endpoint         | _(you specify)_             |

Copy the key somewhere safe — most providers only show it once.

---

## 2. Configure it in Settings

1. Start the app (`make dev-server` + `make dev-client`, or `make run`) and log
   in with your web password.
2. Open **Settings** in the left sidebar.
3. Choose your **Provider** — this pre-fills the Base URL and default Model
   (both remain editable).
4. Paste your **API Key**, adjust the **Model** if you want a different one, and
   click **Save**.
5. Click **Test connection** — you should see *Connection OK*.

The key is stored **encrypted at rest** (AES-256-GCM, using your
`ENCRYPTION_KEY`) in the SQLite database. The Settings page only ever shows a
masked value like `••••7890`. The database is the single source of truth for all
LLM settings.

### Switching providers later

Just pick a different **Provider** in Settings, paste that provider's key, and
Save. Because they all use the OpenAI-compatible API, no code changes are needed.
For a provider not in the list, choose **Custom (OpenAI-compatible)** and enter
its Base URL and Model manually.

---

## 3. Settings reference

| Field    | Notes                                                                    |
| -------- | ------------------------------------------------------------------------ |
| Provider | Selects preset Base URL + default Model. `custom` for any other endpoint. |
| API Key  | Required. Stored encrypted. Blank = keep existing key.                    |
| Model    | Any model id your provider accepts.                                      |
| Base URL | The provider's OpenAI-compatible endpoint.                              |

---

## 4. How the agent works

1. Your message plus recent conversation history is sent to the model, along
   with the schema of every available tool.
2. If the model calls a tool (e.g. `reminder_set`), the call is mapped onto the
   matching capability handler and executed. Each tool maps to one capability
   action:

   | Tool                                              | Capability |
   | ------------------------------------------------- | ---------- |
   | `calendar_list/create/update/delete`              | Calendar   |
   | `email_inbox/read/search/draft`                   | Email      |
   | `reminder_set/list/cancel`                        | Reminders  |
   | `note_save/search/list/delete`                    | Notes      |

3. The tool's result is fed back to the model, which either calls another tool
   or writes the final reply. The loop is capped at 5 iterations.
4. Token usage for every turn is recorded in the `llm_usage` table (this will
   power the usage/cost Dashboard).

Calendar and email tools require Google to be connected and enabled; reminders
and notes work without any external setup.

---

## 5. Troubleshooting

| Symptom                                                        | Fix                                                                                   |
| ------------------------------------------------------------- | ------------------------------------------------------------------------------------- |
| Chat replies "The assistant isn't configured yet"             | No API key set. Add one in Settings.                                                   |
| **Test connection** fails with `401` / invalid key            | The key is wrong or revoked. Re-copy it from the provider and Save again.              |
| Test fails with `model not found` / `400`                     | The **Model** id isn't valid for the selected provider. Check the provider's model list. |
| Test fails with a connection/timeout error                    | Wrong **Base URL**, or no network access to the provider from the server.             |
| The agent never uses a tool                                   | Make sure the relevant capability is enabled in `config.yaml`; try a clearer request. |
| Calendar/email tools error                                    | Google isn't connected — see the Google setup, or disable those capabilities.         |

The server logs each agent run; set `logging.level: debug` in `config.yaml` to
see the tools being called.
