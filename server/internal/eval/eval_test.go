package eval

import (
	"strings"
	"testing"

	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

func TestParseVerdict(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantAcc int
		wantErr bool
	}{
		{"plain json", `{"accuracy":5,"helpfulness":4,"safety":5,"rationale":"good"}`, 5, false},
		{"code fenced", "```json\n{\"accuracy\":3,\"helpfulness\":3,\"safety\":4,\"rationale\":\"meh\"}\n```", 3, false},
		{"prose around", `Here is my verdict: {"accuracy":2,"helpfulness":1,"safety":5,"rationale":"wrong"} Hope that helps.`, 2, false},
		{"no json", `I cannot rate this.`, 0, true},
		{"all zero", `{"accuracy":0,"helpfulness":0,"safety":0,"rationale":"x"}`, 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, err := parseVerdict(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", v)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if v.Accuracy != c.wantAcc {
				t.Errorf("accuracy = %d, want %d", v.Accuracy, c.wantAcc)
			}
		})
	}
}

func TestJudgePromptsSelectsTranslatorRubric(t *testing.T) {
	tr := &store.Trace{
		Input:  "duh sakit banget kaki",
		Output: "Ouch, my foot really hurts",
		Skills: []string{"translator"},
	}
	sys, user := judgePrompts(tr)
	if sys != translatorJudgeSystemPrompt {
		t.Fatalf("translator run should use the translator system prompt")
	}
	if !strings.Contains(user, "Assistant's translation:") {
		t.Errorf("translator user prompt should label the output as a translation, got:\n%s", user)
	}
	if !strings.Contains(user, tr.Input) || !strings.Contains(user, tr.Output) {
		t.Errorf("user prompt missing input/output")
	}
}

func TestJudgePromptsDefaultsToGeneralRubric(t *testing.T) {
	for _, skills := range [][]string{nil, {"web_search"}, {"bucket_list", "reminder"}} {
		tr := &store.Trace{Input: "what's the weather", Output: "It's sunny", Skills: skills}
		sys, user := judgePrompts(tr)
		if sys != judgeSystemPrompt {
			t.Errorf("skills %v should use the general system prompt", skills)
		}
		if !strings.Contains(user, "Assistant reply:") {
			t.Errorf("skills %v should use the general reply label", skills)
		}
	}
}

func TestClamp15(t *testing.T) {
	for in, want := range map[int]int{-3: 1, 0: 1, 1: 1, 3: 3, 5: 5, 9: 5} {
		if got := clamp15(in); got != want {
			t.Errorf("clamp15(%d) = %d, want %d", in, got, want)
		}
	}
}
