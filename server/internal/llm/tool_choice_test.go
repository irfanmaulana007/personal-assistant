package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// captureToolChoice runs one completion against a stub server and returns the
// tool_choice field the client sent (empty string if absent).
func captureToolChoice(t *testing.T, call func(c *Client, cfg Config, tools []Tool)) any {
	t.Helper()
	var got struct {
		ToolChoice any `json:"tool_choice"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{}}`))
	}))
	defer srv.Close()

	cfg := Config{APIKey: "k", BaseURL: srv.URL, Model: "test-model"}
	tools := []Tool{{Type: "function", Function: ToolFunction{Name: "t", Parameters: json.RawMessage(`{}`)}}}
	call(NewClient(), cfg, tools)
	return got.ToolChoice
}

func TestCompleteSendsAutoToolChoice(t *testing.T) {
	got := captureToolChoice(t, func(c *Client, cfg Config, tools []Tool) {
		if _, err := c.Complete(context.Background(), cfg, []Message{{Role: "user", Content: "hi"}}, tools); err != nil {
			t.Fatalf("Complete: %v", err)
		}
	})
	if got != "auto" {
		t.Errorf("Complete tool_choice = %v, want auto", got)
	}
}

func TestCompleteRequiringToolSendsRequired(t *testing.T) {
	got := captureToolChoice(t, func(c *Client, cfg Config, tools []Tool) {
		if _, err := c.CompleteRequiringTool(context.Background(), cfg, []Message{{Role: "user", Content: "catat ini"}}, tools); err != nil {
			t.Fatalf("CompleteRequiringTool: %v", err)
		}
	})
	if got != "required" {
		t.Errorf("CompleteRequiringTool tool_choice = %v, want required", got)
	}
}

func TestCompleteWithoutToolsOmitsToolChoice(t *testing.T) {
	var got struct {
		ToolChoice any `json:"tool_choice"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{}}`))
	}))
	defer srv.Close()

	cfg := Config{APIKey: "k", BaseURL: srv.URL, Model: "test-model"}
	if _, err := NewClient().Complete(context.Background(), cfg, []Message{{Role: "user", Content: "hi"}}, nil); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got.ToolChoice != nil {
		t.Errorf("tool_choice should be omitted with no tools, got %v", got.ToolChoice)
	}
}
