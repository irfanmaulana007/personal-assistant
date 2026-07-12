// Package llm provides a minimal client for OpenAI-compatible chat completion
// APIs (DeepSeek, OpenAI, and other compatible providers). It supports
// tool-calling and reports token usage.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Default provider settings (DeepSeek, OpenAI-compatible).
const (
	DefaultBaseURL = "https://api.deepseek.com"
	DefaultModel   = "deepseek-v4-flash"
)

// thinkingModelMarkers are lower-cased substrings that identify a
// thinking/reasoning model alias. Providers that expose extended thinking
// (DeepSeek's reasoner alias, OpenAI's o-series, "*-thinking"/"*-r1" variants)
// reject any forced tool_choice — only "auto"/"none" are accepted with
// thinking on. IsThinkingModel is used to downgrade a "required" tool_choice to
// "auto" so those turns don't 400 (see complete). Keep this list in sync with
// the thinking aliases noted in pricing.go.
var thinkingModelMarkers = []string{"reasoner", "thinking", "-r1", "-think"}

// IsThinkingModel reports whether model is a thinking/reasoning variant that
// does not support a forced tool_choice. Matching is case-insensitive and by
// substring so agglutinated/prefixed aliases (e.g. "deepseek-reasoner",
// "deepseek-v4-flash-thinking") are covered.
func IsThinkingModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return false
	}
	for _, marker := range thinkingModelMarkers {
		if strings.Contains(m, marker) {
			return true
		}
	}
	// OpenAI o-series (o1, o1-mini, o3, o3-mini, o4-…) are thinking models too.
	if strings.HasPrefix(m, "o1") || strings.HasPrefix(m, "o3") || strings.HasPrefix(m, "o4") {
		return true
	}
	return false
}

// Config holds the runtime configuration for the LLM provider. It is resolved
// per request from the settings store (with config/env fallbacks).
type Config struct {
	APIKey  string
	BaseURL string
	Model   string
	// Vision reports whether the configured model can ingest image content
	// parts. When false, callers must not attach image_url blocks — most
	// OpenAI-compatible text models (e.g. deepseek-v4-flash) reject them with a
	// deserialization 400.
	Vision bool
}

// Message is a single chat message in the OpenAI-compatible format.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`

	// ContentParts, when set, is sent as a multimodal content array (text +
	// images) instead of the plain Content string. Not populated on decode.
	ContentParts []ContentPart `json:"-"`
}

// ContentPart is one element of a multimodal message content array.
type ContentPart struct {
	Type     string    `json:"type"` // "text" or "image_url"
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL carries an image reference (a data: URL is supported).
type ImageURL struct {
	URL string `json:"url"`
}

// MarshalJSON serializes content as a parts array when ContentParts is set,
// otherwise as the plain string (preserving omitempty for tool-call messages).
func (m Message) MarshalJSON() ([]byte, error) {
	type alias struct {
		Role       string     `json:"role"`
		Content    any        `json:"content,omitempty"`
		ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
		ToolCallID string     `json:"tool_call_id,omitempty"`
		Name       string     `json:"name,omitempty"`
	}
	a := alias{Role: m.Role, ToolCalls: m.ToolCalls, ToolCallID: m.ToolCallID, Name: m.Name}
	if len(m.ContentParts) > 0 {
		a.Content = m.ContentParts
	} else if m.Content != "" {
		a.Content = m.Content
	}
	return json.Marshal(a)
}

// Tool describes a callable function exposed to the model.
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction is the schema of a tool the model may call.
type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ToolCall is a function invocation requested by the model.
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction carries the called tool's name and raw JSON arguments.
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Usage reports token counts for a completion.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type chatRequest struct {
	Model      string    `json:"model"`
	Messages   []Message `json:"messages"`
	Tools      []Tool    `json:"tools,omitempty"`
	ToolChoice string    `json:"tool_choice,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message      Message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
	Usage Usage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// CompletionResult is the parsed outcome of a chat completion request.
type CompletionResult struct {
	Message      Message
	Usage        Usage
	FinishReason string
}

// Client is a minimal OpenAI-compatible chat completions client.
type Client struct {
	http *http.Client
}

// NewClient returns a client with a sensible request timeout.
func NewClient() *Client {
	return &Client{http: &http.Client{Timeout: 60 * time.Second}}
}

// Complete sends a chat completion request and returns the assistant message
// plus token usage. If tools are provided, tool_choice is set to "auto".
func (c *Client) Complete(ctx context.Context, cfg Config, messages []Message, tools []Tool) (*CompletionResult, error) {
	choice := ""
	if len(tools) > 0 {
		choice = "auto"
	}
	return c.complete(ctx, cfg, messages, tools, choice)
}

// CompleteRequiringTool behaves like Complete but sets tool_choice to
// "required", forcing the model to call at least one of the provided tools
// instead of returning a text-only reply. It is used for save-intent turns
// where a text-only reply would be a fabricated "done ✅" confirmation with no
// underlying write. tools must be non-empty.
func (c *Client) CompleteRequiringTool(ctx context.Context, cfg Config, messages []Message, tools []Tool) (*CompletionResult, error) {
	return c.complete(ctx, cfg, messages, tools, "required")
}

func (c *Client) complete(ctx context.Context, cfg Config, messages []Message, tools []Tool, toolChoice string) (*CompletionResult, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("no API key configured")
	}

	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	model := cfg.Model
	if model == "" {
		model = DefaultModel
	}

	// Thinking/reasoning models reject a forced tool_choice ("required" or a
	// specific tool) — the provider returns 400 "Thinking mode does not support
	// this tool_choice". Downgrade to "auto" so save-intent turns still run; the
	// prompt-level guard remains the fallback against a fabricated confirmation.
	if toolChoice == "required" && IsThinkingModel(model) {
		slog.Warn("downgrading forced tool_choice to auto for thinking model",
			"model", model, "from", toolChoice)
		toolChoice = "auto"
	}

	reqBody := chatRequest{Model: model, Messages: messages, Tools: tools}
	if len(tools) > 0 {
		reqBody.ToolChoice = toolChoice
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call llm: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var parsed chatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse response (status %d): %w", resp.StatusCode, err)
	}

	if resp.StatusCode != http.StatusOK {
		if parsed.Error != nil {
			return nil, fmt.Errorf("llm error (status %d): %s", resp.StatusCode, parsed.Error.Message)
		}
		return nil, fmt.Errorf("llm error (status %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("llm returned no choices")
	}

	return &CompletionResult{Message: parsed.Choices[0].Message, Usage: parsed.Usage, FinishReason: parsed.Choices[0].FinishReason}, nil
}
