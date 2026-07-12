//go:build integration

// ETL round-trip integration test. Requires Docker (postgres:17 + mongo:7 via
// testcontainers). Run with:
//
//	go test -tags 'sqlite_fts5 integration' ./internal/store/ -run ETL
package store

import (
	"context"
	"testing"
	"time"
)

func TestETLSQLiteToHybridRoundTrip(t *testing.T) {
	ctx := context.Background()
	src := newTestStore(t) // *SQLiteStore, schema migrated + skills seeded
	pg := newTestPostgres(t)
	mongo := newTestMongo(t)
	dst := NewHybrid(pg, mongo)

	// --- Populate the SQLite source across every migrated table ---
	u, err := src.CreateUser(ctx, "etl@example.com", "hash", "admin")
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}

	if _, err := src.CreateContact(ctx, u.ID, "Alice", "1", "alice@x.com", "friend"); err != nil {
		t.Fatalf("seed contact: %v", err)
	}
	g, _ := src.CreateBucketItem(ctx, u.ID, "Learn Go", "", "", CategorySelfImprovement, nil)
	_ = src.SetBucketItemDone(ctx, u.ID, g.ID, true)

	trip, _ := src.CreateTrip(ctx, u.ID, "Japan", "Tokyo", "JPY", 5000)
	if _, err := src.AddExpense(ctx, u.ID, trip.ID, 42.5, "JPY", "food", "ramen", time.Now().UTC()); err != nil {
		t.Fatalf("seed expense: %v", err)
	}

	mtn, _ := src.CreateMountain(ctx, u.ID, "Rinjani")
	trk, _ := src.CreateTrack(ctx, u.ID, mtn.ID, "Senaru")
	hiker, _ := src.CreateHiker(ctx, u.ID, "Bob")
	hikeID, _ := src.CreateHike(ctx, u.ID, &Hike{MountainID: mtn.ID, UpTrackID: trk.ID, Camped: true, Days: 3, Nights: 2, HikedOn: time.Now().UTC()})
	_ = src.AddHikeParticipant(ctx, hikeID, hiker.ID)

	// Enable a seeded skill -> user_skills row (exercises the skills key-mapping).
	skills, _ := src.ListSkills(ctx)
	if len(skills) == 0 {
		t.Fatal("expected seeded skills in source")
	}
	if err := src.SetSkillEnabled(ctx, u.ID, skills[0].ID, true); err != nil {
		t.Fatalf("enable skill: %v", err)
	}
	enabledSkillKey := skills[0].Key

	if _, err := src.CreateReminder(ctx, u.ID, ReminderInput{Title: "standup", RepeatMode: "weekly", Times: []string{"09:00"}, Weekdays: []int{1, 3}, Enabled: true}); err != nil {
		t.Fatalf("seed reminder: %v", err)
	}
	_ = src.SetUserPersona(ctx, u.ID, UserPersona{Tone: "casual", Name: "Ada"})
	if _, err := src.CreateMemory(ctx, u.ID, "loves hiking", ""); err != nil {
		t.Fatalf("seed memory: %v", err)
	}
	if _, err := src.CreateNote(ctx, u.ID, "trip", "hiking gear", "travel"); err != nil {
		t.Fatalf("seed note: %v", err)
	}
	_ = src.SaveToken(ctx, "google", []byte{1, 2, 3})
	_ = src.SetSetting(ctx, "llm_model", []byte("opus"))
	_ = src.UpsertModelPrice(ctx, ModelPrice{Model: "opus", InputPer1M: 5, OutputPer1M: 25})

	// Logs.
	_ = src.LogMessage(ctx, &MessageLog{UserID: u.ID, Platform: "web", Direction: "in", Body: "hi"})
	_, _ = src.CreateActivity(ctx, u.ID, "run", "5k", time.Now().UTC(), "chat")
	_ = src.LogToolUsage(ctx, u.ID, "search", "web")
	traceID, _ := src.CreateTrace(ctx, &Trace{UserID: u.ID, Platform: "web", Model: "opus", TotalTokens: 200, LatencyMs: 30, Status: "ok", Output: "hello"})
	_ = src.SaveTraceScore(ctx, &TraceScore{TraceID: traceID, Accuracy: 5, Helpfulness: 4, Safety: 5, Overall: 4.67, JudgeModel: "judge"})

	// --- Migrate ---
	report, err := MigrateSQLiteToHybrid(ctx, src, dst, MigrateOptions{Truncate: true})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := report.Verify(); err != nil {
		t.Fatalf("count verification failed: %v", err)
	}

	// --- Verify data + id/reference preservation through the hybrid store ---
	gotUser, err := dst.GetUserByEmail(ctx, "etl@example.com")
	if err != nil || gotUser == nil {
		t.Fatalf("user not migrated: %v", err)
	}
	if gotUser.ID != u.ID {
		t.Fatalf("user id not preserved: src %d dst %d", u.ID, gotUser.ID)
	}

	// trip_id FK preserved: the expense resolves under the original trip id.
	exps, err := dst.ListTripExpenses(ctx, u.ID, trip.ID)
	if err != nil || len(exps) != 1 || exps[0].Amount != 42.5 {
		t.Fatalf("expense/trip_id not preserved: %+v (%v)", exps, err)
	}

	// hike_hikers + participant + mountain refs preserved.
	hikes, err := dst.ListHikes(ctx, u.ID, 10)
	if err != nil || len(hikes) != 1 {
		t.Fatalf("hike not migrated: %d (%v)", len(hikes), err)
	}
	if hikes[0].Mountain != "Rinjani" || !hikes[0].Camped || len(hikes[0].Participants) != 1 || hikes[0].Participants[0] != "Bob" {
		t.Fatalf("hike refs not preserved: %+v", hikes[0])
	}

	// user_skills skill_id mapped to the seeded Postgres skill id.
	us, err := dst.ListUserSkills(ctx, u.ID)
	if err != nil {
		t.Fatalf("list user skills: %v", err)
	}
	var enabled bool
	for _, s := range us {
		if s.Key == enabledSkillKey && s.Enabled {
			enabled = true
		}
	}
	if !enabled {
		t.Fatalf("enabled skill %q not preserved through key-mapping", enabledSkillKey)
	}

	// trace id preserved + score folded into the embedded sub-document.
	gotTrace, err := dst.GetTrace(ctx, traceID)
	if err != nil || gotTrace == nil {
		t.Fatalf("trace not migrated: %v", err)
	}
	if gotTrace.Score == nil || gotTrace.Score.Overall != 4.67 {
		t.Fatalf("trace score not folded: %+v", gotTrace.Score)
	}

	// Cross-backend rollup reflects both halves.
	act, err := dst.GetUserActivity(ctx, u.ID)
	if err != nil {
		t.Fatalf("user activity: %v", err)
	}
	if act.Runs != 1 || act.TotalTokens != 200 || act.Reminders != 1 || act.Notes != 1 {
		t.Fatalf("user activity rollup wrong: %+v", act)
	}
}
