// Package composiotools adapts a user's connected Composio apps into agent
// tools: it exposes a curated subset of each connected app's actions to the LLM
// and executes tool calls through Composio. It implements agent.ToolProvider.
package composiotools

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/authctx"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/composio"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/llm"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/settings"
)

// curated maps our supported toolkit slugs to a hand-picked set of Composio
// tool slugs (the most useful actions). If none resolve for a toolkit, we fall
// back to that toolkit's top tools so a slug rename doesn't break everything.
// Google Calendar is intentionally NOT exposed as raw Composio tools here — the
// agent uses the higher-level schedule_event / list_calendar tools (backed by
// internal/calendar), which handle multi-account and the reminder fallback.
var curated = map[string][]string{
	"gmail":  {"GMAIL_SEND_EMAIL", "GMAIL_FETCH_EMAILS", "GMAIL_CREATE_EMAIL_DRAFT"},
	"github": {"GITHUB_CREATE_AN_ISSUE", "GITHUB_LIST_REPOSITORY_ISSUES", "GITHUB_SEARCH_ISSUES"},
	"sentry": {"SENTRY_LIST_ORGANIZATION_ISSUES", "SENTRY_LIST_PROJECTS"},
	"trello": {
		"TRELLO_ADD_CARDS",
		"TRELLO_GET_MEMBERS_BOARDS_BY_ID_MEMBER",
		"TRELLO_GET_BOARDS_LISTS_BY_ID_BOARD",
		"TRELLO_GET_BOARDS_CARDS_BY_ID_BOARD",
		"TRELLO_ADD_CARDS_ACTIONS_COMMENTS_BY_ID_CARD",
	},
}

const fallbackLimit = 6

// Provider resolves and executes Composio tools for the current user.
type Provider struct {
	client   *composio.Client
	settings *settings.Service
	log      *slog.Logger
}

// New creates a Composio tool provider.
func New(client *composio.Client, settingsSvc *settings.Service, log *slog.Logger) *Provider {
	return &Provider{client: client, settings: settingsSvc, log: log.With("component", "composio-tools")}
}

func (p *Provider) resolve(ctx context.Context) (apiKey, userID string) {
	key, err := p.settings.ComposioKey(ctx)
	if err != nil || key == "" {
		return "", ""
	}
	uid := authctx.UserID(ctx)
	if uid == 0 {
		return "", ""
	}
	return key, strconv.FormatInt(uid, 10)
}

// Tools returns the curated tools for the user's connected apps.
func (p *Provider) Tools(ctx context.Context) []llm.Tool {
	key, userID := p.resolve(ctx)
	if key == "" {
		return nil
	}
	conns, err := p.client.ListConnections(ctx, key, userID)
	if err != nil {
		p.log.Warn("list connections for tools", "error", err)
		return nil
	}

	var defs []composio.ToolDef
	seen := map[string]bool{}
	for _, c := range conns {
		if strings.ToUpper(c.Status) != "ACTIVE" {
			continue
		}
		slugs, ok := curated[c.ToolkitSlug]
		if !ok || seen[c.ToolkitSlug] {
			continue
		}
		seen[c.ToolkitSlug] = true

		td, err := p.client.GetTools(ctx, key, slugs)
		if err != nil || len(td) == 0 {
			td, err = p.client.GetToolsByToolkit(ctx, key, c.ToolkitSlug, fallbackLimit)
			if err != nil {
				p.log.Warn("get composio tools", "toolkit", c.ToolkitSlug, "error", err)
				continue
			}
		}
		defs = append(defs, td...)
	}

	tools := make([]llm.Tool, 0, len(defs))
	for _, d := range defs {
		params := d.Parameters
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		tools = append(tools, llm.Tool{
			Type:     "function",
			Function: llm.ToolFunction{Name: d.Slug, Description: d.Description, Parameters: params},
		})
	}
	return tools
}

// Handles reports whether a tool name is a Composio tool for a supported app.
func (p *Provider) Handles(name string) bool {
	for slug := range curated {
		if strings.HasPrefix(name, strings.ToUpper(slug)+"_") {
			return true
		}
	}
	return false
}

// Execute runs a Composio tool for the current user.
func (p *Provider) Execute(ctx context.Context, name, argsJSON string) string {
	key, userID := p.resolve(ctx)
	if key == "" {
		return "This app isn't connected. Ask an admin to connect it in Integrations."
	}
	out, err := p.client.ExecuteTool(ctx, key, name, argsJSON, userID, "")
	if err != nil {
		p.log.Warn("execute composio tool", "tool", name, "error", err)
		return "Error running " + name + ": " + err.Error()
	}
	return out
}
