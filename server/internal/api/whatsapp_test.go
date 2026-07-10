package api

import "testing"

func TestNormalizeWhatsAppJID(t *testing.T) {
	cases := map[string]string{
		"6285121503971":                "6285121503971@s.whatsapp.net",
		"+62 851-2150-3971":            "6285121503971@s.whatsapp.net",
		"6285121503971@s.whatsapp.net": "6285121503971@s.whatsapp.net",
		"  6285121503972  ":            "6285121503972@s.whatsapp.net",
		"55057957568710@lid":           "55057957568710@lid",
		"":                             "",
		"   ":                          "",
		"abc":                          "",
	}
	for in, want := range cases {
		if got := normalizeWhatsAppJID(in); got != want {
			t.Errorf("normalizeWhatsAppJID(%q) = %q, want %q", in, got, want)
		}
	}
}
