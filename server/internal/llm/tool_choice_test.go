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

// TestCompleteRequiringToolFallsBackToAuto verifies the reactive safety net:
// when the provider still rejects tool_choice=required at request time (e.g. a
// thinking model whose alias IsThinkingModel doesn't recognize), the client
// retries with "auto" and the turn completes instead of erroring. Model is a
// non-thinking name so the proactive downgrade doesn't fire — the 400 does.
func TestCompleteRequiringToolFallsBackToAuto(t *testing.T) {
	var choices []any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got struct {
			ToolChoice any `json:"tool_choice"`
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		choices = append(choices, got.ToolChoice)
		w.Header().Set("Content-Type", "application/json")
		if got.ToolChoice == "required" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"Thinking mode does not support this tool_choice","type":"invalid_request_error"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{}}`))
	}))
	defer srv.Close()

	cfg := Config{APIKey: "k", BaseURL: srv.URL, Model: "test-model"}
	tools := []Tool{{Type: "function", Function: ToolFunction{Name: "t", Parameters: json.RawMessage(`{}`)}}}
	res, err := NewClient().CompleteRequiringTool(context.Background(), cfg, []Message{{Role: "user", Content: "catat ini"}}, tools)
	if err != nil {
		t.Fatalf("CompleteRequiringTool: %v", err)
	}
	if res.Message.Content != "ok" {
		t.Errorf("content = %q, want ok", res.Message.Content)
	}
	want := []any{"required", "auto"}
	if len(choices) != len(want) || choices[0] != want[0] || choices[1] != want[1] {
		t.Errorf("tool_choice sequence = %v, want %v", choices, want)
	}
}

// TestCompleteRequiringToolDowngradesForThinkingModel verifies the proactive
// path: a recognized thinking model never receives tool_choice=required — it is
// downgraded to auto before the request is sent, avoiding the 400 round-trip.
func TestCompleteRequiringToolDowngradesForThinkingModel(t *testing.T) {
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

	// A thinking model must never receive tool_choice=required — the provider
	// 400s with "Thinking mode does not support this tool_choice". It downgrades
	// to auto instead.
	cfg := Config{APIKey: "k", BaseURL: srv.URL, Model: "deepseek-reasoner"}
	tools := []Tool{{Type: "function", Function: ToolFunction{Name: "t", Parameters: json.RawMessage(`{}`)}}}
	if _, err := NewClient().CompleteRequiringTool(context.Background(), cfg, []Message{{Role: "user", Content: "catat ini"}}, tools); err != nil {
		t.Fatalf("CompleteRequiringTool: %v", err)
	}
	if got.ToolChoice != "auto" {
		t.Errorf("thinking-model tool_choice = %v, want auto (downgraded from required)", got.ToolChoice)
	}
}

func TestIsThinkingModel(t *testing.T) {
	cases := map[string]bool{
		"deepseek-reasoner":          true,
		"deepseek-v4-flash-thinking": true,
		"DeepSeek-R1":                true,
		"o1":                         true,
		"o3-mini":                    true,
		"o4-mini":                    true,
		"deepseek-v4-flash":          false,
		"deepseek-v4-pro":            false,
		"gpt-4o-mini":                false,
		"":                           false,
	}
	for model, want := range cases {
		if got := IsThinkingModel(model); got != want {
			t.Errorf("IsThinkingModel(%q) = %v, want %v", model, got, want)
		}
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
