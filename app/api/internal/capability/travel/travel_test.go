package travel

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

func TestTravelFlow(t *testing.T) {
	db := storetest.New(t)

	h := New(db, time.UTC, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := authctx.WithUserID(context.Background(), 5)

	// Start a trip with a budget.
	if _, err := h.Handle(ctx, &intent.ParseResult{
		Capability: intent.CapabilityTravel, Action: intent.ActionTripCreate,
		Entities: map[string]string{"name": "Bali", "destination": "Bali", "budget": "1000", "currency": "USD"},
	}); err != nil {
		t.Fatalf("create trip: %v", err)
	}

	// Add two expenses to the active trip.
	for _, e := range []map[string]string{
		{"amount": "200", "category": "hotel"},
		{"amount": "50", "category": "food"},
	} {
		e["trip"] = "" // active
		if _, err := h.Handle(ctx, &intent.ParseResult{
			Capability: intent.CapabilityTravel, Action: intent.ActionExpenseAdd, Entities: e,
		}); err != nil {
			t.Fatalf("add expense: %v", err)
		}
	}

	out, err := h.Handle(ctx, &intent.ParseResult{
		Capability: intent.CapabilityTravel, Action: intent.ActionTripSummary,
		Entities: map[string]string{},
	})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if !strings.Contains(out, "2 expense") || !strings.Contains(out, "USD 250") || !strings.Contains(out, "hotel") {
		t.Errorf("summary wrong: %q", out)
	}
	if !strings.Contains(out, "USD 750 left") {
		t.Errorf("budget remaining wrong: %q", out)
	}
}
