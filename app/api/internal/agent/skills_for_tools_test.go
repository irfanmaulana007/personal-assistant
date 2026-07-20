package agent

import (
	"reflect"
	"testing"
)

func TestSkillsForTools(t *testing.T) {
	tests := []struct {
		name  string
		tools []string
		want  []string
	}{
		{
			name:  "single skill tool maps to its owner",
			tools: []string{"bucketlist_add"},
			want:  []string{"bucket_list"},
		},
		{
			name:  "multiple tools from same skill collapse to one key",
			tools: []string{"bucketlist_add", "bucketlist_list"},
			want:  []string{"bucket_list"},
		},
		{
			name:  "distinct skills preserve first-seen order",
			tools: []string{"trip_create", "contact_add", "expense_add"},
			want:  []string{"travel_control", "ask_about_contact"},
		},
		{
			name:  "always-on base tools have no owning skill",
			tools: []string{"reminder_schedule", "schedule_event"},
			want:  nil,
		},
		{
			name:  "unknown tool names are ignored",
			tools: []string{"does_not_exist", "web_search"},
			want:  []string{"web_search"},
		},
		{
			name:  "no tools invoked yields no skills",
			tools: nil,
			want:  nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SkillsForTools(tc.tools)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("SkillsForTools(%v) = %v, want %v", tc.tools, got, tc.want)
			}
		})
	}
}

// Every tool registered under a skill must resolve back to that same skill key,
// so the run-detail "Skills" list can never misattribute an invoked tool.
func TestToolSkillOwnerCoversAllSkillTools(t *testing.T) {
	for key, specs := range skillTools {
		for _, s := range specs {
			if owner := toolSkillOwner[s.name]; owner != key {
				t.Errorf("tool %q owned by skill %q, got %q", s.name, key, owner)
			}
		}
	}
}
