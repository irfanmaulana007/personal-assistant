package routine

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/agent"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/settings"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/store"
)

var testTZ = time.FixedZone("WIB", 7*3600) // UTC+7, no DST

// fakeStore backs settings with an in-memory map and a fixed admin owner; any
// other Store call panics (nil embedded interface), surfacing unexpected usage.
type fakeStore struct {
	store.Store
	kv       map[string][]byte
	admin    *store.User
	traces   []store.Trace
	projects []store.ProjectSummary
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		kv:    map[string][]byte{},
		admin: &store.User{ID: 1},
		// One default project so a routine run is scoped to it (mirrors the owner's
		// unmapped 1:1 chat) instead of the unscoped project 0.
		projects: []store.ProjectSummary{{Project: store.Project{ID: 1}, Role: store.GlobalRoleSuperadmin}},
	}
}

func (f *fakeStore) ListProjectsForUser(_ context.Context, _ int64) ([]store.ProjectSummary, error) {
	return f.projects, nil
}

func (f *fakeStore) GetSetting(_ context.Context, key string) ([]byte, error) { return f.kv[key], nil }
func (f *fakeStore) SetSetting(_ context.Context, key string, val []byte) error {
	f.kv[key] = val
	return nil
}
func (f *fakeStore) FirstAdmin(_ context.Context) (*store.User, error) { return f.admin, nil }
func (f *fakeStore) CreateTrace(_ context.Context, t *store.Trace) (int64, error) {
	f.traces = append(f.traces, *t)
	return int64(len(f.traces)), nil
}

// fakeAgent records its calls and returns a canned reply/error.
type fakeAgent struct {
	reply string
	err   error
	calls int
}

func (a *fakeAgent) Run(_ context.Context, _ string, _ []agent.Message, _ string) (*agent.Result, error) {
	a.calls++
	if a.err != nil {
		return nil, a.err
	}
	return &agent.Result{Reply: a.reply}, nil
}

func newSvc(t *testing.T, ag AgentRunner) (*Service, *fakeStore, *[]string) {
	t.Helper()
	fs := newFakeStore()
	set := settings.New(fs, nil)
	svc := New(set, fs, ag, testTZ, "owner@s.whatsapp.net", slog.New(slog.NewTextHandler(io.Discard, nil)))
	var sent []string
	svc.SetSendFunc(func(_ context.Context, _, text string) error {
		sent = append(sent, text)
		return nil
	})
	return svc, fs, &sent
}

func at(hh, mm int) time.Time { return time.Date(2026, time.March, 10, hh, mm, 0, 0, testTZ) }

func TestList_Defaults(t *testing.T) {
	svc, _, _ := newSvc(t, &fakeAgent{})
	views := svc.List(context.Background())
	if len(views) != 3 {
		t.Fatalf("expected 3 routines, got %d", len(views))
	}
	sod := views[0]
	if sod.Key != "start_of_day" || sod.Enabled || sod.Time != "07:00" {
		t.Errorf("start_of_day defaults wrong: %+v", sod)
	}
	if sod.Prompt != sod.DefaultPrompt || sod.Prompt == "" {
		t.Errorf("effective prompt should equal the built-in default")
	}
	if views[1].Key != "end_of_day" || views[1].Time != "21:00" {
		t.Errorf("end_of_day defaults wrong: %+v", views[1])
	}
	if views[2].Key != "nightly_triage" || views[2].Enabled || views[2].Time != "23:00" {
		t.Errorf("nightly_triage defaults wrong: %+v", views[2])
	}
}

func TestUpdate_PersistsAndValidates(t *testing.T) {
	svc, _, _ := newSvc(t, &fakeAgent{})
	ctx := context.Background()

	on := true
	tm := "6:30" // should normalize to 06:30
	pr := "do the thing"
	v, err := svc.Update(ctx, "start_of_day", Update{Enabled: &on, Time: &tm, Prompt: &pr})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !v.Enabled || v.Time != "06:30" || v.Prompt != "do the thing" {
		t.Errorf("update not applied/normalized: %+v", v)
	}
	// Reloading reflects the persisted values.
	if got := svc.List(ctx)[0]; !got.Enabled || got.Time != "06:30" {
		t.Errorf("persisted values not reflected: %+v", got)
	}
	// An empty prompt clears the override, reverting to the default.
	empty := ""
	v, _ = svc.Update(ctx, "start_of_day", Update{Prompt: &empty})
	if v.Prompt != v.DefaultPrompt {
		t.Errorf("empty prompt should revert to default")
	}
	// A malformed time is rejected.
	bad := "25:61"
	if _, err := svc.Update(ctx, "start_of_day", Update{Time: &bad}); err == nil {
		t.Error("expected error for malformed time")
	}
	// An unknown routine is rejected.
	if _, err := svc.Update(ctx, "nope", Update{Enabled: &on}); err == nil {
		t.Error("expected error for unknown routine")
	}
}

