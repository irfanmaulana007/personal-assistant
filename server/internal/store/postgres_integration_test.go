//go:build integration

// Package store integration tests for the PostgreSQL backend. These require
// Docker (a real postgres:17 is started via testcontainers) and are excluded
// from the default build; run with:
//
//	go test -tags 'sqlite_fts5 integration' ./internal/store/ -run Postgres
package store

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func newTestPostgres(t *testing.T) *PostgresStore {
	t.Helper()
	ctx := context.Background()

	pgC, err := postgres.Run(ctx, "postgres:17",
		postgres.WithDatabase("assistant"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(90*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = pgC.Terminate(ctx) })

	dsn, err := pgC.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	s, err := NewPostgres(ctx, dsn)
	if err != nil {
		t.Fatalf("NewPostgres (migrate + seed): %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestPostgresUsersAndSettings(t *testing.T) {
	s := newTestPostgres(t)
	ctx := context.Background()

	u, err := s.CreateUser(ctx, "a@example.com", "hash", "admin")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if u.ID == 0 {
		t.Fatal("expected non-zero user id from RETURNING")
	}

	got, err := s.GetUserByEmail(ctx, "a@example.com")
	if err != nil || got == nil {
		t.Fatalf("get user by email: %v (got %v)", err, got)
	}
	if got.Role != "admin" {
		t.Fatalf("role = %q, want admin", got.Role)
	}

	// No-rows contract: unknown email returns (nil, nil).
	missing, err := s.GetUserByEmail(ctx, "nobody@example.com")
	if err != nil || missing != nil {
		t.Fatalf("expected (nil,nil) for missing user, got (%v,%v)", missing, err)
	}

	// Settings + tokens (BYTEA round-trip).
	if err := s.SetSetting(ctx, "k", []byte("v")); err != nil {
		t.Fatalf("set setting: %v", err)
	}
	if v, err := s.GetSetting(ctx, "k"); err != nil || string(v) != "v" {
		t.Fatalf("get setting = %q, %v", v, err)
	}
	if err := s.SaveToken(ctx, "google", []byte{0x01, 0x02}); err != nil {
		t.Fatalf("save token: %v", err)
	}
	if v, err := s.GetToken(ctx, "google"); err != nil || len(v) != 2 {
		t.Fatalf("get token = %v, %v", v, err)
	}
}

func TestPostgresSkillsSeeded(t *testing.T) {
	s := newTestPostgres(t)
	ctx := context.Background()

	skills, err := s.ListSkills(ctx)
	if err != nil {
		t.Fatalf("list skills: %v", err)
	}
	if len(skills) == 0 {
		t.Fatal("expected seedSkills to populate skills, got none")
	}

	// seedSkills is idempotent — a second NewPostgres-equivalent call must not error.
	if err := s.seedSkills(ctx); err != nil {
		t.Fatalf("second seedSkills: %v", err)
	}
	again, err := s.ListSkills(ctx)
	if err != nil || len(again) != len(skills) {
		t.Fatalf("skills count changed after re-seed: %d -> %d (%v)", len(skills), len(again), err)
	}
}

func TestPostgresMemoryFullTextSearch(t *testing.T) {
	s := newTestPostgres(t)
	ctx := context.Background()

	u, err := s.CreateUser(ctx, "m@example.com", "h", "member")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := s.CreateMemory(ctx, u.ID, "I love hiking mountains in summer", ""); err != nil {
		t.Fatalf("create memory: %v", err)
	}
	if _, err := s.CreateMemory(ctx, u.ID, "My favorite food is ramen", ""); err != nil {
		t.Fatalf("create memory: %v", err)
	}

	// tsvector search: raw text with punctuation must match on tokens.
	hits, err := s.SearchMemories(ctx, u.ID, "hiking, mountains!", 10)
	if err != nil {
		t.Fatalf("search memories: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hiking memory, got %d", len(hits))
	}

	// Empty/whitespace query short-circuits to no results, no error.
	empty, err := s.SearchMemories(ctx, u.ID, "   ", 10)
	if err != nil || len(empty) != 0 {
		t.Fatalf("expected no results for blank query, got %d (%v)", len(empty), err)
	}

	// User scoping: another user sees nothing.
	other, _ := s.CreateUser(ctx, "n@example.com", "h", "member")
	none, err := s.SearchMemories(ctx, other.ID, "hiking", 10)
	if err != nil || len(none) != 0 {
		t.Fatalf("cross-user leak: got %d (%v)", len(none), err)
	}
}

func TestPostgresNotesFullTextSearch(t *testing.T) {
	s := newTestPostgres(t)
	ctx := context.Background()

	u, _ := s.CreateUser(ctx, "notes@example.com", "h", "member")
	if _, err := s.CreateNote(ctx, u.ID, "Trip plan", "hiking gear checklist for the mountains", "travel"); err != nil {
		t.Fatalf("create note: %v", err)
	}

	// websearch_to_tsquery ANDs bare terms; punctuation is ignored. Both tokens
	// are present in the note's title/content/tags tsvector.
	notes, err := s.SearchNotes(ctx, u.ID, "hiking, gear!")
	if err != nil {
		t.Fatalf("search notes: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(notes))
	}

	// A term absent from the document yields no match (AND semantics).
	none, err := s.SearchNotes(ctx, u.ID, "hiking submarine")
	if err != nil || len(none) != 0 {
		t.Fatalf("expected no match when a term is absent, got %d (%v)", len(none), err)
	}
}

func TestPostgresRemindersRoundTrip(t *testing.T) {
	s := newTestPostgres(t)
	ctx := context.Background()

	u, _ := s.CreateUser(ctx, "r@example.com", "h", "member")
	in := ReminderInput{
		Title:      "Standup",
		RepeatMode: "weekly",
		Times:      []string{"09:00", "17:30"},
		Weekdays:   []int{1, 3, 5},
		Enabled:    true,
	}
	rem, err := s.CreateReminder(ctx, u.ID, in)
	if err != nil {
		t.Fatalf("create reminder: %v", err)
	}
	if rem.ID == 0 {
		t.Fatal("expected reminder id")
	}

	got, err := s.GetReminder(ctx, u.ID, rem.ID)
	if err != nil || got == nil {
		t.Fatalf("get reminder: %v", err)
	}
	// Slice serialization round-trip through the CSV TEXT columns.
	if len(got.Times) != 2 || got.Times[0] != "09:00" {
		t.Fatalf("times not round-tripped: %v", got.Times)
	}
	if len(got.Weekdays) != 3 || got.Weekdays[1] != 3 {
		t.Fatalf("weekdays not round-tripped: %v", got.Weekdays)
	}

	list, err := s.ListReminders(ctx, u.ID, true)
	if err != nil || len(list) != 1 {
		t.Fatalf("list reminders = %d (%v)", len(list), err)
	}
}

func TestPostgresContactsBucketListTravelHiking(t *testing.T) {
	s := newTestPostgres(t)
	ctx := context.Background()
	u, _ := s.CreateUser(ctx, "c@example.com", "h", "member")

	// Contacts (ILIKE search).
	if _, err := s.CreateContact(ctx, u.ID, "Alice Smith", "123", "alice@x.com", ""); err != nil {
		t.Fatalf("create contact: %v", err)
	}
	found, err := s.SearchContacts(ctx, u.ID, "alice")
	if err != nil || len(found) != 1 {
		t.Fatalf("search contacts = %d (%v)", len(found), err)
	}

	// Bucket list (BOOLEAN done + nullable done_at + nullable resolution_year).
	g, err := s.CreateBucketItem(ctx, u.ID, "Learn Go", "", "", CategorySelfImprovement, nil)
	if err != nil {
		t.Fatalf("create bucket item: %v", err)
	}
	if err := s.SetBucketItemDone(ctx, u.ID, g.ID, true); err != nil {
		t.Fatalf("set done: %v", err)
	}
	year := 2026
	if err := s.SetBucketItemResolution(ctx, u.ID, g.ID, &year); err != nil {
		t.Fatalf("set resolution: %v", err)
	}
	items, _ := s.ListBucketItems(ctx, u.ID)
	if len(items) != 1 || !items[0].Done || items[0].DoneAt == nil {
		t.Fatalf("bucket item done state wrong: %+v", items)
	}
	if items[0].Category != CategorySelfImprovement || items[0].ResolutionYear == nil || *items[0].ResolutionYear != 2026 {
		t.Fatalf("bucket item category/resolution wrong: %+v", items)
	}

	// Travel (DOUBLE PRECISION + BOOLEAN active).
	trip, err := s.CreateTrip(ctx, u.ID, "Japan", "Tokyo", "JPY", 5000)
	if err != nil {
		t.Fatalf("create trip: %v", err)
	}
	active, err := s.ActiveTrip(ctx, u.ID)
	if err != nil || active == nil || active.ID != trip.ID {
		t.Fatalf("active trip: %v (%v)", active, err)
	}
	if _, err := s.AddExpense(ctx, u.ID, trip.ID, 42.5, "JPY", "food", "ramen", time.Now().UTC()); err != nil {
		t.Fatalf("add expense: %v", err)
	}
	exps, _ := s.ListTripExpenses(ctx, u.ID, trip.ID)
	if len(exps) != 1 || exps[0].Amount != 42.5 {
		t.Fatalf("expenses wrong: %+v", exps)
	}

	// Hiking (BOOLEAN camped + participant string_agg).
	m, err := s.CreateMountain(ctx, u.ID, "Rinjani")
	if err != nil {
		t.Fatalf("create mountain: %v", err)
	}
	p, _ := s.CreateHiker(ctx, u.ID, "Bob")
	hikeID, err := s.CreateHike(ctx, u.ID, &Hike{MountainID: m.ID, Camped: true, Days: 3, Nights: 2, HikedOn: time.Now().UTC()})
	if err != nil {
		t.Fatalf("create hike: %v", err)
	}
	if err := s.AddHikeParticipant(ctx, hikeID, p.ID); err != nil {
		t.Fatalf("add participant: %v", err)
	}
	// Idempotent (ON CONFLICT DO NOTHING).
	if err := s.AddHikeParticipant(ctx, hikeID, p.ID); err != nil {
		t.Fatalf("add participant twice: %v", err)
	}
	hikes, err := s.ListHikes(ctx, u.ID, 10)
	if err != nil || len(hikes) != 1 {
		t.Fatalf("list hikes = %d (%v)", len(hikes), err)
	}
	if hikes[0].Mountain != "Rinjani" || !hikes[0].Camped || len(hikes[0].Participants) != 1 {
		t.Fatalf("hike detail wrong: %+v", hikes[0])
	}
}

func TestPostgresPersonaAndPrices(t *testing.T) {
	s := newTestPostgres(t)
	ctx := context.Background()
	u, _ := s.CreateUser(ctx, "p@example.com", "h", "member")

	// Persona default on no rows, then upsert.
	def, err := s.GetUserPersona(ctx, u.ID)
	if err != nil {
		t.Fatalf("get default persona: %v", err)
	}
	_ = def
	if err := s.SetUserPersona(ctx, u.ID, UserPersona{Tone: "casual", Name: "Ada"}); err != nil {
		t.Fatalf("set persona: %v", err)
	}
	got, err := s.GetUserPersona(ctx, u.ID)
	if err != nil || got.Tone != "casual" || got.Name != "Ada" {
		t.Fatalf("persona upsert wrong: %+v (%v)", got, err)
	}

	// Model prices upsert.
	if err := s.UpsertModelPrice(ctx, ModelPrice{Model: "claude-opus-4-8", InputPer1M: 5, OutputPer1M: 25}); err != nil {
		t.Fatalf("upsert price: %v", err)
	}
	if err := s.UpsertModelPrice(ctx, ModelPrice{Model: "claude-opus-4-8", InputPer1M: 6, OutputPer1M: 30}); err != nil {
		t.Fatalf("re-upsert price: %v", err)
	}
	prices, _ := s.ListModelPrices(ctx)
	if len(prices) != 1 || prices[0].InputPer1M != 6 {
		t.Fatalf("prices wrong: %+v", prices)
	}
}
