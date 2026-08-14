// Package mcptools adapts a project's enabled MCP servers into agent tools. A
// server's tools are exposed only when (a) the server's in-app skill is enabled
// for the active project, and (b) it is configured/connected — a token for
// token-auth providers (Cloudflare), or an OAuth connection for OAuth providers
// (Notion, Railway). Exposed tools are further filtered by the server's curated
// read/write classification and the project's access mode. It implements
// agent.ToolProvider.
//
// Tool names are namespaced as "<provider>__<tool>" (e.g. notion__notion-search)
// so routing is unambiguous and names never collide with built-in or Composio
// tools.
package mcptools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/authctx"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/llm"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/mcp"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/settings"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/store"
	"golang.org/x/oauth2"
)

const (
	nameSep       = "__"
	listCacheTTL  = 5 * time.Minute
	emptyToolArgs = `{"type":"object","properties":{}}`
)

// validToolName is the OpenAI/LLM function-name charset (letters, digits, _ and
// -, up to 64 chars). Namespaced names that don't fit are skipped.
var validToolName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// notionTargetStore is the slice of the store the Notion guidance needs.
type notionTargetStore interface {
	ListNotionTargets(ctx context.Context) ([]store.NotionTarget, error)
}

// skillChecker reports which skills are enabled for a project.
type skillChecker interface {
	EnabledProjectSkillKeys(ctx context.Context, projectID int64) ([]string, error)
}

// oauthTokenSourcer returns an auto-refreshing OAuth token source for an
// OAuth-auth provider connected to the active project.
type oauthTokenSourcer interface {
	TokenSource(ctx context.Context, provider string) (oauth2.TokenSource, error)
}

// Provider resolves and executes MCP tools for the active project.
type Provider struct {
	client   *mcp.Client
	settings *settings.Service
	store    notionTargetStore
	skills   skillChecker
	oauth    oauthTokenSourcer
	log      *slog.Logger

	mu    sync.Mutex
	cache map[string]cacheEntry // key: pid|provider|endpoint
}

type cacheEntry struct {
	tools     []mcp.ToolInfo
	expiresAt time.Time
}

// New creates an MCP tool provider.
func New(client *mcp.Client, settingsSvc *settings.Service, st notionTargetStore, skills skillChecker, oauth oauthTokenSourcer, log *slog.Logger) *Provider {
	return &Provider{
		client:   client,
		settings: settingsSvc,
		store:    st,
		skills:   skills,
		oauth:    oauth,
		log:      log.With("component", "mcp-tools"),
		cache:    map[string]cacheEntry{},
	}
}

// serverConfig is a resolved, enabled+configured MCP server for the active project.
type serverConfig struct {
	info     mcp.ProviderInfo
	endpoint string
	auth     mcp.Auth
	mode     mcp.Mode
}

// enabledSkillSet returns the active project's enabled skill keys as a set.
func (p *Provider) enabledSkillSet(ctx context.Context) map[string]bool {
	keys, err := p.skills.EnabledProjectSkillKeys(ctx, authctx.ProjectID(ctx))
	if err != nil {
		p.log.Warn("read enabled skills", "error", err)
		return nil
	}
	m := make(map[string]bool, len(keys))
	for _, k := range keys {
		m[k] = true
	}
	return m
}

// resolveServer resolves one provider for the active project: it must have its
// skill enabled and be configured (token) or connected (oauth). Returns ok=false
// otherwise.
func (p *Provider) resolveServer(ctx context.Context, info mcp.ProviderInfo, enabledSkills map[string]bool) (serverConfig, bool) {
	if !enabledSkills[info.SkillKey] {
		return serverConfig{}, false
	}
	cfg, err := p.settings.MCPServer(ctx, string(info.Slug))
	if err != nil {
		p.log.Warn("read mcp config", "provider", info.Slug, "error", err)
		return serverConfig{}, false
	}
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = info.DefaultEndpoint
	}
	var a mcp.Auth
	switch info.Auth {
	case mcp.AuthOAuth:
		ts, err := p.oauth.TokenSource(ctx, string(info.Slug))
		if err != nil {
			return serverConfig{}, false // not connected
		}
		a = mcp.OAuthAuth(ts)
	default:
		if cfg.Token == "" {
			return serverConfig{}, false // no token
		}
		a = mcp.TokenAuth(cfg.Token)
	}
	return serverConfig{info: info, endpoint: endpoint, auth: a, mode: mcp.Mode(cfg.Mode).Normalize()}, true
}

// enabledServers returns every provider enabled+configured for the active project.
func (p *Provider) enabledServers(ctx context.Context) []serverConfig {
	if authctx.ProjectID(ctx) == 0 {
		return nil
	}
	enabled := p.enabledSkillSet(ctx)
	if len(enabled) == 0 {
		return nil
	}
	var out []serverConfig
	for _, info := range mcp.Registry() {
		if sc, ok := p.resolveServer(ctx, info, enabled); ok {
			out = append(out, sc)
		}
	}
	return out
}

