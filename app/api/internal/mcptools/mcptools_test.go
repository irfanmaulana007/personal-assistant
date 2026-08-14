package mcptools

import "testing"

func TestSplitName(t *testing.T) {
	cases := []struct {
		in           string
		wantProvider string
		wantTool     string
		wantOK       bool
	}{
		{"notion__notion-search", "notion", "notion-search", true},
		{"cloudflare__workers_list", "cloudflare", "workers_list", true},
		{"railway__list_projects", "railway", "list_projects", true},
		{"notion__a__b", "notion", "a__b", true}, // split on first separator only
		{"no-separator", "", "", false},
		{"__trailing", "", "", false},
		{"leading__", "", "", false},
		{"", "", "", false},
	}
	for _, c := range cases {
		p, tool, ok := splitName(c.in)
		if ok != c.wantOK || p != c.wantProvider || tool != c.wantTool {
			t.Errorf("splitName(%q) = (%q,%q,%v), want (%q,%q,%v)",
				c.in, p, tool, ok, c.wantProvider, c.wantTool, c.wantOK)
		}
	}
}

func TestHandles(t *testing.T) {
	p := &Provider{}
	if !p.Handles("notion__notion-search") {
		t.Error("should handle a notion-namespaced tool")
	}
	if p.Handles("GMAIL_SEND_EMAIL") {
		t.Error("must not handle a Composio tool")
	}
	if p.Handles("unknown__tool") {
		t.Error("must not handle an unknown provider")
	}
}
