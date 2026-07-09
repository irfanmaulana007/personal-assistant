package memory

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/irfanmaulana007/personal-assistant/server/internal/authctx"
	"github.com/irfanmaulana007/personal-assistant/server/internal/intent"
	"github.com/irfanmaulana007/personal-assistant/server/internal/memory"
	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

func TestRememberRecall(t *testing.T) {
	db, err := store.NewSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	h := New(memory.New(db), slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := authctx.WithUserID(context.Background(), 7)

	// Remember a fact.
	if _, err := h.Handle(ctx, &intent.ParseResult{
		Capability: intent.CapabilityMemory, Action: intent.ActionMemoryRemember,
		Entities: map[string]string{"content": "User's Japan trip budget is Rp35-50 million, solo, 14 days"},
	}); err != nil {
		t.Fatalf("remember: %v", err)
	}

	// Recall finds it — even with messy punctuation/quotes in the query.
	out, err := h.Handle(ctx, &intent.ParseResult{
		Capability: intent.CapabilityMemory, Action: intent.ActionMemoryRecall,
		Entities: map[string]string{"query": `what's the "japan" budget??`},
	})
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if !strings.Contains(out, "Japan trip budget") {
		t.Errorf("recall did not find the memory: %q", out)
	}

	// Another user recalls nothing (per-user isolation).
	out2, _ := h.Handle(authctx.WithUserID(context.Background(), 99), &intent.ParseResult{
		Capability: intent.CapabilityMemory, Action: intent.ActionMemoryRecall,
		Entities: map[string]string{"query": "japan budget"},
	})
	if strings.Contains(out2, "Japan trip budget") {
		t.Errorf("user isolation broken: %q", out2)
	}
}
