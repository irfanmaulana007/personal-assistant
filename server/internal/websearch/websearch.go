// Package websearch is a thin client over the Tavily Search API. It gives the
// assistant a way to answer questions about current, real-world information
// (news, scores, prices, "what happened today") that isn't in the user's own
// data or the model's training cutoff.
//
// Tavily is used because it's purpose-built for LLM agents: it returns a
// synthesized answer alongside ranked, pre-summarized results, and offers a
// genuine free tier (no card required). The API key is supplied per call
// (resolved from encrypted settings) rather than baked into the client, so the
// same client instance serves every request.
package websearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// endpoint is the Tavily search REST endpoint.
const endpoint = "https://api.tavily.com/search"

// Result is a single web result, trimmed to what the assistant needs to
// synthesize an answer and cite a source.
type Result struct {
	Title   string
	URL     string
	Content string
}

// Response is a search response: Tavily's synthesized answer (may be empty) and
// the ranked results backing it.
type Response struct {
	Answer  string
	Results []Result
}

// Client calls the Tavily Search API. It is safe for concurrent use.
type Client struct {
	http *http.Client
}

// New creates a search client with a sane request timeout.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 15 * time.Second}}
}

type tavilyRequest struct {
	Query         string `json:"query"`
	MaxResults    int    `json:"max_results"`
	SearchDepth   string `json:"search_depth"`
	IncludeAnswer bool   `json:"include_answer"`
}

// tavilyResponse mirrors the slice of the Tavily payload we care about.
type tavilyResponse struct {
	Answer  string `json:"answer"`
	Results []struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Content string `json:"content"`
	} `json:"results"`
}

// Search runs a query and returns Tavily's answer plus up to count results.
// apiKey is the Tavily API key; count is clamped to Tavily's 0..20 range.
func (c *Client) Search(ctx context.Context, apiKey, query string, count int) (*Response, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("web search is not configured")
	}
	if count <= 0 {
		count = 5
	}
	if count > 20 {
		count = 20
	}

	body, err := json.Marshal(tavilyRequest{
		Query:         query,
		MaxResults:    count,
		SearchDepth:   "basic",
		IncludeAnswer: true,
	})
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tavily search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("web search rejected the API key (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("web search rate limit reached, try again shortly")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tavily search returned HTTP %d", resp.StatusCode)
	}

	var parsed tavilyResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode tavily response: %w", err)
	}

	out := &Response{Answer: parsed.Answer, Results: make([]Result, 0, len(parsed.Results))}
	for _, r := range parsed.Results {
		out.Results = append(out.Results, Result{Title: r.Title, URL: r.URL, Content: r.Content})
	}
	return out, nil
}
