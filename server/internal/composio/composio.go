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
// URL the user must visit to authorize.
func (c *Client) InitiateConnection(ctx context.Context, apiKey, authConfigID, userID, callbackURL string) (redirectURL, id string, err error) {
	connection := map[string]any{"user_id": userID}
	if callbackURL != "" {
		connection["callback_url"] = callbackURL
	}
	body := map[string]any{
		"auth_config": map[string]string{"id": authConfigID},
		"connection":  connection,
	}
	var resp struct {
		ID               string `json:"id"`
		RedirectURL      string `json:"redirect_url"`
		RedirectURLCamel string `json:"redirectUrl"`
		ConnectionData   struct {
			RedirectURL      string `json:"redirect_url"`
			RedirectURLCamel string `json:"redirectUrl"`
		} `json:"connectionData"`
	}
	if err := c.do(ctx, apiKey, http.MethodPost, "/connected_accounts", body, &resp); err != nil {
		return "", "", err
	}
	redirect := firstNonEmpty(resp.RedirectURL, resp.RedirectURLCamel, resp.ConnectionData.RedirectURL, resp.ConnectionData.RedirectURLCamel)
	return redirect, resp.ID, nil
}

// DeleteConnection removes a connection.
func (c *Client) DeleteConnection(ctx context.Context, apiKey, id string) error {
	return c.do(ctx, apiKey, http.MethodDelete, "/connected_accounts/"+url.PathEscape(id), nil, nil)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
