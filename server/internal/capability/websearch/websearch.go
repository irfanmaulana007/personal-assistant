// Package websearch implements the Web Search skill: it lets the assistant look
// up current, real-world information on the open web (news, sports scores,
// prices, recent events) that isn't in the user's own data or the model's
// training cutoff. Results are handed back to the model as text so it can
// synthesize an answer and cite sources.
package websearch

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/irfanmaulana007/personal-assistant/server/internal/intent"
	"github.com/irfanmaulana007/personal-assistant/server/internal/settings"
	"github.com/irfanmaulana007/personal-assistant/server/internal/websearch"
)

// defaultResults is how many results to fetch when the caller doesn't specify.
const defaultResults = 5

// Handler answers web-search tool calls via the Brave Search client.
type Handler struct {
	client   *websearch.Client
	settings *settings.Service
	log      *slog.Logger
}

// New creates a web-search handler.
func New(client *websearch.Client, settingsSvc *settings.Service, log *slog.Logger) *Handler {
	return &Handler{client: client, settings: settingsSvc, log: log.With("component", "websearch")}
}

func (h *Handler) Name() string { return "web_search" }

func (h *Handler) Match(result *intent.ParseResult) bool {
	return result.Capability == intent.CapabilityWebSearch
}

func (h *Handler) Handle(ctx context.Context, result *intent.ParseResult) (string, error) {
	query := strings.TrimSpace(result.Entities["query"])
	if query == "" {
		return "What should I search the web for?", nil
	}

	apiKey, err := h.settings.WebSearchKey(ctx)
	if err != nil {
		return "", fmt.Errorf("resolve web search key: %w", err)
	}
	if apiKey == "" {
		// Reported to the model as text so it can tell the user gracefully.
		return "Web search is not configured — no search API key has been set. Ask the user to add a Brave Search API key on the Integrations page.", nil
	}

	count := defaultResults
	if c := strings.TrimSpace(result.Entities["count"]); c != "" {
		if n, convErr := strconv.Atoi(c); convErr == nil && n > 0 {
			count = n
		}
	}

	results, err := h.client.Search(ctx, apiKey, query, count)
	if err != nil {
		h.log.Warn("web search failed", "query", query, "error", err)
		// Surface a readable reason to the model rather than an error page.
		return fmt.Sprintf("Web search failed: %v", err), nil
	}
	if len(results) == 0 {
		return fmt.Sprintf("No web results found for %q.", query), nil
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Top web results for %q:", query))
	for i, r := range results {
		b.WriteString(fmt.Sprintf("\n\n%d. %s\n%s", i+1, strings.TrimSpace(r.Title), strings.TrimSpace(r.URL)))
		if desc := strings.TrimSpace(r.Description); desc != "" {
			b.WriteString("\n" + desc)
		}
	}
	b.WriteString("\n\nSummarize these for the user and cite the sources; do not invent facts beyond them.")
	return b.String(), nil
}
