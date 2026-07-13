package hiking

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/irfanmaulana007/personal-assistant/server/internal/authctx"
	"github.com/irfanmaulana007/personal-assistant/server/internal/intent"
	"github.com/irfanmaulana007/personal-assistant/server/internal/store/storetest"
)

func TestSimilar(t *testing.T) {
	same := [][2]string{
		{"Rinjani", "Rinjani "},           // trailing space
		{"Rinjani", "rinjani"},            // case
		{"Rinjani", "Rinjany"},            // 1 typo
		{"Mount Semeru", "mount  semeru"}, // whitespace
		{"Andi", "Andy"},                  // 1 typo
	}
	for _, p := range same {
		if !similar(p[0], p[1]) {
			t.Errorf("expected %q ~ %q to be similar", p[0], p[1])
		}
	}
	diff := [][2]string{
		{"Rinjani", "Semeru"},
		{"Andi", "Charlie"},
		{"Senaru", "Torean"},
	}
	for _, p := range diff {
		if similar(p[0], p[1]) {
			t.Errorf("expected %q and %q to be distinct", p[0], p[1])
		}
	}
}

func TestHikeLogDedup(t *testing.T) {
	db := storetest.New(t)

	h := New(db, time.UTC, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := authctx.WithUserID(context.Background(), 4)

	// First hike, establishing canonical names.
	if _, err := h.Handle(ctx, &intent.ParseResult{
		Capability: intent.CapabilityHiking, Action: intent.ActionHikeLog,
		Entities: map[string]string{
			"mountain": "Rinjani", "up_track": "Senaru", "down_track": "Torean",
			"camped": "yes", "days": "3", "nights": "2", "date": "Aug 2",
			"participants": "Andi, Budi",
		},
	}); err != nil {
		t.Fatalf("log 1: %v", err)
	}

	// Second hike with typos/case/spacing that should fold into existing names.
	out, err := h.Handle(ctx, &intent.ParseResult{
		Capability: intent.CapabilityHiking, Action: intent.ActionHikeLog,
		Entities: map[string]string{
			"mountain": "Rinjani ", "up_track": "senaru",
			"participants": "Andy", // typo of Andi
		},
	})
	if err != nil {
		t.Fatalf("log 2: %v", err)
	}
	if !strings.Contains(out, "matched") {
		t.Errorf("expected a canonicalization note, got: %q", out)
	}

	// Exactly one mountain, two participants, and Rinjani's trails deduped.
	mountains, _ := db.ListMountains(ctx, 4)
	if len(mountains) != 1 {
		t.Fatalf("expected 1 mountain, got %d: %+v", len(mountains), mountains)
	}
	hikers, _ := db.ListHikers(ctx, 4)
	if len(hikers) != 2 {
		t.Errorf("expected 2 hikers (Andi, Budi), got %d: %+v", len(hikers), hikers)
	}
	tracks, _ := db.ListTracks(ctx, 4, mountains[0].ID)
	if len(tracks) != 2 {
		t.Errorf("expected 2 trails (Senaru, Torean), got %d: %+v", len(tracks), tracks)
	}

	// Summary reflects 2 hikes, 1 mountain.
	sum, err := h.Handle(ctx, &intent.ParseResult{
		Capability: intent.CapabilityHiking, Action: intent.ActionHikeSummary,
		Entities: map[string]string{},
	})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if !strings.Contains(sum, "Your hikes* (2)") || !strings.Contains(sum, "1 mountain") {
		t.Errorf("summary wrong: %q", sum)
	}

	// User isolation: another user has no hikes.
	sum2, _ := h.Handle(authctx.WithUserID(context.Background(), 99), &intent.ParseResult{
		Capability: intent.CapabilityHiking, Action: intent.ActionHikeSummary, Entities: map[string]string{},
	})
	if !strings.Contains(sum2, "haven't logged any hikes") {
		t.Errorf("user isolation broken: %q", sum2)
	}
}
