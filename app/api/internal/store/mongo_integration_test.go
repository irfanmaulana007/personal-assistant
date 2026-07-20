//go:build integration

// MongoDB + HybridStore integration tests. Require Docker (real mongo:7 and, for
// the hybrid test, postgres:17 via testcontainers). Excluded from the default
// build; run with:
//
//	go test -tags integration ./internal/store/ -run 'Mongo|Hybrid'
package store

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/bson"
)

func newTestMongo(t *testing.T) *MongoStore {
	t.Helper()
	ctx := context.Background()

	mongoC, err := mongodb.Run(ctx, "mongo:7")
	if err != nil {
		t.Fatalf("start mongo container: %v", err)
	}
	t.Cleanup(func() { _ = mongoC.Terminate(ctx) })

	uri, err := mongoC.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	m, err := NewMongo(ctx, uri, "assistant_logs")
	if err != nil {
		t.Fatalf("NewMongo: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m
}

func TestMongoLogRoundTrips(t *testing.T) {
	m := newTestMongo(t)
	ctx := context.Background()

	// Message log: monotonic ids + oldest-first history.
	for i := 0; i < 3; i++ {
		if err := m.LogMessage(ctx, &MessageLog{UserID: 1, Platform: "web", Direction: "in", Body: "hi"}); err != nil {
			t.Fatalf("log message: %v", err)
		}
	}
	hist, err := m.GetMessageHistory(ctx, 1, "web", 10)
	if err != nil || len(hist) != 3 {
		t.Fatalf("history = %d (%v)", len(hist), err)
	}
	if hist[0].ID == 0 || hist[0].ID >= hist[2].ID {
		t.Fatalf("expected ascending monotonic ids, got %d..%d", hist[0].ID, hist[2].ID)
	}

	// Activities.
	if _, err := m.CreateActivity(ctx, 1, "run", "5k", time.Now().UTC(), ""); err != nil {
		t.Fatalf("create activity: %v", err)
	}
	acts, err := m.ListActivitiesSince(ctx, 1, time.Now().Add(-time.Hour).UTC())
	if err != nil || len(acts) != 1 || acts[0].Source != "chat" {
		t.Fatalf("activities wrong: %+v (%v)", acts, err)
	}

	// Trace + embedded score round-trip.
	id, err := m.CreateTrace(ctx, &Trace{UserID: 1, Platform: "web", Model: "m1", TotalTokens: 100, Status: "ok", Output: "hello"})
	if err != nil || id == 0 {
		t.Fatalf("create trace: %v (id=%d)", err, id)
	}
	if err := m.SaveTraceScore(ctx, &TraceScore{TraceID: id, Accuracy: 5, Helpfulness: 4, Safety: 5, Overall: 4.67, JudgeModel: "judge"}); err != nil {
		t.Fatalf("save score: %v", err)
	}
	got, err := m.GetTrace(ctx, id)
	if err != nil || got == nil || got.Score == nil || got.Score.Overall != 4.67 {
		t.Fatalf("get trace/score wrong: %+v (%v)", got, err)
	}

	// Unscored listing excludes the now-scored trace; a fresh one appears.
	id2, _ := m.CreateTrace(ctx, &Trace{UserID: 1, Platform: "web", Model: "m1", Status: "ok", Output: "yo"})
	uns, err := m.ListUnscoredTraces(ctx, time.Now().Add(-time.Hour).UTC(), 10)
	if err != nil {
		t.Fatalf("list unscored: %v", err)
	}
	if len(uns) != 1 || uns[0].ID != id2 {
		t.Fatalf("expected only the unscored trace %d, got %+v", id2, uns)
	}
}

func TestMongoAnalytics(t *testing.T) {
	m := newTestMongo(t)
	ctx := context.Background()

	// Known dataset: latencies 10..50 (+ one zero-latency row that must be
	// excluded from avg/percentiles), two users, one error, two models.
	seed := []struct {
		user            int64
		model           string
		latency, tokens int
		status          string
	}{
		{1, "opus", 10, 100, "ok"},
		{1, "opus", 20, 100, "ok"},
		{2, "opus", 30, 100, "ok"},
		{2, "sonnet", 40, 100, "ok"},
		{2, "sonnet", 50, 100, "error"},
		{1, "sonnet", 0, 100, "ok"}, // zero latency: excluded from latency stats
	}
	for _, s := range seed {
		if _, err := m.CreateTrace(ctx, &Trace{
			UserID: s.user, Platform: "web", Model: s.model,
			LatencyMs: s.latency, TotalTokens: s.tokens, Status: s.status, Output: "x",
		}); err != nil {
			t.Fatalf("seed trace: %v", err)
		}
		if err := m.LogToolUsage(ctx, s.user, "search", "web"); err != nil {
			t.Fatalf("log tool: %v", err)
		}
	}

	from := time.Now().Add(-24 * time.Hour).UTC()
	to := time.Now().Add(24 * time.Hour).UTC()
	st, err := m.UsageStatsBetween(ctx, from, to, nil)
	if err != nil {
		t.Fatalf("usage stats: %v", err)
	}

	if st.Summary.Requests != 6 {
		t.Fatalf("requests = %d, want 6", st.Summary.Requests)
	}
	if st.Errors != 1 {
		t.Fatalf("errors = %d, want 1", st.Errors)
	}
	if st.ActiveUsers != 2 {
		t.Fatalf("active users = %d, want 2", st.ActiveUsers)
	}
	// Percentiles over the non-zero latencies [10,20,30,40,50] (n=5), using the
	// SQLite pick() index math: p50 -> idx 2 (30), p95/p99 -> idx 4 (50).
	if st.LatencyP50 != 30 || st.LatencyP95 != 50 || st.LatencyP99 != 50 {
		t.Fatalf("percentiles = p50 %d p95 %d p99 %d, want 30/50/50", st.LatencyP50, st.LatencyP95, st.LatencyP99)
	}
	// Average excludes the zero-latency row: (10+20+30+40+50)/5 = 30.
	if st.AvgLatencyMs != 30 {
		t.Fatalf("avg latency = %d, want 30 (zero-latency row must be excluded)", st.AvgLatencyMs)
	}
	// Invariant: the hour/weekday histograms must account for every request. A
	// $dayOfWeek off-by-one (7 -> out of range) would silently drop a count.
	sum := func(xs []int) int {
		t := 0
		for _, x := range xs {
			t += x
		}
		return t
	}
	if got := sum(st.ByHour[:]); got != 6 {
		t.Fatalf("ByHour sums to %d, want 6", got)
	}
	if got := sum(st.ByWeekday[:]); got != 6 {
		t.Fatalf("ByWeekday sums to %d, want 6 (a mismatch signals the $dayOfWeek off-by-one)", got)
	}
	// Tool usage aggregation.
	if len(st.TopTools) != 1 || st.TopTools[0].Tool != "search" || st.TopTools[0].Count != 6 {
		t.Fatalf("top tools wrong: %+v", st.TopTools)
	}

	// Per-user/model breakdown.
	um, err := m.UsageByUserModel(ctx, from, to, nil)
	if err != nil {
		t.Fatalf("usage by user/model: %v", err)
	}
	total := 0
	for _, r := range um {
		total += r.Requests
	}
	if total != 6 {
		t.Fatalf("user/model requests total = %d, want 6", total)
	}
}

// TestHybridGetUserActivity verifies the one cross-backend method: it must merge
// trace totals (Mongo) with active-reminder and note counts (Postgres).
func TestHybridGetUserActivity(t *testing.T) {
	pg := newTestPostgres(t)
	mongo := newTestMongo(t)
	h := NewHybrid(pg, mongo)
	ctx := context.Background()

	u, err := pg.CreateUser(ctx, "h@example.com", "hash", "member")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Postgres side: 2 active reminders + 1 note.
	for i := 0; i < 2; i++ {
		if _, err := h.CreateReminder(ctx, u.ID, ReminderInput{Title: "r", RepeatMode: "daily", Times: []string{"09:00"}, Enabled: true}); err != nil {
			t.Fatalf("create reminder: %v", err)
		}
	}
	if _, err := h.CreateNote(ctx, u.ID, "n", "body", ""); err != nil {
		t.Fatalf("create note: %v", err)
	}

	// Mongo side: 3 traces totalling 600 tokens.
	for i := 0; i < 3; i++ {
		if _, err := h.CreateTrace(ctx, &Trace{UserID: u.ID, Platform: "web", Model: "m", TotalTokens: 200, Status: "ok"}); err != nil {
			t.Fatalf("create trace: %v", err)
		}
	}

	act, err := h.GetUserActivity(ctx, u.ID)
	if err != nil {
		t.Fatalf("get user activity: %v", err)
	}
	if act.Runs != 3 || act.TotalTokens != 600 {
		t.Fatalf("trace totals wrong: runs=%d tokens=%d, want 3/600", act.Runs, act.TotalTokens)
	}
	if act.Reminders != 2 {
		t.Fatalf("active reminders = %d, want 2", act.Reminders)
	}
	if act.Notes != 1 {
		t.Fatalf("notes = %d, want 1", act.Notes)
	}
}

// TestMongoBackfillProjectID verifies the project_id backfill migration: legacy
// documents (field absent, or the 0 sentinel) are stamped with defaultProjectID,
// documents already naming a real project are untouched, and re-running is a
// no-op.
func TestMongoBackfillProjectID(t *testing.T) {
	m := newTestMongo(t)
	ctx := context.Background()

	// Insert documents the way the pre-multi-project code did: message_log /
	// tool_usage / activities never carried project_id at all, and old traces had
	// it missing or set to the 0 sentinel. One trace already names project 7 and
	// must survive untouched.
	mustInsert := func(coll string, doc bson.M) {
		if _, err := m.col(coll).InsertOne(ctx, doc); err != nil {
			t.Fatalf("insert into %s: %v", coll, err)
		}
	}
	mustInsert(colMessageLog, bson.M{"id": int64(1), "user_id": int64(1), "platform": "web"})
	mustInsert(colToolUsage, bson.M{"user_id": int64(1), "tool": "search", "platform": "web"})
	mustInsert(colActivities, bson.M{"id": int64(1), "user_id": int64(1), "type": "run"})
	mustInsert(colTraces, bson.M{"id": int64(1), "user_id": int64(1)})                       // field absent
	mustInsert(colTraces, bson.M{"id": int64(2), "user_id": int64(1), "project_id": int64(0)}) // 0 sentinel
	mustInsert(colTraces, bson.M{"id": int64(3), "user_id": int64(1), "project_id": int64(7)}) // real project

	if err := m.backfillProjectID(ctx); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	projectOf := func(coll string, filter bson.M) int64 {
		var doc struct {
			ProjectID int64 `bson:"project_id"`
		}
		if err := m.col(coll).FindOne(ctx, filter).Decode(&doc); err != nil {
			t.Fatalf("read back %s %v: %v", coll, filter, err)
		}
		return doc.ProjectID
	}

	// Every legacy document is now attributed to the default project.
	if got := projectOf(colMessageLog, bson.M{"id": int64(1)}); got != defaultProjectID {
		t.Fatalf("message_log project_id = %d, want %d", got, defaultProjectID)
	}
	if got := projectOf(colToolUsage, bson.M{"tool": "search"}); got != defaultProjectID {
		t.Fatalf("tool_usage project_id = %d, want %d", got, defaultProjectID)
	}
	if got := projectOf(colActivities, bson.M{"id": int64(1)}); got != defaultProjectID {
		t.Fatalf("activities project_id = %d, want %d", got, defaultProjectID)
	}
	if got := projectOf(colTraces, bson.M{"id": int64(1)}); got != defaultProjectID {
		t.Fatalf("trace(missing) project_id = %d, want %d", got, defaultProjectID)
	}
	if got := projectOf(colTraces, bson.M{"id": int64(2)}); got != defaultProjectID {
		t.Fatalf("trace(0 sentinel) project_id = %d, want %d", got, defaultProjectID)
	}
	// The trace already naming a real project is left alone.
	if got := projectOf(colTraces, bson.M{"id": int64(3)}); got != 7 {
		t.Fatalf("trace(project 7) project_id = %d, want 7 (must not be overwritten)", got)
	}

	// Idempotent: a second run changes nothing.
	if err := m.backfillProjectID(ctx); err != nil {
		t.Fatalf("second backfill: %v", err)
	}
	if got := projectOf(colTraces, bson.M{"id": int64(3)}); got != 7 {
		t.Fatalf("after re-run, trace(project 7) project_id = %d, want 7", got)
	}
}

// TestMongoMigrationsRunOnce verifies the migration ledger: runMigrations records
// each applied step in schema_migrations and does not run it again.
func TestMongoMigrationsRunOnce(t *testing.T) {
	m := newTestMongo(t)
	ctx := context.Background()

	// newTestMongo already ran migrations once via NewMongo; every registered
	// migration must have a ledger entry.
	for _, mig := range mongoMigrationList {
		n, err := m.col(colMigrations).CountDocuments(ctx, bson.M{"_id": mig.name})
		if err != nil {
			t.Fatalf("count ledger for %s: %v", mig.name, err)
		}
		if n != 1 {
			t.Fatalf("ledger entries for %s = %d, want 1", mig.name, n)
		}
	}

	// Re-running is a no-op and must not duplicate ledger rows.
	if err := m.runMigrations(ctx); err != nil {
		t.Fatalf("re-run migrations: %v", err)
	}
	total, err := m.col(colMigrations).CountDocuments(ctx, bson.M{})
	if err != nil {
		t.Fatalf("count ledger: %v", err)
	}
	if total != int64(len(mongoMigrationList)) {
		t.Fatalf("ledger has %d rows, want %d", total, len(mongoMigrationList))
	}
}
