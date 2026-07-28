package groupproject

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/store"
)

// fakeStore is an in-memory projectStore for tests.
type fakeStore struct {
	projects []store.ProjectSummary           // owner's projects
	mappings map[string]store.WhatsAppMapping // jid → mapping
	nextID   int64
}

func newFakeStore(projects ...store.Project) *fakeStore {
	f := &fakeStore{mappings: map[string]store.WhatsAppMapping{}, nextID: 100}
	for _, p := range projects {
		f.projects = append(f.projects, store.ProjectSummary{Project: p, Role: store.ProjectRoleAdmin})
	}
	return f
}

func (f *fakeStore) GetWhatsAppMapping(_ context.Context, jid string) (*store.WhatsAppMapping, error) {
	if m, ok := f.mappings[jid]; ok {
		return &m, nil
	}
	return nil, nil
}

func (f *fakeStore) CreateWhatsAppMapping(_ context.Context, m store.WhatsAppMapping) (*store.WhatsAppMapping, error) {
	if existing, ok := f.mappings[m.JID]; ok {
		m.ID = existing.ID
	} else {
		f.nextID++
		m.ID = f.nextID
	}
	f.mappings[m.JID] = m
	return &m, nil
}

func (f *fakeStore) DeleteWhatsAppMapping(_ context.Context, id int64) error {
	for jid, m := range f.mappings {
		if m.ID == id {
			delete(f.mappings, jid)
		}
	}
	return nil
}

func (f *fakeStore) ListProjectsForUser(_ context.Context, _ int64) ([]store.ProjectSummary, error) {
	return f.projects, nil
}

func (f *fakeStore) GetProject(_ context.Context, id int64) (*store.Project, error) {
	for _, p := range f.projects {
		if p.ID == id {
			pr := p.Project
			return &pr, nil
		}
	}
	return nil, nil
}

