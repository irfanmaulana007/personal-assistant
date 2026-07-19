//go:build integration

package store

import (
	"context"
	"testing"
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
