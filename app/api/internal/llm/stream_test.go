package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// sseServer returns a test server that replies with the given raw SSE body and
// the client's Config pointed at it.
func sseServer(t *testing.T, body string) (*httptest.Server, Config) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(body))
	}))
	return srv, Config{APIKey: "test-key", BaseURL: srv.URL, Model: "test-model"}
}

func TestCompleteStreamAccumulatesTextAndUsage(t *testing.T) {
	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"Hello"}}]}`,
		`data: {"choices":[{"delta":{"content":", world"}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		`data: {"choices":[],"usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10}}`,
		`data: [DONE]`,
		"",
	}, "\n\n")

	srv, cfg := sseServer(t, body)
	defer srv.Close()

	var deltas []string
	res, err := NewClient().CompleteStream(context.Background(), cfg, nil, nil, func(s string) {
		deltas = append(deltas, s)
	})
	if err != nil {
		t.Fatalf("CompleteStream: %v", err)
	}
	if res.Message.Content != "Hello, world" {
		t.Errorf("content = %q, want %q", res.Message.Content, "Hello, world")
	}
	if strings.Join(deltas, "|") != "Hello|, world" {
		t.Errorf("deltas = %v, want [Hello , world]", deltas)
	}
	if res.Usage.TotalTokens != 10 {
		t.Errorf("total tokens = %d, want 10", res.Usage.TotalTokens)
	}
	if res.FinishReason != "stop" {
		t.Errorf("finish reason = %q, want stop", res.FinishReason)
	}
}

func TestCompleteStreamReassemblesToolCalls(t *testing.T) {
	// A tool call whose name arrives in the first fragment and whose JSON
	// arguments are split across several chunks — the OpenAI-compatible shape.
	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"reminder_schedule","arguments":"{\"te"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"xt\":\"buy milk\"}"}}]}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
		"",
	}, "\n\n")

	srv, cfg := sseServer(t, body)
	defer srv.Close()

	res, err := NewClient().CompleteStream(context.Background(), cfg, nil, nil, nil)
	if err != nil {
		t.Fatalf("CompleteStream: %v", err)
	}
	if len(res.Message.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(res.Message.ToolCalls))
	}
	tc := res.Message.ToolCalls[0]
	if tc.ID != "call_1" || tc.Function.Name != "reminder_schedule" {
		t.Errorf("tool call id/name wrong: %+v", tc)
	}
	if tc.Function.Arguments != `{"text":"buy milk"}` {
		t.Errorf("arguments = %q, want %q", tc.Function.Arguments, `{"text":"buy milk"}`)
	}
}

func TestCompleteStreamPropagatesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer srv.Close()
	cfg := Config{APIKey: "x", BaseURL: srv.URL, Model: "m"}

	_, err := NewClient().CompleteStream(context.Background(), cfg, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "bad key") {
		t.Fatalf("expected error mentioning provider message, got %v", err)
	}
}
