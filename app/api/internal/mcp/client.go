package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// dialTimeout bounds a single connect+list or connect+call round-trip.
const dialTimeout = 30 * time.Second

// ToolInfo is a server tool as advertised over tools/list, reduced to what the
// agent adapter needs: the name, description, raw JSON-Schema for parameters,
// and the server's read-only hint (nil when the server did not annotate it).
type ToolInfo struct {
	Name         string
	Description  string
	InputSchema  json.RawMessage
	ReadOnlyHint *bool
}

// Client dials remote MCP servers over streamable HTTP. It is stateless — auth
// and endpoint are passed per call — so a single shared instance is safe for
// concurrent use.
type Client struct {
	impl *mcpsdk.Implementation
}

// NewClient returns an MCP client identifying itself as this app.
func NewClient() *Client {
	return &Client{impl: &mcpsdk.Implementation{Name: "personal-assistant", Version: "1.0"}}
}

// bearerRoundTripper injects an Authorization: Bearer header on every request.
type bearerRoundTripper struct {
	token string
	base  http.RoundTripper
}

func (b bearerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+b.token)
	return b.base.RoundTrip(req)
}

// connect opens an initialized session to endpoint authenticated with token.
// The caller must Close the returned session.
func (c *Client) connect(ctx context.Context, endpoint, token string) (*mcpsdk.ClientSession, error) {
	httpClient := &http.Client{
		Timeout:   dialTimeout,
		Transport: bearerRoundTripper{token: token, base: http.DefaultTransport},
	}
	// Request-response only: we never need server-initiated messages, and some
	// servers mishandle the standalone SSE GET.
	transport := &mcpsdk.StreamableClientTransport{
		Endpoint:             endpoint,
		HTTPClient:           httpClient,
		DisableStandaloneSSE: true,
	}
	sess, err := mcpsdk.NewClient(c.impl, nil).Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp connect %s: %w", endpoint, err)
	}
	return sess, nil
}

// ListTools returns every tool the server advertises, following pagination.
func (c *Client) ListTools(ctx context.Context, endpoint, token string) ([]ToolInfo, error) {
	sess, err := c.connect(ctx, endpoint, token)
	if err != nil {
		return nil, err
	}
	defer sess.Close()

	var out []ToolInfo
	params := &mcpsdk.ListToolsParams{}
	for {
		res, err := sess.ListTools(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("mcp list tools: %w", err)
		}
		for _, t := range res.Tools {
			out = append(out, toToolInfo(t))
		}
		if res.NextCursor == "" {
			break
		}
		params.Cursor = res.NextCursor
	}
	return out, nil
}

// CallTool invokes a tool and returns its textual result. A tool-level error
// (CallToolResult.IsError) is returned as an error carrying the server's text so
// the caller can surface it to the model.
func (c *Client) CallTool(ctx context.Context, endpoint, token, name string, args json.RawMessage) (string, error) {
	sess, err := c.connect(ctx, endpoint, token)
	if err != nil {
		return "", err
	}
	defer sess.Close()

	params := &mcpsdk.CallToolParams{Name: name}
	if len(args) > 0 {
		params.Arguments = args
	}
	res, err := sess.CallTool(ctx, params)
	if err != nil {
		return "", fmt.Errorf("mcp call %s: %w", name, err)
	}
	text := resultText(res)
	if res.IsError {
		return text, fmt.Errorf("tool %s returned an error", name)
	}
	return text, nil
}

// toToolInfo reduces an SDK Tool to our ToolInfo, preserving the raw input
// schema and the read-only annotation (nil when unannotated).
func toToolInfo(t *mcpsdk.Tool) ToolInfo {
	ti := ToolInfo{Name: t.Name, Description: t.Description}
	if t.InputSchema != nil {
		if raw, err := json.Marshal(t.InputSchema); err == nil {
			ti.InputSchema = raw
		}
	}
	if t.Annotations != nil {
		hint := t.Annotations.ReadOnlyHint
		ti.ReadOnlyHint = &hint
	}
	return ti
}

// resultText flattens a CallToolResult's content blocks into plain text.
func resultText(res *mcpsdk.CallToolResult) string {
	var b strings.Builder
	for _, ct := range res.Content {
		if tc, ok := ct.(*mcpsdk.TextContent); ok {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(tc.Text)
		}
	}
	if b.Len() == 0 && res.StructuredContent != nil {
		if raw, err := json.Marshal(res.StructuredContent); err == nil {
			return string(raw)
		}
	}
	return b.String()
}
