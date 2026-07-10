package composio

import (
	"encoding/json"
	"testing"
)

func TestConnectionLabel(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "email under an email key",
			raw:  `{"id":"ca_1","status":"ACTIVE","toolkit":{"slug":"gmail"},"data":{"email":"alice@gmail.com"}}`,
			want: "alice@gmail.com",
		},
		{
			name: "emailAddress key (contains 'email')",
			raw:  `{"data":{"response_data":{"emailAddress":"bob@gmail.com"}}}`,
			want: "bob@gmail.com",
		},
		{
			name: "no email key, falls back to any email-looking value",
			raw:  `{"meta":{"profile":{"login":"carol@example.com"}}}`,
			want: "carol@example.com",
		},
		{
			name: "no email anywhere",
			raw:  `{"id":"ca_2","status":"ACTIVE","toolkit":{"slug":"github"}}`,
			want: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := connectionLabel(json.RawMessage(c.raw)); got != c.want {
				t.Errorf("connectionLabel = %q, want %q", got, c.want)
			}
		})
	}
}
