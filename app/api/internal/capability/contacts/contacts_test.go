package contacts

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/authctx"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/intent"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/store/storetest"
)

func TestContactsAddAndSearch(t *testing.T) {
	db := storetest.New(t)

	h := New(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := authctx.WithUserID(context.Background(), 7)

	// Add a contact.
	addRes := &intent.ParseResult{
		Capability: intent.CapabilityContact,
		Action:     intent.ActionContactAdd,
		Entities:   map[string]string{"name": "John Doe", "phone": "0812345678", "note": "plumber"},
	}
	if _, err := h.Handle(ctx, addRes); err != nil {
		t.Fatalf("add: %v", err)
	}

	// Search finds it.
	searchRes := &intent.ParseResult{
		Capability: intent.CapabilityContact,
		Action:     intent.ActionContactSearch,
		Entities:   map[string]string{"query": "john"},
	}
	out, err := h.Handle(ctx, searchRes)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if !strings.Contains(out, "John Doe") || !strings.Contains(out, "0812345678") {
		t.Errorf("search result missing contact: %q", out)
	}

	// Search by a different user finds nothing (per-user isolation).
	otherCtx := authctx.WithUserID(context.Background(), 8)
	out2, err := h.Handle(otherCtx, searchRes)
	if err != nil {
		t.Fatalf("search other: %v", err)
	}
	if strings.Contains(out2, "John Doe") {
		t.Errorf("user isolation broken: %q", out2)
	}
}
