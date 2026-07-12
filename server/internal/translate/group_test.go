package translate

import "testing"

func TestParseCommand(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		wantOK   bool
		wantKind cmdKind
		wantA    string
		wantB    string
		wantText string
	}{
		{"plain text is not a command", "hello there", false, 0, "", "", ""},
		{"mention only is not a command", "@6281234567890", false, 0, "", "", ""},
		{"t as a word is not a command", "the quick brown fox", false, 0, "", "", ""},

		{"translate after leading mention", "@6281234567890 /t Apa kabar?", true, cmdTranslate, "", "", "Apa kabar?"},
		{"translate with trailing mention", "/t Apa kabar? @6281234567890", true, cmdTranslate, "", "", "Apa kabar?"},
		{"translate long form", "/translate お元気ですか？", true, cmdTranslate, "", "", "お元気ですか？"},
		{"translate preserves punctuation & casing", "/t Halo, Pak Tanaka!", true, cmdTranslate, "", "", "Halo, Pak Tanaka!"},

		{"set two words", "/t set Indonesian Japanese", true, cmdSet, "Indonesian", "Japanese", ""},
		{"set with mention", "@628123456789 /t set indonesian japanese", true, cmdSet, "Indonesian", "Japanese", ""},
		{"set with 'and' separator multiword", "/t set Bahasa Indonesia and Japanese", true, cmdSet, "Bahasa Indonesia", "Japanese", ""},
		{"set with slash", "/t set id/ja", true, cmdSet, "Id", "Ja", ""},
		{"set with arrow", "/t set Indonesian ↔ German", true, cmdSet, "Indonesian", "German", ""},
		{"set missing second language", "/t set Indonesian", true, cmdSet, "", "", ""},

		{"off", "/t off", true, cmdOff, "", "", ""},
		{"clear alias", "/t clear", true, cmdOff, "", "", ""},
		{"status", "/t status", true, cmdStatus, "", "", ""},
		{"languages alias", "/t languages", true, cmdStatus, "", "", ""},
		{"help", "/t help", true, cmdHelp, "", "", ""},
		{"bare t is translate with empty text", "/t", true, cmdTranslate, "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, ok := parseCommand(tt.in)
			if ok != tt.wantOK {
				t.Fatalf("parseCommand(%q) ok = %v, want %v", tt.in, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if cmd.kind != tt.wantKind {
				t.Errorf("kind = %v, want %v", cmd.kind, tt.wantKind)
			}
			if cmd.langA != tt.wantA || cmd.langB != tt.wantB {
				t.Errorf("langs = (%q,%q), want (%q,%q)", cmd.langA, cmd.langB, tt.wantA, tt.wantB)
			}
			if cmd.text != tt.wantText {
				t.Errorf("text = %q, want %q", cmd.text, tt.wantText)
			}
		})
	}
}

func TestStripMentions(t *testing.T) {
	cases := map[string]string{
		"@6281234567890 /t hi":     "/t hi",
		"/t hi @6281234567890":     "/t hi",
		"meeting @ 5pm with @1234": "meeting @ 5pm with @1234", // short @token kept, "@ 5pm" untouched
		"john@example.com wrote":   "john@example.com wrote",   // email is not a mention
	}
	for in, want := range cases {
		if got := stripMentions(in); got != want {
			t.Errorf("stripMentions(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatBoth(t *testing.T) {
	// Both languages recognised → flag labels, source line first.
	got := formatBoth("Indonesian", "Japanese", "Indonesian", "Apa kabar?", "お元気ですか？")
	want := "🇮🇩 Apa kabar?\n🇯🇵 お元気ですか？"
	if got != want {
		t.Errorf("flags: got %q, want %q", got, want)
	}

	// Source is language B → target is A, and order still shows source first.
	got = formatBoth("Indonesian", "Japanese", "Japanese", "お元気ですか？", "Apa kabar?")
	want = "🇯🇵 お元気ですか？\n🇮🇩 Apa kabar?"
	if got != want {
		t.Errorf("reverse: got %q, want %q", got, want)
	}

	// Unknown language falls back to text labels for both lines.
	got = formatBoth("Klingon", "Elvish", "Klingon", "nuqneH", "elen síla")
	want = "Klingon: nuqneH\nElvish: elen síla"
	if got != want {
		t.Errorf("labels: got %q, want %q", got, want)
	}

	// Unknown source (model didn't report it) → translation only.
	got = formatBoth("Indonesian", "Japanese", "", "whatever", "translated")
	if got != "🌐 translated" {
		t.Errorf("unknown source: got %q", got)
	}
}

func TestStripFences(t *testing.T) {
	cases := map[string]string{
		"{\"a\":1}":               "{\"a\":1}",
		"```json\n{\"a\":1}\n```": "{\"a\":1}",
		"```\n{\"a\":1}\n```":     "{\"a\":1}",
		"  {\"a\":1}  ":           "{\"a\":1}",
		"```{\"a\":1}```":         "{\"a\":1}",
	}
	for in, want := range cases {
		if got := stripFences(in); got != want {
			t.Errorf("stripFences(%q) = %q, want %q", in, got, want)
		}
	}
}
