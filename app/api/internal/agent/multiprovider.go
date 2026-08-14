package agent

import (
	"context"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/llm"
)

// multiProvider fans a single ToolProvider seam out across several underlying
// providers (e.g. Composio + MCP): Tools concatenates, Handles ORs, and Execute
// routes to the first provider that Handles the tool name.
type multiProvider struct {
	providers []ToolProvider
}

// CombineProviders composes several ToolProviders into one. nil entries are
// dropped; if fewer than two remain it returns the single provider (or nil) so
// callers can pass the result straight to New.
func CombineProviders(providers ...ToolProvider) ToolProvider {
	var kept []ToolProvider
	for _, p := range providers {
		if p != nil {
			kept = append(kept, p)
		}
	}
	switch len(kept) {
	case 0:
		return nil
	case 1:
		return kept[0]
	default:
		return &multiProvider{providers: kept}
	}
}

func (m *multiProvider) Tools(ctx context.Context) []llm.Tool {
	var out []llm.Tool
	for _, p := range m.providers {
		out = append(out, p.Tools(ctx)...)
	}
	return out
}

func (m *multiProvider) Handles(name string) bool {
	for _, p := range m.providers {
		if p.Handles(name) {
			return true
		}
	}
	return false
}

func (m *multiProvider) Execute(ctx context.Context, name, argsJSON string) string {
	for _, p := range m.providers {
		if p.Handles(name) {
			return p.Execute(ctx, name, argsJSON)
		}
	}
	return "Unknown tool: " + name
}
