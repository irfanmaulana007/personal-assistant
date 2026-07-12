package eval

import "testing"

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

func TestClamp15(t *testing.T) {
	for in, want := range map[int]int{-3: 1, 0: 1, 1: 1, 3: 3, 5: 5, 9: 5} {
		if got := clamp15(in); got != want {
			t.Errorf("clamp15(%d) = %d, want %d", in, got, want)
		}
	}
}
