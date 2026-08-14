// Package mcp is a minimal client for remote Model Context Protocol (MCP)
// servers spoken over the streamable-HTTP transport, plus a registry of the
// providers this app supports (Cloudflare, Railway, Notion).
//
// Each provider is a hosted MCP endpoint. Authentication is per provider: some
// accept a static API token (Cloudflare), others require an OAuth 2.1 connect
// flow (Notion, Railway — their hosted servers reject bearer tokens). The
// registry also carries a curated read/write classification of each server's
// tools so a project can enable a server in read-only or read & write mode, and
// the skill key that gates the server per project.
package mcp

import "strings"

// Provider identifies a supported MCP server. The value is the stable slug used
// in settings keys, API routes, and the tool-name namespace.
type Provider string

const (
	Cloudflare Provider = "cloudflare"
	Railway    Provider = "railway"
	Notion     Provider = "notion"
)

// AuthMode is how a provider authenticates.
type AuthMode string

const (
	// AuthToken authenticates with a static API token as an Authorization: Bearer
	// header (Cloudflare).
	AuthToken AuthMode = "token"
	// AuthOAuth authenticates with an OAuth 2.1 authorization-code flow — dynamic
	// client registration + PKCE + refresh (Notion, Railway).
	AuthOAuth AuthMode = "oauth"
)

// Mode is a per-project access level for an enabled MCP server.
type Mode string

const (
	// ModeRead exposes only tools classified as read-only.
	ModeRead Mode = "read"
	// ModeReadWrite exposes read and write tools.
	ModeReadWrite Mode = "readwrite"
)

// Normalize returns a valid Mode, defaulting unknown/empty values to ModeRead
// (the safe default — a misconfigured server never gains write access).
func (m Mode) Normalize() Mode {
	if m == ModeReadWrite {
		return ModeReadWrite
	}
	return ModeRead
}

// toolKind is the internal read/write classification of a single server tool.
type toolKind int

const (
	kindHidden toolKind = iota // not exposed to the agent at all
	kindRead                   // read-only tool
	kindWrite                  // mutating tool
)

// ProviderInfo describes a supported MCP server: its display name, default
// hosted endpoint, and the curated read/write tool classification.
type ProviderInfo struct {
	Slug            Provider
	Name            string
	DefaultEndpoint string

	// Auth is how this provider authenticates (token or oauth).
	Auth AuthMode

	// SkillKey is the in-app skill that gates this provider per project. The
	// provider's tools are only exposed when the skill is enabled for the active
	// project (in addition to being configured/connected).
	SkillKey string

	// Read and Write are explicit, curated tool-name allow-lists (matched
	// case-insensitively). A tool named here is always classified accordingly,
	// regardless of what the server advertises.
	Read  []string
	Write []string

	// UseAnnotations controls what happens to a tool that is in neither Read nor
	// Write. When false (strict allow-list) the tool is hidden — a new/unknown
	// server tool is never exposed until it is classified here. When true, the
	// tool is classified from the MCP `readOnlyHint` annotation when the server
	// provides one, and hidden when it does not. Used for large, fast-evolving
	// toolsets (Cloudflare, Railway) where hand-listing every tool is impractical;
	// it still never exposes an unannotated (unclassified) tool in read-only mode.
	UseAnnotations bool
}

// registry is the ordered set of supported providers.
var registry = []ProviderInfo{
	{
		Slug:            Notion,
		Name:            "Notion",
		DefaultEndpoint: "https://mcp.notion.com/mcp",
		Auth:            AuthOAuth,
		SkillKey:        "mcp_notion",
		// The Notion hosted MCP server exposes a fixed set of 22 tools, so we
		// classify all of them explicitly (strict allow-list, UseAnnotations off).
		Read: []string{
			"notion-search", "notion-fetch", "notion-download-attachment",
			"notion-query-data-sources", "notion-query-meeting-notes",
			"notion-get-comments", "notion-get-teams", "notion-get-users",
			"notion-get-async-task",
			// Some clients (OpenAI-style) receive these two with the notion- prefix
			// stripped; accept both forms.
			"search", "fetch",
		},
		Write: []string{
			"notion-create-file-upload", "notion-create-attachment",
			"notion-create-pages", "notion-update-page", "notion-convert-page-to-skill",
			"notion-move-pages", "notion-duplicate-page", "notion-create-database",
			"notion-create-folder", "notion-update-data-source", "notion-create-view",
			"notion-update-view", "notion-create-comment",
		},
	},
	{
		Slug:            Cloudflare,
		Name:            "Cloudflare",
		DefaultEndpoint: "https://mcp.cloudflare.com/mcp",
		Auth:            AuthToken,
		SkillKey:        "mcp_cloudflare",
		// Cloudflare's toolset is large and changes often; lean on the server's
		// readOnlyHint annotation, with a few well-known reads seeded explicitly.
		Read: []string{
			"search_cloudflare_documentation", "workers_list", "workers_get_worker",
			"accounts_list", "kv_namespaces_list", "r2_buckets_list", "d1_databases_list",
		},
		UseAnnotations: true,
	},
	{
		Slug:            Railway,
		Name:            "Railway",
		DefaultEndpoint: "https://mcp.railway.com/mcp",
		Auth:            AuthOAuth,
		SkillKey:        "mcp_railway",
		// Railway's hosted MCP requires OAuth. Same annotation-assisted read/write
		// classification as Cloudflare.
		Read: []string{
			"list_projects", "get_project", "list_services", "list_deployments",
			"get_deployment", "list_variables",
		},
		UseAnnotations: true,
	},
}

// Registry returns the supported providers in display order.
func Registry() []ProviderInfo { return registry }

// Lookup returns the ProviderInfo for a slug.
func Lookup(slug string) (ProviderInfo, bool) {
	for _, p := range registry {
		if string(p.Slug) == slug {
			return p, true
		}
	}
	return ProviderInfo{}, false
}

// contains reports whether name appears in list (case-insensitive).
func contains(list []string, name string) bool {
	for _, n := range list {
		if strings.EqualFold(n, name) {
			return true
		}
	}
	return false
}

// classify returns the read/write kind of a tool given its name and the
// server-advertised readOnlyHint (nil when the server did not annotate it).
func (pi ProviderInfo) classify(name string, readOnlyHint *bool) toolKind {
	switch {
	case contains(pi.Write, name):
		return kindWrite
	case contains(pi.Read, name):
		return kindRead
	case pi.UseAnnotations && readOnlyHint != nil:
		if *readOnlyHint {
			return kindRead
		}
		return kindWrite
	default:
		return kindHidden
	}
}

// Exposes reports whether a tool should be offered to the agent for the given
// access mode. Read-only mode offers only read tools; read & write offers both.
// Tools classified as hidden are never offered.
func (pi ProviderInfo) Exposes(name string, readOnlyHint *bool, mode Mode) bool {
	switch pi.classify(name, readOnlyHint) {
	case kindRead:
		return true
	case kindWrite:
		return mode.Normalize() == ModeReadWrite
	default:
		return false
	}
}
