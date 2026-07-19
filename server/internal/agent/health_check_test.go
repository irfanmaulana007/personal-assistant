package agent

import (
	"context"
	"testing"
)

// A bare "ping" must return "pong" immediately, before any LLM config, tool,
// or memory processing. A zero-value Agent (all deps nil) proves the short
// circuit never touches those dependencies — if it did, this would panic.
func TestPingReturnsPongWithoutProcessing(t *testing.T) {
	a := &Agent{}
	for _, in := range []string{"ping", "Ping", "PING", "  ping  "} {
		res, err := a.Run(context.Background(), in, nil, "")
		if err != nil {
			t.Fatalf("Run(%q) returned error: %v", in, err)
		}
		if res.Reply != "pong" {
			t.Errorf("Run(%q) reply = %q, want %q", in, res.Reply, "pong")
		}
		if res.Model != "" || res.Usage.TotalTokens != 0 || len(res.Steps) != 0 {
			t.Errorf("Run(%q) did processing: model=%q usage=%d steps=%d",
				in, res.Model, res.Usage.TotalTokens, len(res.Steps))
		}
	}
}

// A message that merely contains "ping" is a normal message, not a health
// check — it must fall through to real processing (which, with a nil settings
// dependency, panics rather than short-circuiting).
func TestPingSubstringIsNotHealthCheck(t *testing.T) {
	a := &Agent{}
	defer func() {
		if recover() == nil {
			t.Error(`"ping me at noon" should not be treated as a health check`)
		}
	}()
	_, _ = a.Run(context.Background(), "ping me at noon", nil, "")
}

// The streaming path must also deliver "pong" via the onDelta callback so a
// token-by-token UI shows the health-check reply, not an empty stream.
func TestPingStreamsPongViaOnDelta(t *testing.T) {
	a := &Agent{}
	var streamed string
	res, err := a.RunStream(context.Background(), "ping", nil, "", func(d string) {
		streamed += d
	})
	if err != nil {
		t.Fatalf("RunStream returned error: %v", err)
	}
	if res.Reply != "pong" {
		t.Errorf("RunStream reply = %q, want %q", res.Reply, "pong")
	}
	if streamed != "pong" {
		t.Errorf("onDelta received %q, want %q", streamed, "pong")
	}
}
