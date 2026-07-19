package agent

import (
	"reflect"
	"testing"

	"github.com/irfanmaulana007/personal-assistant/server/internal/llm"
	"github.com/irfanmaulana007/personal-assistant/server/internal/media"
)

// buildResult must label a run with only the skills it actually exercised
// (derived from the tools it invoked), never every enabled skill. Regression
// for run #187: a pure Trello turn scored 2.33/5 because "translator" was in
// the run's Skills — it rode along as an enabled-but-unused skill — so the
// skill-aware judge graded the reply as a (missing) translation.
func TestBuildResultSkillsFromExercisedToolsOnly(t *testing.T) {
	a := &Agent{}
	collector := &media.Collector{}

	tests := []struct {
		name string
		used []ToolInvocation
		want []string
	}{
		{
			name: "trello turn reports only trello_card, not translator",
			used: []ToolInvocation{
				{Name: "TRELLO_GET_MEMBERS_BOARDS_BY_ID_MEMBER"}, // provider/MCP tool, no owning skill
				{Name: "trello_update_card"},
				{Name: "trello_create_task"},
			},
			want: []string{"trello_card"},
		},
		{
			name: "no tools invoked yields no skills",
			used: nil,
			want: nil,
		},
		{
			name: "distinct skills preserve first-seen order",
			used: []ToolInvocation{
				{Name: "bucketlist_add"},
				{Name: "web_search"},
			},
			want: []string{"bucket_list", "web_search"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := a.buildResult("ok", llm.Usage{}, "deepseek-v4-flash", tc.used, nil, collector)
			if !reflect.DeepEqual(r.Skills, tc.want) {
				t.Errorf("buildResult Skills = %v, want %v", r.Skills, tc.want)
			}
			for _, s := range r.Skills {
				if s == "translator" {
					t.Errorf("run must never be labelled with translator via the agent path; got %v", r.Skills)
				}
			}
		})
	}
}