func TestRunNow_SendsReply(t *testing.T) {
	ag := &fakeAgent{reply: "Good morning ☀️"}
	svc, fs, sent := newSvc(t, ag)
	ok, msg, err := svc.RunNow(context.Background(), "start_of_day")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !ok || msg != "Good morning ☀️" {
		t.Errorf("expected sent reply, got ok=%v msg=%q", ok, msg)
	}
	if len(*sent) != 1 || (*sent)[0] != "Good morning ☀️" {
		t.Errorf("send func should receive the reply, got %v", *sent)
	}
	// The run is logged as a trace attributed to the routine, so it shows up on
	// the Logs page tagged with its source.
	if len(fs.traces) != 1 {
		t.Fatalf("expected one trace recorded, got %d", len(fs.traces))
	}
	tr := fs.traces[0]
	if tr.Source != "start_of_day" || tr.Platform != "whatsapp" || tr.Output != "Good morning ☀️" {
		t.Errorf("trace not tagged correctly: %+v", tr)
	}
}

func TestRunNow_RecordsTraceOnAgentError(t *testing.T) {
	ag := &fakeAgent{err: errors.New("llm down")}
	svc, fs, _ := newSvc(t, ag)
	if _, _, err := svc.RunNow(context.Background(), "end_of_day"); err == nil {
		t.Fatal("expected an error when the agent fails")
	}
	// A failed run is still logged (as an error trace) so it is visible on the
	// Logs page rather than silently vanishing.
	if len(fs.traces) != 1 {
		t.Fatalf("expected one trace recorded, got %d", len(fs.traces))
	}
	tr := fs.traces[0]
	if tr.Source != "end_of_day" || tr.Status != "error" || tr.Error == "" {
		t.Errorf("error trace not recorded correctly: %+v", tr)
	}
}

func TestRunNow_SentinelSendsNothing(t *testing.T) {
	ag := &fakeAgent{reply: Sentinel}
	svc, _, sent := newSvc(t, ag)
	ok, _, err := svc.RunNow(context.Background(), "end_of_day")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if ok {
		t.Error("sentinel reply should not be sent")
	}
	if len(*sent) != 0 {
		t.Errorf("nothing should be sent, got %v", *sent)
	}
}

func TestMaybeRun_FiresOnceThenDedupes(t *testing.T) {
	ag := &fakeAgent{reply: "hi"}
	svc, _, sent := newSvc(t, ag)
	ctx := context.Background()
	on := true
	tm := "07:00"
	svc.Update(ctx, "start_of_day", Update{Enabled: &on, Time: &tm})
	d := Catalog[0] // start_of_day

	svc.maybeRun(ctx, d, at(7, 5)) // 5 min after slot, within grace
	svc.maybeRun(ctx, d, at(7, 6)) // same day → deduped
	if ag.calls != 1 {
		t.Errorf("expected exactly one agent run, got %d", ag.calls)
	}
	if len(*sent) != 1 {
		t.Errorf("expected exactly one send, got %d", len(*sent))
	}
}

func TestMaybeRun_NotBeforeSlot(t *testing.T) {
	ag := &fakeAgent{reply: "hi"}
	svc, _, _ := newSvc(t, ag)
	ctx := context.Background()
	on := true
	tm := "07:00"
	svc.Update(ctx, "start_of_day", Update{Enabled: &on, Time: &tm})
	svc.maybeRun(ctx, Catalog[0], at(6, 30)) // before the slot
	if ag.calls != 0 {
		t.Errorf("should not run before the slot, got %d calls", ag.calls)
	}
}

func TestMaybeRun_PastGraceClaimsButSkipsSend(t *testing.T) {
	ag := &fakeAgent{reply: "hi"}
	svc, fs, sent := newSvc(t, ag)
	ctx := context.Background()
	on := true
	tm := "07:00"
	svc.Update(ctx, "start_of_day", Update{Enabled: &on, Time: &tm})
	svc.maybeRun(ctx, Catalog[0], at(9, 0)) // 2h past slot, beyond grace
	if ag.calls != 0 || len(*sent) != 0 {
		t.Errorf("beyond grace should not run/send; calls=%d sent=%d", ag.calls, len(*sent))
	}
	// But the day is still claimed so it never fires late.
	if string(fs.kv["routine_start_of_day_last_run"]) != "2026-03-10" {
		t.Errorf("expected last_run claimed, got %q", fs.kv["routine_start_of_day_last_run"])
	}
}

func TestMaybeRun_DisabledDoesNothing(t *testing.T) {
	ag := &fakeAgent{reply: "hi"}
	svc, _, _ := newSvc(t, ag)
	// start_of_day is disabled by default.
	svc.maybeRun(context.Background(), Catalog[0], at(7, 5))
	if ag.calls != 0 {
		t.Errorf("disabled routine must not run, got %d calls", ag.calls)
	}
}

func TestMigrateFromDigest(t *testing.T) {
	svc, _, _ := newSvc(t, &fakeAgent{})
	ctx := context.Background()

	// Seed a legacy digest time, then migrate.
	if err := svc.settings.SetReminderDigestTime(ctx, "08:15"); err != nil {
		t.Fatal(err)
	}
	svc.MigrateFromDigest(ctx)

	v := svc.List(ctx)[0]
	if v.Time != "08:15" || !v.Enabled {
		t.Errorf("migration should carry time over and enable: %+v", v)
	}
	if svc.settings.ReminderDigestTime(ctx) != "" {
		t.Error("legacy digest time should be cleared after migration")
	}
	// Idempotent: a second migrate (routine already configured) is a no-op even
	// if a stray digest reappears.
	_ = svc.settings.SetReminderDigestTime(ctx, "05:00")
	svc.MigrateFromDigest(ctx)
	if got := svc.List(ctx)[0].Time; got != "08:15" {
		t.Errorf("second migrate should be a no-op, got %q", got)
	}
}
