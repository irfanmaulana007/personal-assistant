package activity

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/authctx"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/intent"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/store/storetest"
)

func TestActivityLogAndSummarize(t *testing.T) {
	db := storetest.New(t)

	h := New(db, time.UTC, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := authctx.WithUserID(context.Background(), 3)

	for _, typ := range []string{"running", "running", "gym"} {
		res := &intent.ParseResult{
			Capability: intent.CapabilityActivity,
			Action:     intent.ActionActivityLog,
			Entities:   map[string]string{"type": typ, "description": "session"},
		}
		if _, err := h.Handle(ctx, res); err != nil {
			t.Fatalf("log %s: %v", typ, err)
		}
	}

	out, err := h.Handle(ctx, &intent.ParseResult{
		Capability: intent.CapabilityActivity,
		Action:     intent.ActionActivitySummarize,
		Entities:   map[string]string{"days": "30"},
	})
	if err != nil {
		t.Fatalf("summarize: %v", err)
	}
	if !strings.Contains(out, "3 session") || !strings.Contains(out, "running: 2") {
		t.Errorf("summary wrong: %q", out)
	}

	// Other user sees nothing.
	out2, _ := h.Handle(authctx.WithUserID(context.Background(), 9), &intent.ParseResult{
		Capability: intent.CapabilityActivity,
		Action:     intent.ActionActivitySummarize,
		Entities:   map[string]string{"days": "30"},
	})
	if !strings.Contains(out2, "No activities") {
		t.Errorf("user isolation broken: %q", out2)
	}
}
