// Package composio is a minimal client for the Composio v3 REST API, used to
// manage per-user app connections (OAuth) via Composio's hosted auth.
//
// NOTE: Composio's API has shifted across versions. This client targets the
// documented v3 shapes; the exact endpoints/field names may need a small tweak
// once verified against a live Composio account.
package composio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is the Composio v3 API base.
const DefaultBaseURL = "https://backend.composio.dev/api/v3"

// Client talks to the Composio REST API. The API key is passed per call so it
// can be resolved from settings at request time.
type Client struct {
	http    *http.Client
	baseURL string
}

// NewClient returns a Composio client with a sensible timeout.
func NewClient() *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}, baseURL: DefaultBaseURL}
}

func (c *Client) do(ctx context.Context, apiKey, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("composio request: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("composio %s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("parse composio response: %w", err)
		}
	}
	return nil
}

// Connection is a user's connection to a toolkit.
type Connection struct {
	ID          string
	Status      string // ACTIVE, INITIATED, FAILED, INACTIVE
	ToolkitSlug string
}

type authConfig struct {
	ID      string `json:"id"`
	Toolkit struct {
		Slug string `json:"slug"`
	} `json:"toolkit"`
}

// EnsureAuthConfig returns an auth config id for the toolkit, creating a
// Composio-managed one if none exists yet.
func (c *Client) EnsureAuthConfig(ctx context.Context, apiKey, toolkitSlug string) (string, error) {
	var list struct {
		Items []authConfig `json:"items"`
	}
	q := url.Values{}
	q.Set("toolkit_slug", toolkitSlug)
	if err := c.do(ctx, apiKey, http.MethodGet, "/auth_configs?"+q.Encode(), nil, &list); err != nil {
		return "", err
	}
	if len(list.Items) > 0 {
		return list.Items[0].ID, nil
	}

	body := map[string]any{
		"toolkit":     map[string]string{"slug": toolkitSlug},
		"auth_config": map[string]any{"type": "use_composio_managed_auth"},
	}
	var created struct {
		ID         string `json:"id"`
		AuthConfig struct {
			ID string `json:"id"`
		} `json:"auth_config"`
	}
	if err := c.do(ctx, apiKey, http.MethodPost, "/auth_configs", body, &created); err != nil {
		return "", err
	}
	if id := firstNonEmpty(created.ID, created.AuthConfig.ID); id != "" {
		return id, nil
	}
	return "", fmt.Errorf("composio: no auth config id returned")
}

// ListConnections returns a user's connections.
func (c *Client) ListConnections(ctx context.Context, apiKey, userID string) ([]Connection, error) {
	var resp struct {
		Items []struct {
			ID      string `json:"id"`
			Status  string `json:"status"`
			Toolkit struct {
				Slug string `json:"slug"`
			} `json:"toolkit"`
			ToolkitSlug string `json:"toolkit_slug"`
		} `json:"items"`
	}
	q := url.Values{}
	q.Set("user_ids", userID)
	if err := c.do(ctx, apiKey, http.MethodGet, "/connected_accounts?"+q.Encode(), nil, &resp); err != nil {
		return nil, err
	}
	out := make([]Connection, 0, len(resp.Items))
	for _, it := range resp.Items {
		slug := firstNonEmpty(it.Toolkit.Slug, it.ToolkitSlug)
		out = append(out, Connection{ID: it.ID, Status: it.Status, ToolkitSlug: strings.ToLower(slug)})
	}
	return out, nil
}

// InitiateConnection starts a hosted OAuth connection and returns the redirect
// URL the user must visit to authorize. Composio-managed OAuth uses the
// /connected_accounts/link endpoint (the old /connected_accounts create path
// was retired for managed auth configs).
func (c *Client) InitiateConnection(ctx context.Context, apiKey, authConfigID, userID, callbackURL string) (redirectURL, id string, err error) {
	body := map[string]any{
		"auth_config_id": authConfigID,
		"user_id":        userID,
	}
	if callbackURL != "" {
		body["callback_url"] = callbackURL
	}
	var resp struct {
		ID               string `json:"id"`
		ConnectedID      string `json:"connected_account_id"`
		RedirectURL      string `json:"redirect_url"`
		RedirectURLCamel string `json:"redirectUrl"`
		ConnectionData   struct {
			RedirectURL      string `json:"redirect_url"`
			RedirectURLCamel string `json:"redirectUrl"`
		} `json:"connectionData"`
	}
	if err := c.do(ctx, apiKey, http.MethodPost, "/connected_accounts/link", body, &resp); err != nil {
		return "", "", err
	}
	redirect := firstNonEmpty(resp.RedirectURL, resp.RedirectURLCamel, resp.ConnectionData.RedirectURL, resp.ConnectionData.RedirectURLCamel)
	return redirect, firstNonEmpty(resp.ID, resp.ConnectedID), nil
}

