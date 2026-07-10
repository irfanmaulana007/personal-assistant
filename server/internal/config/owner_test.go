package config

import "testing"

func TestOwnerAllowedAndPrimaryJID(t *testing.T) {
	cases := []struct {
		in      string
		allowed []string
		primary string
	}{
		{"", nil, ""},
		{"628971@s.whatsapp.net", []string{"628971@s.whatsapp.net"}, "628971@s.whatsapp.net"},
		{
			" 628971@s.whatsapp.net , 628972@s.whatsapp.net ",
			[]string{"628971@s.whatsapp.net", "628972@s.whatsapp.net"},
			"628971@s.whatsapp.net",
		},
		{",,628972@s.whatsapp.net,", []string{"628972@s.whatsapp.net"}, "628972@s.whatsapp.net"},
	}
	for _, c := range cases {
		o := OwnerConfig{WhatsAppJID: c.in}
		got := o.AllowedJIDs()
		if len(got) != len(c.allowed) {
			t.Fatalf("AllowedJIDs(%q) = %v, want %v", c.in, got, c.allowed)
		}
		for i := range got {
			if got[i] != c.allowed[i] {
				t.Fatalf("AllowedJIDs(%q) = %v, want %v", c.in, got, c.allowed)
			}
		}
		if p := o.PrimaryJID(); p != c.primary {
			t.Errorf("PrimaryJID(%q) = %q, want %q", c.in, p, c.primary)
		}
	}
}
