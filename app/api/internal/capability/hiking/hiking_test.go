package hiking

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/authctx"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/intent"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/store"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/store/storetest"
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

func TestHikeAutoCamped(t *testing.T) {
	db := storetest.New(t)
	h := New(db, time.UTC, slog.New(slog.NewTextHandler(io.Discard, nil)))

	cases := []struct {
		name     string
		entities map[string]string
		want     bool
	}{
		{
			// Days > 1 ⇒ camped inferred true even though not provided.
			name:     "multi_day_infers_camped",
			entities: map[string]string{"mountain": "Rinjani", "days": "3", "nights": "2"},
			want:     true,
		},
		{
			// Nights > 0 with a single day ⇒ camped inferred true.
			name:     "overnight_infers_camped",
			entities: map[string]string{"mountain": "Semeru", "days": "1", "nights": "1"},
			want:     true,
		},
		{
			// Multi-day trip overrides an explicit "no" — you cannot span
			// nights without staying overnight.
			name:     "multi_day_overrides_explicit_no",
			entities: map[string]string{"mountain": "Merbabu", "days": "2", "nights": "1", "camped": "no"},
			want:     true,
		},
		{
			// Single-day hike, camping unspecified ⇒ stays false (question still
			// applies, existing behavior unchanged).
			name:     "single_day_default_false",
			entities: map[string]string{"mountain": "Andong", "days": "1", "nights": "0"},
			want:     false,
		},
		{
			// Single-day hike with explicit yes ⇒ honored.
			name:     "single_day_explicit_yes",
			entities: map[string]string{"mountain": "Prau", "days": "1", "nights": "0", "camped": "yes"},
			want:     true,
		},
	}

	for i, c := range cases {
		// Each case runs as a distinct user so hikes don't collide.
		ctx := authctx.WithUserID(context.Background(), int64(100+i))
		if _, err := h.Handle(ctx, &intent.ParseResult{
			Capability: intent.CapabilityHiking, Action: intent.ActionHikeLog,
			Entities: c.entities,
		}); err != nil {
			t.Fatalf("%s: log: %v", c.name, err)
		}
		hikes, err := db.ListHikes(ctx, int64(100+i), 10)
		if err != nil {
			t.Fatalf("%s: list: %v", c.name, err)
		}
		if len(hikes) != 1 {
			t.Fatalf("%s: expected 1 hike, got %d", c.name, len(hikes))
		}
		if hikes[0].Camped != c.want {
			t.Errorf("%s: camped = %v, want %v", c.name, hikes[0].Camped, c.want)
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

	// Second hike: the mountain typo folds (fuzzy) and the trail case-folds, but
	// the participant is saved exactly as typed — participant names are never
	// fuzzy-remapped onto a different person.
	out, err := h.Handle(ctx, &intent.ParseResult{
		Capability: intent.CapabilityHiking, Action: intent.ActionHikeLog,
		Entities: map[string]string{
			"mountain": "Rinjany", "up_track": "senaru", // typo / case → fold
			"participants": "Andy", // typo of Andi, but kept as its own participant
		},
	})
	if err != nil {
		t.Fatalf("log 2: %v", err)
	}
	if !strings.Contains(out, "matched") {
		t.Errorf("expected a mountain canonicalization note, got: %q", out)
	}

	// Exactly one mountain and Rinjani's trails deduped, but "Andy" is a new,
	// distinct participant (not folded into "Andi").
	mountains, _ := db.ListMountains(ctx, 4)
	if len(mountains) != 1 {
		t.Fatalf("expected 1 mountain, got %d: %+v", len(mountains), mountains)
	}
	hikers, _ := db.ListHikers(ctx, 4)
	if len(hikers) != 3 {
		t.Errorf("expected 3 hikers (Andi, Budi, Andy), got %d: %+v", len(hikers), hikers)
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

// TestParticipantNameSystem exercises the participant name management the card
// asks for: input saved as-is (no fuzzy override), rename, nickname matching,
// and merging duplicates retroactively.
func TestParticipantNameSystem(t *testing.T) {
	db := storetest.New(t)
	h := New(db, time.UTC, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := authctx.WithUserID(context.Background(), 7)

	do := func(action intent.Action, entities map[string]string) string {
		t.Helper()
		out, err := h.Handle(ctx, &intent.ParseResult{
			Capability: intent.CapabilityHiking, Action: action, Entities: entities,
		})
		if err != nil {
			t.Fatalf("%s %+v: %v", action, entities, err)
		}
		return out
	}
	log := func(entities map[string]string) string {
		return do(intent.ActionHikeLog, entities)
	}
	find := func(name string) *store.Hiker {
		t.Helper()
		hs, err := db.ListHikers(ctx, 7)
		if err != nil {
			t.Fatalf("list hikers: %v", err)
		}
		for i := range hs {
			if hs[i].Name == name {
				return &hs[i]
			}
		}
		return nil
	}

	// AC#4: two similarly-spelled but distinct people are BOTH saved as-is — the
	// old fuzzy matcher would have collapsed "Abi" into "Ali".
	log(map[string]string{"mountain": "Merbabu", "participants": "Ali, Abi"})
	if hs, _ := db.ListHikers(ctx, 7); len(hs) != 2 {
		t.Fatalf("expected 2 distinct participants (Ali, Abi), got %d: %+v", len(hs), hs)
	}

	// AC#1: rename a participant; the corrected name is stored exactly, and the
	// system does not auto-remap it back.
	do(intent.ActionHikeParticipantUpdate, map[string]string{"name": "Abi", "new_name": "Abraham"})
	if find("Abraham") == nil {
		t.Fatalf("rename to Abraham did not take effect")
	}
	if find("Abi") != nil {
		t.Errorf("old name Abi should be gone after rename")
	}

	// AC#3: give Abraham explicit nicknames; logging a nickname resolves to him
	// (reported as a match note) instead of creating a new participant.
	do(intent.ActionHikeParticipantUpdate, map[string]string{"name": "Abraham", "nicknames": "Abi, Bram"})
	abraham := find("Abraham")
	if abraham == nil || len(abraham.Nicknames) != 2 {
		t.Fatalf("expected Abraham to have 2 nicknames, got %+v", abraham)
	}
	before, _ := db.ListHikers(ctx, 7)
	out := log(map[string]string{"mountain": "Sindoro", "participants": "Abi"})
	if !strings.Contains(out, "matched") {
		t.Errorf("expected a nickname match note, got %q", out)
	}
	if after, _ := db.ListHikers(ctx, 7); len(after) != len(before) {
		t.Errorf("a nickname match must not create a new participant: before %d, after %d", len(before), len(after))
	}

	// AC#2 & AC#5: the same person recorded twice gets merged retroactively.
	log(map[string]string{"mountain": "Prau", "participants": "Rama"})
	log(map[string]string{"mountain": "Andong", "participants": "Raka"})
	if find("Rama") == nil || find("Raka") == nil {
		t.Fatalf("expected both Rama and Raka to exist before merge")
	}
	do(intent.ActionHikeParticipantMerge, map[string]string{"from": "Raka", "into": "Rama"})
	if find("Raka") != nil {
		t.Errorf("merged-away participant Raka should be gone")
	}
	rama := find("Rama")
	if rama == nil {
		t.Fatalf("surviving participant Rama missing after merge")
	}
	if !containsFold(rama.Nicknames, "Raka") {
		t.Errorf("expected Raka preserved as a nickname of Rama, got %+v", rama.Nicknames)
	}
	// Both the Prau and Andong hikes now attribute to Rama.
	hikes, _ := db.ListHikes(ctx, 7, 50)
	var ramaHikes int
	for _, hk := range hikes {
		for _, p := range hk.Participants {
			if p == "Rama" {
				ramaHikes++
			}
		}
	}
	if ramaHikes != 2 {
		t.Errorf("expected Rama on 2 hikes after merge, got %d", ramaHikes)
	}
}

func containsFold(items []string, want string) bool {
	for _, it := range items {
		if strings.EqualFold(strings.TrimSpace(it), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}
