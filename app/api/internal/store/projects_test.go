//go:build integration

package store

import (
	"context"
	"testing"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/authctx"
)

func findFeatureID(t *testing.T, s *PostgresStore, key string) int64 {
	t.Helper()
	feats, err := s.ListFeatures(context.Background())
	if err != nil {
		t.Fatalf("list features: %v", err)
	}
	for _, f := range feats {
		if f.Key == key {
			return f.ID
		}
	}
	t.Fatalf("feature %q not seeded", key)
	return 0
}

func findSkillID(t *testing.T, s *PostgresStore, key string) int64 {
	t.Helper()
	skills, err := s.ListSkills(context.Background())
	if err != nil {
		t.Fatalf("list skills: %v", err)
	}
	for _, sk := range skills {
		if sk.Key == key {
			return sk.ID
		}
	}
	t.Fatalf("skill %q not seeded", key)
	return 0
}

func TestProjectsAndMembersRoundTrip(t *testing.T) {
	s := newTestPostgres(t)
	ctx := context.Background()

	owner, _ := s.CreateUser(ctx, "owner@example.com", "h", GlobalRoleSuperadmin)
	member, _ := s.CreateUser(ctx, "member@example.com", "h", GlobalRoleMember)

	p, err := s.CreateProject(ctx, "Acme", owner.ID)
	if err != nil || p.ID == 0 {
		t.Fatalf("create project: %v", err)
	}
	if err := s.AddProjectMember(ctx, p.ID, owner.ID, ProjectRoleAdmin); err != nil {
		t.Fatalf("add owner as admin: %v", err)
	}
	if err := s.AddProjectMember(ctx, p.ID, member.ID, ProjectRoleMember); err != nil {
		t.Fatalf("add member: %v", err)
	}

	// Roles resolve correctly.
	if role, _ := s.GetProjectRole(ctx, p.ID, owner.ID); role != ProjectRoleAdmin {
		t.Fatalf("owner role = %q, want admin", role)
	}
	if role, _ := s.GetProjectRole(ctx, p.ID, member.ID); role != ProjectRoleMember {
		t.Fatalf("member role = %q, want member", role)
	}
	// Non-member resolves to "".
	stranger, _ := s.CreateUser(ctx, "stranger@example.com", "h", GlobalRoleMember)
	if role, _ := s.GetProjectRole(ctx, p.ID, stranger.ID); role != "" {
		t.Fatalf("stranger role = %q, want empty", role)
	}

	members, err := s.ListProjectMembers(ctx, p.ID)
	if err != nil || len(members) != 2 {
		t.Fatalf("list members = %d (%v)", len(members), err)
	}
	if n, _ := s.CountProjectAdmins(ctx, p.ID); n != 1 {
		t.Fatalf("admin count = %d, want 1", n)
	}

	// ListProjectsForUser shows the member's project with their role + member count.
	summaries, err := s.ListProjectsForUser(ctx, member.ID)
	if err != nil || len(summaries) != 1 {
		t.Fatalf("projects for member = %d (%v)", len(summaries), err)
	}
	if summaries[0].Role != ProjectRoleMember || summaries[0].MemberCount != 2 {
		t.Fatalf("summary wrong: %+v", summaries[0])
	}

	// Promote member; admin count rises. Then remove; count of members drops.
	if err := s.UpdateProjectMemberRole(ctx, p.ID, member.ID, ProjectRoleAdmin); err != nil {
		t.Fatalf("promote: %v", err)
	}
	if n, _ := s.CountProjectAdmins(ctx, p.ID); n != 2 {
		t.Fatalf("admin count after promote = %d, want 2", n)
	}
	if err := s.RemoveProjectMember(ctx, p.ID, member.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if members, _ := s.ListProjectMembers(ctx, p.ID); len(members) != 1 {
		t.Fatalf("members after remove = %d, want 1", len(members))
	}

	// Rename + delete.
	if err := s.UpdateProjectName(ctx, p.ID, "Acme Renamed"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if got, _ := s.GetProject(ctx, p.ID); got == nil || got.Name != "Acme Renamed" {
		t.Fatalf("rename round-trip: %+v", got)
	}
	if err := s.DeleteProject(ctx, p.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if got, _ := s.GetProject(ctx, p.ID); got != nil {
		t.Fatalf("project should be deleted, got %+v", got)
	}
}

// TestCrossProjectDataIsolation verifies that domain rows written under one
// active project are invisible under another, while a project-less context
// (project id 0, e.g. the scheduler) still sees everything for the owner.
func TestCrossProjectDataIsolation(t *testing.T) {
	s := newTestPostgres(t)
	base := context.Background()
	u, _ := s.CreateUser(base, "iso@example.com", "h", GlobalRoleMember)
	pA, _ := s.CreateProject(base, "A", u.ID)
	pB, _ := s.CreateProject(base, "B", u.ID)
	ctxA := authctx.WithProjectID(base, pA.ID)
	ctxB := authctx.WithProjectID(base, pB.ID)

	// Bucket item written in project A.
	item, err := s.CreateBucketItem(ctxA, u.ID, "A-only", "", "", CategoryOther, nil)
	if err != nil {
		t.Fatalf("create bucket item: %v", err)
	}
	if got, _ := s.ListBucketItems(ctxA, u.ID); len(got) != 1 {
		t.Fatalf("project A should see its item, got %d", len(got))
	}
	if got, _ := s.ListBucketItems(ctxB, u.ID); len(got) != 0 {
		t.Fatalf("project B must NOT see project A's item, got %d", len(got))
	}
	if got, _ := s.GetBucketItem(ctxB, u.ID, item.ID); got != nil {
		t.Fatal("GetBucketItem from project B must return nil for A's item")
	}
	// A project-less context (scheduler/tests) sees all of the owner's rows.
	if got, _ := s.ListBucketItems(base, u.ID); len(got) != 1 {
		t.Fatalf("project-less context should see the item, got %d", len(got))
	}

	// Notes exhibit the same isolation.
	if _, err := s.CreateNote(ctxA, u.ID, "secret", "in A", ""); err != nil {
		t.Fatalf("create note: %v", err)
	}
	if got, _ := s.ListNotes(ctxB, u.ID, ""); len(got) != 0 {
		t.Fatalf("project B must not see project A's note, got %d", len(got))
	}
	if got, _ := s.ListNotes(ctxA, u.ID, ""); len(got) != 1 {
		t.Fatalf("project A should see its note, got %d", len(got))
	}

	// Reminders too: created in B, invisible in A.
	if _, err := s.CreateReminder(ctxB, u.ID, ReminderInput{Title: "B standup", RepeatMode: "daily", Times: []string{"09:00"}, Enabled: true}); err != nil {
		t.Fatalf("create reminder: %v", err)
	}
	if got, _ := s.ListReminders(ctxA, u.ID, false); len(got) != 0 {
		t.Fatalf("project A must not see project B's reminder, got %d", len(got))
	}
	if got, _ := s.ListReminders(ctxB, u.ID, false); len(got) != 1 {
		t.Fatalf("project B should see its reminder, got %d", len(got))
	}
}

func TestProjectSkillsAndFeatureCascade(t *testing.T) {
	s := newTestPostgres(t)
	ctx := context.Background()
	owner, _ := s.CreateUser(ctx, "o@example.com", "h", GlobalRoleSuperadmin)
	p, _ := s.CreateProject(ctx, "P", owner.ID)

	effective := func() map[string]bool {
		skills, err := s.ListProjectSkills(ctx, p.ID)
		if err != nil {
			t.Fatalf("list project skills: %v", err)
		}
		m := map[string]bool{}
		for _, sk := range skills {
			m[sk.Key] = sk.Enabled
		}
		return m
	}

	// bucket_list ships default-enabled and its feature is enabled by default.
	if !effective()["bucket_list"] {
		t.Fatal("bucket_list should be effective-enabled by default")
	}
	// web_search ships default-disabled.
	if effective()["web_search"] {
		t.Fatal("web_search should be effective-disabled by default")
	}

	// Per-project override enables web_search (its feature 'knowledge' is on).
	webID := findSkillID(t, s, "web_search")
	if err := s.SetProjectSkillEnabled(ctx, p.ID, webID, true); err != nil {
		t.Fatalf("enable web_search: %v", err)
	}
	if !effective()["web_search"] {
		t.Fatal("web_search should be enabled after per-project override")
	}

	// Disabling the bucket_list FEATURE cascades: its skill goes effective-off
	// even though the skill's own default/override is on.
	blFeature := findFeatureID(t, s, "bucket_list")
	if err := s.SetProjectFeatureEnabled(ctx, p.ID, blFeature, false); err != nil {
		t.Fatalf("disable bucket_list feature: %v", err)
	}
	if effective()["bucket_list"] {
		t.Fatal("bucket_list skill must be off when its feature is disabled (cascade)")
	}
	keys, _ := s.EnabledProjectSkillKeys(ctx, p.ID)
	for _, k := range keys {
		if k == "bucket_list" {
			t.Fatal("EnabledProjectSkillKeys must exclude a feature-gated skill")
		}
	}

	// Re-enabling the feature restores the skill.
	if err := s.SetProjectFeatureEnabled(ctx, p.ID, blFeature, true); err != nil {
		t.Fatalf("re-enable feature: %v", err)
	}
	if !effective()["bucket_list"] {
		t.Fatal("bucket_list should be back on after re-enabling its feature")
	}

	// ListProjectFeatures reports the feature with its attached skill keys.
	feats, err := s.ListProjectFeatures(ctx, p.ID)
	if err != nil {
		t.Fatalf("list project features: %v", err)
	}
	var sawBucket bool
	for _, f := range feats {
		if f.Key == "bucket_list" {
			sawBucket = true
			if len(f.SkillKeys) != 1 || f.SkillKeys[0] != "bucket_list" {
				t.Fatalf("bucket_list feature skill keys = %v, want [bucket_list]", f.SkillKeys)
			}
			if !f.Enabled {
				t.Fatal("bucket_list feature should be enabled")
			}
		}
	}
	if !sawBucket {
		t.Fatal("bucket_list feature missing from ListProjectFeatures")
	}
}

// TestProjectSkillForks covers project-owned skills: a fork shadows the global
// skill of the same key for that project (with its own prompt), inherits the
// global twin's feature gate, drives EnabledProjectSkillKeys, is isolated to its
// project, and deleting it reverts the project to the global skill.
func TestProjectSkillForks(t *testing.T) {
	s := newTestPostgres(t)
	ctx := context.Background()
	owner, _ := s.CreateUser(ctx, "o@example.com", "h", GlobalRoleSuperadmin)
	p, _ := s.CreateProject(ctx, "P", owner.ID)
	other, _ := s.CreateProject(ctx, "Other", owner.ID)

	// A helper returning the row for a key in a project's effective listing.
	row := func(pid int64, key string) *UserSkill {
		list, err := s.ListProjectSkills(ctx, pid)
		if err != nil {
			t.Fatalf("list project skills: %v", err)
		}
		for i := range list {
			if list[i].Key == key {
				return &list[i]
			}
		}
		return nil
	}

	// web_search is global, default-disabled. Enable it for p, then fork it.
	webID := findSkillID(t, s, "web_search")
	if err := s.SetProjectSkillEnabled(ctx, p.ID, webID, true); err != nil {
		t.Fatalf("enable web_search: %v", err)
	}
	base := Skill{Key: "web_search", Name: "Web Search (P)", Description: "d", Prompt: "PROJECT PROMPT", Category: "Knowledge", DefaultEnabled: true, SortOrder: 8}
	fork, err := s.CreateProjectSkill(ctx, p.ID, base, "o@example.com")
	if err != nil {
		t.Fatalf("create fork: %v", err)
	}
	if !fork.IsProjectOwned() || fork.ProjectID == nil || *fork.ProjectID != p.ID {
		t.Fatalf("fork not owned by project: %+v", fork)
	}
	// Carry the enabled state over, as the API layer does.
	if err := s.SetProjectSkillEnabled(ctx, p.ID, fork.ID, true); err != nil {
		t.Fatalf("enable fork: %v", err)
	}

	// In p's listing the fork SHADOWS the global row: exactly one web_search, the
	// project-owned one, with the project prompt.
	var count int
	for _, u := range mustList(t, s, p.ID) {
		if u.Key == "web_search" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one web_search row for p, got %d", count)
	}
	r := row(p.ID, "web_search")
	if r == nil || !r.IsProjectOwned() || r.Prompt != "PROJECT PROMPT" {
		t.Fatalf("p's web_search should be the fork with the project prompt, got %+v", r)
	}
	if !r.Enabled {
		t.Fatal("forked web_search should be enabled for p")
	}

	// The fork inherits the global twin's feature ('knowledge'): disabling that
	// feature cascades the fork off.
	knowledge := findFeatureID(t, s, "knowledge")
	if err := s.SetProjectFeatureEnabled(ctx, p.ID, knowledge, false); err != nil {
		t.Fatalf("disable knowledge feature: %v", err)
	}
	if row(p.ID, "web_search").Enabled {
		t.Fatal("forked web_search must go off when its inherited feature is disabled")
	}
	if err := s.SetProjectFeatureEnabled(ctx, p.ID, knowledge, true); err != nil {
		t.Fatalf("re-enable knowledge feature: %v", err)
	}

	// EnabledProjectSkillKeys reflects the fork (the key is the shared capability).
	keys, _ := s.EnabledProjectSkillKeys(ctx, p.ID)
	if !contains(keys, "web_search") {
		t.Fatalf("EnabledProjectSkillKeys should include web_search via the fork, got %v", keys)
	}

	// Isolation: the other project still sees only the global web_search.
	if o := row(other.ID, "web_search"); o == nil || o.IsProjectOwned() {
		t.Fatalf("other project must see the global web_search, got %+v", o)
	}

	// Deleting the fork reverts p to the global skill.
	if err := s.DeleteProjectSkill(ctx, p.ID, fork.ID); err != nil {
		t.Fatalf("delete fork: %v", err)
	}
	after := row(p.ID, "web_search")
	if after == nil || after.IsProjectOwned() {
		t.Fatalf("after delete, p should see the global web_search, got %+v", after)
	}
	// DeleteProjectSkill must refuse to touch a global skill.
	if err := s.DeleteProjectSkill(ctx, p.ID, webID); err == nil {
		t.Fatal("DeleteProjectSkill must not delete a global skill")
	}
	// The global catalog is intact.
	if findSkillID(t, s, "web_search") == 0 {
		t.Fatal("global web_search must still exist")
	}
}

func mustList(t *testing.T, s *PostgresStore, pid int64) []UserSkill {
	t.Helper()
	list, err := s.ListProjectSkills(context.Background(), pid)
	if err != nil {
		t.Fatalf("list project skills: %v", err)
	}
	return list
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
