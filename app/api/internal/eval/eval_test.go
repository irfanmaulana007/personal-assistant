package eval

import (
	"strings"
	"testing"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/store"
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

func TestRenderJudgePromptShowsToolResultsAndFlagsCreates(t *testing.T) {
	tr := &store.Trace{
		Input:  "add milk to my wishlist",
		Output: "Done, I've added milk to your wishlist.",
		Tools: []store.ToolInvocation{
			{Name: "wishlist_add", Arguments: `{"item":"milk"}`, Result: "Added 'milk' to your wishlist (#12)."},
		},
	}
	user := renderJudgePrompt(tr, "User message:", "Assistant reply:")
	if !strings.Contains(user, "creates data") {
		t.Errorf("create tool should be flagged, got:\n%s", user)
	}
	if !strings.Contains(user, "result: Added 'milk' to your wishlist (#12).") {
		t.Errorf("tool result should be shown to the judge, got:\n%s", user)
	}
	if !strings.Contains(user, `args={"item":"milk"}`) {
		t.Errorf("tool arguments should be shown, got:\n%s", user)
	}
}

func TestRenderJudgePromptStatesWhenNoToolRan(t *testing.T) {
	tr := &store.Trace{Input: "remind me to call mom", Output: "Sure, I've set that reminder."}
	user := renderJudgePrompt(tr, "User message:", "Assistant reply:")
	if !strings.Contains(user, "Tools the assistant called: none") {
		t.Errorf("absence of tools should be stated so fabricated actions are detectable, got:\n%s", user)
	}
}

func TestRenderJudgePromptMarksMissingResult(t *testing.T) {
	tr := &store.Trace{
		Input:  "add milk to my wishlist",
		Output: "Added.",
		Tools:  []store.ToolInvocation{{Name: "wishlist_add", Arguments: `{"item":"milk"}`, Result: "  "}},
	}
	user := renderJudgePrompt(tr, "User message:", "Assistant reply:")
	if !strings.Contains(user, "result: (no result returned)") {
		t.Errorf("blank tool result should be surfaced explicitly, got:\n%s", user)
	}
}

func TestJudgeSystemPromptCoversToolExpectationAndVerification(t *testing.T) {
	for _, want := range []string{"FABRICATED action", "read-back confirmation", "was expected to CALL"} {
		if !strings.Contains(judgeSystemPrompt, want) {
			t.Errorf("judge system prompt should cover %q", want)
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