func testService(f *fakeStore) *Service {
	return New(f, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// --- classify ---

func TestClassify(t *testing.T) {
	cases := []struct {
		name   string
		text   string
		kind   cmdKind
		target string
	}{
		{"status question ID", "@45943953072271 di sini sebagai assistant di project apa?", cmdStatus, ""},
		{"status question EN", "which project are you in?", cmdStatus, ""},
		{"status bare project apa", "project apa?", cmdStatus, ""},
		{"assign EN explicit", "assign to project Beta", cmdAssign, "Beta"},
		{"assign EN act as", "act as project Personal", cmdAssign, "Personal"},
		{"assign ID pindah", "pindah ke project Beta", cmdAssign, "Beta"},
		{"assign ID jadikan", "jadikan project Personal", cmdAssign, "Personal"},
		{"assign bare", "project Beta", cmdAssignBare, "Beta"},
		{"assign this group", "assign this group to project Work Stuff", cmdAssign, "Work Stuff"},
		{"list EN", "list projects", cmdList, ""},
		{"list ID daftar", "daftar project", cmdList, ""},
		{"list ID apa aja", "project apa aja yang ada?", cmdList, ""},
		{"unassign EN", "unassign from this project", cmdUnassign, ""},
		{"unassign ID", "lepas project dari grup ini", cmdUnassign, ""},
		{"ordinary reminder mentioning project", "ingatkan aku meeting project Apollo jam 3", cmdStatus, ""},
		{"ordinary chat no project", "halo apa kabar?", cmdNone, ""},
		{"ordinary set reminder", "set a reminder for tomorrow 9am", cmdNone, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classify(tc.text)
			if got.kind != tc.kind {
				t.Fatalf("kind = %v, want %v (text=%q)", got.kind, tc.kind, tc.text)
			}
			if tc.target != "" && !strings.EqualFold(got.target, tc.target) {
				t.Fatalf("target = %q, want %q", got.target, tc.target)
			}
		})
	}
}

// "ingatkan aku meeting project Apollo jam 3" must NOT hijack a real reminder in
// a bound group: classify yields cmdStatus, and a bound group routes status to
// the agent, so Handle returns handled=false.
func TestBoundGroupOrdinaryMessageReachesAgent(t *testing.T) {
	f := newFakeStore(store.Project{ID: 1, Name: "Personal", Slug: "personal"})
	f.mappings["g@g.us"] = store.WhatsAppMapping{ID: 1, JID: "g@g.us", Kind: "group", ProjectID: 1}
	s := testService(f)

	for _, text := range []string{
		"ingatkan aku meeting project Apollo jam 3",
		"which project are you in?",
		"halo, tolong buatkan reminder besok",
	} {
		if reply, handled := s.Handle(context.Background(), "g@g.us", text, 1, true, true); handled {
			t.Fatalf("bound-group message %q was intercepted (reply=%q); want it to reach the agent", text, reply)
		}
	}
}

// An unbound group intercepts every addressed message (the agent must not run
// unscoped) and prompts the owner, listing projects.
func TestUnboundGroupPromptsOwner(t *testing.T) {
	f := newFakeStore(store.Project{ID: 1, Name: "Personal", Slug: "personal"}, store.Project{ID: 2, Name: "Beta"})
	s := testService(f)

	reply, handled := s.Handle(context.Background(), "g@g.us", "hi there", 1, false, true)
	if !handled {
		t.Fatal("unbound group must handle every message")
	}
	if !strings.Contains(reply, "not assigned") {
		t.Fatalf("owner prompt should say it is unassigned, got: %q", reply)
	}
	if !strings.Contains(reply, "Personal") || !strings.Contains(reply, "Beta") {
		t.Fatalf("owner prompt should list projects, got: %q", reply)
	}
}

// A non-owner in an unbound group is told to ask the owner and never sees the
// project list.
func TestUnboundGroupNonOwnerNoLeak(t *testing.T) {
	f := newFakeStore(store.Project{ID: 1, Name: "SecretProj"})
	s := testService(f)

	reply, handled := s.Handle(context.Background(), "g@g.us", "project apa?", 1, false, false)
	if !handled {
		t.Fatal("unbound group must handle every message")
	}
	if strings.Contains(reply, "SecretProj") {
		t.Fatalf("non-owner must not see project names, got: %q", reply)
	}
	if !strings.Contains(strings.ToLower(reply), "owner") {
		t.Fatalf("non-owner should be pointed to the owner, got: %q", reply)
	}
}

// The owner assigning a project in an unbound group creates the mapping and
// confirms.
func TestOwnerAssignCreatesMapping(t *testing.T) {
	f := newFakeStore(store.Project{ID: 1, Name: "Personal"}, store.Project{ID: 2, Name: "Beta", Slug: "beta"})
	s := testService(f)

	reply, handled := s.Handle(context.Background(), "g@g.us", "assign to project Beta", 1, false, true)
	if !handled {
		t.Fatal("assign must be handled")
	}
	if !strings.Contains(reply, "Beta") {
		t.Fatalf("confirm should name the project, got: %q", reply)
	}
	m, ok := f.mappings["g@g.us"]
	if !ok {
		t.Fatal("mapping was not created")
	}
	if m.ProjectID != 2 || m.Kind != "group" || m.Role != store.ProjectRoleAdmin {
		t.Fatalf("mapping = %+v, want project 2 / group / admin", m)
	}
}

// A non-owner cannot assign; nothing is written.
func TestNonOwnerCannotAssign(t *testing.T) {
	f := newFakeStore(store.Project{ID: 2, Name: "Beta"})
	s := testService(f)

	reply, handled := s.Handle(context.Background(), "g@g.us", "assign to project Beta", 1, false, false)
	if !handled {
		t.Fatal("unbound group must handle the message")
	}
	if _, ok := f.mappings["g@g.us"]; ok {
		t.Fatal("non-owner assign must not create a mapping")
	}
	if !strings.Contains(strings.ToLower(reply), "owner") {
		t.Fatalf("reply should explain only the owner can assign, got: %q", reply)
	}
}

// The owner switching the project in a bound group updates the mapping.
func TestOwnerSwitchInBoundGroup(t *testing.T) {
	f := newFakeStore(store.Project{ID: 1, Name: "Personal"}, store.Project{ID: 2, Name: "Beta"})
	f.mappings["g@g.us"] = store.WhatsAppMapping{ID: 1, JID: "g@g.us", Kind: "group", ProjectID: 1, Role: store.ProjectRoleAdmin}
	s := testService(f)

	// Bare "project Beta" from the owner switches.
	reply, handled := s.Handle(context.Background(), "g@g.us", "project Beta", 1, true, true)
	if !handled {
		t.Fatal("owner switch must be handled")
	}
	if !strings.Contains(reply, "Beta") {
		t.Fatalf("confirm should name the new project, got: %q", reply)
	}
	if f.mappings["g@g.us"].ProjectID != 2 {
		t.Fatalf("mapping not switched: %+v", f.mappings["g@g.us"])
	}
}

// A bare "project <text>" that names nothing real in a bound group is treated as
// ordinary chat (reaches the agent), not an assignment error.
func TestBareProjectUnresolvedBoundReachesAgent(t *testing.T) {
	f := newFakeStore(store.Project{ID: 1, Name: "Personal"})
	f.mappings["g@g.us"] = store.WhatsAppMapping{ID: 1, JID: "g@g.us", Kind: "group", ProjectID: 1}
	s := testService(f)

	if _, handled := s.Handle(context.Background(), "g@g.us", "project status update is due", 1, true, true); handled {
		t.Fatal("bare 'project ...' naming no real project in a bound group should reach the agent")
	}
}

// The owner unassigning a bound group deletes the mapping.
func TestOwnerUnassign(t *testing.T) {
	f := newFakeStore(store.Project{ID: 1, Name: "Personal"})
	f.mappings["g@g.us"] = store.WhatsAppMapping{ID: 1, JID: "g@g.us", Kind: "group", ProjectID: 1}
	s := testService(f)

	reply, handled := s.Handle(context.Background(), "g@g.us", "unassign this project", 1, true, true)
	if !handled {
		t.Fatal("unassign must be handled")
	}
	if !strings.Contains(strings.ToLower(reply), "detach") {
		t.Fatalf("reply should confirm detach, got: %q", reply)
	}
	if _, ok := f.mappings["g@g.us"]; ok {
		t.Fatal("mapping should have been deleted")
	}
}

// resolveProject matches by name, slug, "default" alias, and substring; and
// returns nil for unknown or ambiguous input.
func TestResolveProject(t *testing.T) {
	f := newFakeStore(
		store.Project{ID: 1, Name: "Personal", Slug: "personal"},
		store.Project{ID: 2, Name: "Beta Work", Slug: "beta-work"},
		store.Project{ID: 3, Name: "Betamax", Slug: "betamax"},
	)
	s := testService(f)
	ctx := context.Background()

	if p := s.resolveProject(ctx, 1, "Personal"); p == nil || p.ID != 1 {
		t.Fatal("exact name match failed")
	}
	if p := s.resolveProject(ctx, 1, "beta-work"); p == nil || p.ID != 2 {
		t.Fatal("exact slug match failed")
	}
	if p := s.resolveProject(ctx, 1, "default"); p == nil || p.ID != 1 {
		t.Fatal("'default' alias should map to project id 1")
	}
	if p := s.resolveProject(ctx, 1, "personal stuff"); p != nil {
		t.Fatalf("unknown project should not match, got %+v", p)
	}
	// "beta" is a substring of both "Beta Work" and "Betamax" → ambiguous → nil.
	if p := s.resolveProject(ctx, 1, "beta"); p != nil {
		t.Fatalf("ambiguous substring should return nil, got %+v", p)
	}
}