// DeleteConnection removes a connection.
func (c *Client) DeleteConnection(ctx context.Context, apiKey, id string) error {
	return c.do(ctx, apiKey, http.MethodDelete, "/connected_accounts/"+url.PathEscape(id), nil, nil)
}

// ToolDef is a Composio tool definition (its slug and JSON-Schema parameters).
type ToolDef struct {
	Slug        string
	Name        string
	Description string
	// Parameters is the tool's input JSON Schema (OpenAI-compatible).
	Parameters json.RawMessage
}

type toolItem struct {
	Slug            string          `json:"slug"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	InputParameters json.RawMessage `json:"input_parameters"`
	Parameters      json.RawMessage `json:"parameters"`
}

func (t toolItem) toDef() ToolDef {
	params := t.InputParameters
	if len(params) == 0 {
		params = t.Parameters
	}
	return ToolDef{Slug: t.Slug, Name: t.Name, Description: t.Description, Parameters: params}
}

// GetTools fetches tool definitions by their exact slugs.
func (c *Client) GetTools(ctx context.Context, apiKey string, slugs []string) ([]ToolDef, error) {
	if len(slugs) == 0 {
		return nil, nil
	}
	q := url.Values{}
	q.Set("tool_slugs", strings.Join(slugs, ","))
	return c.fetchTools(ctx, apiKey, q)
}

// GetToolsByToolkit fetches up to limit tools for a toolkit (fallback when
// specific slugs aren't known).
func (c *Client) GetToolsByToolkit(ctx context.Context, apiKey, toolkitSlug string, limit int) ([]ToolDef, error) {
	q := url.Values{}
	q.Set("toolkit_slug", toolkitSlug)
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	return c.fetchTools(ctx, apiKey, q)
}

func (c *Client) fetchTools(ctx context.Context, apiKey string, q url.Values) ([]ToolDef, error) {
	var resp struct {
		Items []toolItem `json:"items"`
	}
	if err := c.do(ctx, apiKey, http.MethodGet, "/tools?"+q.Encode(), nil, &resp); err != nil {
		return nil, err
	}
	defs := make([]ToolDef, 0, len(resp.Items))
	for _, it := range resp.Items {
		if it.Slug != "" {
			defs = append(defs, it.toDef())
		}
	}
	return defs, nil
}

// ExecuteTool runs a tool for a user and returns a compact result string.
// connectedAccountID targets a specific connected account when non-empty (a user
// may have several accounts connected for the same toolkit); empty lets Composio
// pick the user's default.
func (c *Client) ExecuteTool(ctx context.Context, apiKey, toolSlug, argumentsJSON, userID, connectedAccountID string) (string, error) {
	var args map[string]any
	if strings.TrimSpace(argumentsJSON) != "" {
		if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
	}
	body := map[string]any{"arguments": args, "user_id": userID}
	if connectedAccountID != "" {
		body["connected_account_id"] = connectedAccountID
	}

	var resp struct {
		Data       json.RawMessage `json:"data"`
		Successful *bool           `json:"successful"`
		Success    *bool           `json:"success"`
		Error      string          `json:"error"`
	}
	if err := c.do(ctx, apiKey, http.MethodPost, "/tools/execute/"+url.PathEscape(toolSlug), body, &resp); err != nil {
		return "", err
	}
	ok := (resp.Successful != nil && *resp.Successful) || (resp.Success != nil && *resp.Success)
	if !ok && resp.Error != "" {
		return "", fmt.Errorf("%s", resp.Error)
	}
	if len(resp.Data) > 0 {
		return string(resp.Data), nil
	}
	return "Done.", nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
