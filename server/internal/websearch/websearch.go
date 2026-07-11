// Package websearch is a thin client over the Brave Search API. It gives the
// assistant a way to answer questions about current, real-world information
// (news, scores, prices, "what happened today") that isn't in the user's own
// data or the model's training cutoff.
//
// Brave is used because it's a genuine general-web search with a single-header
// API key and a clean JSON response. The subscription token is supplied per
// call (resolved from encrypted settings) rather than baked into the client, so
// the same client instance serves every request.
package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// endpoint is the Brave Search web-search REST endpoint.
const endpoint = "https://api.search.brave.com/res/v1/web/search"

// Result is a single web result, trimmed to what the assistant needs to
// synthesize an answer and cite a source.
type Result struct {
	Title       string
	URL         string
	Description string
}

// Client calls the Brave Search API. It is safe for concurrent use.
type Client struct {
	http *http.Client
}

// New creates a search client with a sane request timeout.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 12 * time.Second}}
}

// braveResponse mirrors the slice of the Brave payload we care about.
type braveResponse struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"results"`
	} `json:"web"`
}

// Search runs a query and returns up to count results. apiKey is the Brave
// subscription token; count is clamped to Brave's 1..20 range.
func (c *Client) Search(ctx context.Context, apiKey, query string, count int) ([]Result, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("web search is not configured")
	}
	if count <= 0 {
		count = 5
	}
	if count > 20 {
		count = 20
	}

	q := url.Values{}
	q.Set("q", query)
	q.Set("count", strconv.Itoa(count))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brave search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("web search rejected the API key (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("web search rate limit reached, try again shortly")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("brave search returned HTTP %d", resp.StatusCode)
	}

	var parsed braveResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode brave response: %w", err)
	}

	out := make([]Result, 0, len(parsed.Web.Results))
	for _, r := range parsed.Web.Results {
		out = append(out, Result{Title: r.Title, URL: r.URL, Description: r.Description})
	}
	return out, nil
}