// Tools returns the tools exposed by the active project's enabled MCP servers,
// filtered by each server's curated read/write classification and access mode.
func (p *Provider) Tools(ctx context.Context) []llm.Tool {
	servers := p.enabledServers(ctx)
	if len(servers) == 0 {
		return nil
	}

	var out []llm.Tool
	for _, sc := range servers {
		tools, err := p.listTools(ctx, sc)
		if err != nil {
			p.log.Warn("mcp list tools", "provider", sc.info.Slug, "error", err)
			continue
		}
		var suffix string
		if sc.info.Slug == mcp.Notion {
			suffix = p.notionGuidance(ctx)
		}
		for _, t := range tools {
			if !sc.info.Exposes(t.Name, t.ReadOnlyHint, sc.mode) {
				continue
			}
			name := string(sc.info.Slug) + nameSep + t.Name
			if !validToolName.MatchString(name) {
				p.log.Warn("skip mcp tool with unsupported name", "provider", sc.info.Slug, "tool", t.Name)
				continue
			}
			params := t.InputSchema
			if len(params) == 0 {
				params = json.RawMessage(emptyToolArgs)
			}
			desc := t.Description
			if suffix != "" {
				desc = strings.TrimSpace(desc + "\n\n" + suffix)
			}
			out = append(out, llm.Tool{
				Type:     "function",
				Function: llm.ToolFunction{Name: name, Description: desc, Parameters: params},
			})
		}
	}
	return out
}

// Handles reports whether a tool name belongs to one of our MCP providers.
func (p *Provider) Handles(name string) bool {
	provider, _, ok := splitName(name)
	if !ok {
		return false
	}
	_, found := mcp.Lookup(provider)
	return found
}

// Execute runs an MCP tool for the active project.
func (p *Provider) Execute(ctx context.Context, name, argsJSON string) string {
	provider, tool, ok := splitName(name)
	if !ok {
		return "Unknown tool: " + name
	}
	info, found := mcp.Lookup(provider)
	if !found {
		return "Unknown MCP provider for tool: " + name
	}
	sc, ok := p.resolveServer(ctx, info, p.enabledSkillSet(ctx))
	if !ok {
		return info.Name + " isn't enabled or connected for this project. Ask an admin to enable it on the Skills page and connect it on the Integrations → MCP page."
	}
	// Enforce the access mode server-side too: a call the model shouldn't have
	// been offered (write tool in read-only mode) is refused. Uses cached tool
	// metadata (populated by Tools() earlier this turn) for the annotation hint.
	if !info.Exposes(tool, p.cachedHint(ctx, sc, tool), sc.mode) {
		return fmt.Sprintf("%s is in read-only mode for this project, so %q is not allowed. Ask an admin to switch it to read & write on the Integrations → MCP page.", info.Name, tool)
	}

	out, err := p.client.CallTool(ctx, sc.endpoint, sc.auth, tool, json.RawMessage(argsJSON))
	if err != nil {
		p.log.Warn("execute mcp tool", "provider", provider, "tool", tool, "error", err)
		if out != "" {
			return out // server-provided error text is more useful to the model
		}
		return "Error running " + name + ": " + err.Error()
	}
	return out
}

// listTools returns a server's advertised tools, cached per project+server for a
// short TTL so we don't re-handshake on every turn.
func (p *Provider) listTools(ctx context.Context, sc serverConfig) ([]mcp.ToolInfo, error) {
	key := cacheKey(authctx.ProjectID(ctx), sc.info.Slug, sc.endpoint)

	p.mu.Lock()
	if e, ok := p.cache[key]; ok && time.Now().Before(e.expiresAt) {
		tools := e.tools
		p.mu.Unlock()
		return tools, nil
	}
	p.mu.Unlock()

	tools, err := p.client.ListTools(ctx, sc.endpoint, sc.auth)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	p.cache[key] = cacheEntry{tools: tools, expiresAt: time.Now().Add(listCacheTTL)}
	p.mu.Unlock()
	return tools, nil
}

// cachedHint returns the read-only hint for a tool from the cache, or nil when
// unknown (cache miss or the server didn't annotate it).
func (p *Provider) cachedHint(ctx context.Context, sc serverConfig, tool string) *bool {
	key := cacheKey(authctx.ProjectID(ctx), sc.info.Slug, sc.endpoint)
	p.mu.Lock()
	defer p.mu.Unlock()
	e, ok := p.cache[key]
	if !ok {
		return nil
	}
	for _, t := range e.tools {
		if t.Name == tool {
			return t.ReadOnlyHint
		}
	}
	return nil
}

// notionGuidance builds a short suffix naming the project's mapped Notion
// databases so the model targets the right one. Returns "" when none are mapped.
func (p *Provider) notionGuidance(ctx context.Context) string {
	if p.store == nil {
		return ""
	}
	targets, err := p.store.ListNotionTargets(ctx)
	if err != nil || len(targets) == 0 {
		return ""
	}
	parts := make([]string, 0, len(targets))
	for _, t := range targets {
		label := t.Kind
		if t.Name != "" {
			label = fmt.Sprintf("%s tracker %q", t.Kind, t.Name)
		}
		parts = append(parts, fmt.Sprintf("%s → database_id %s", label, t.DatabaseID))
	}
	return "This project's Notion databases: " + strings.Join(parts, "; ") +
		". Use the matching database_id when creating or querying items."
}

// splitName splits "<provider>__<tool>" on the first separator. The tool part
// keeps any further separators intact.
func splitName(name string) (provider, tool string, ok bool) {
	i := strings.Index(name, nameSep)
	if i <= 0 || i+len(nameSep) >= len(name) {
		return "", "", false
	}
	return name[:i], name[i+len(nameSep):], true
}

// cacheKey derives a stable cache key for a project+provider+endpoint.
func cacheKey(projectID int64, provider mcp.Provider, endpoint string) string {
	return fmt.Sprintf("%d|%s|%s", projectID, provider, endpoint)
}
