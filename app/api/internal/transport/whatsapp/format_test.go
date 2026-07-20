package whatsapp

import "testing"

func TestToWhatsAppMarkup(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"plain text untouched", "hello there", "hello there"},
		{"bold double star", "say **hello** now", "say *hello* now"},
		{"bold double underscore", "say __hello__ now", "say *hello* now"},
		{"italic single star", "say *hello* now", "say _hello_ now"},
		{"italic underscore left alone", "say _hello_ now", "say _hello_ now"},
		{"bold then italic in one line", "**bold** and *italic*", "*bold* and _italic_"},
		{"bold not chewed by italic pass", "a **b** c", "a *b* c"},
		{"strikethrough", "~~gone~~", "~gone~"},
		{"heading to bold", "# Title", "*Title*"},
		{"h3 to bold", "### Sub Title", "*Sub Title*"},
		{"dash bullet to glyph", "- item one", "• item one"},
		{"star bullet to glyph", "* item one", "• item one"},
		{"plus bullet to glyph", "+ item one", "• item one"},
		{"indented bullet keeps indent", "  - nested", "  • nested"},
		{"horizontal rule removed", "above\n---\nbelow", "above\n\nbelow"},
		{"link to text and url", "see [Google](https://google.com)", "see Google (https://google.com)"},
		{"blockquote strips marker", "> quoted line", "quoted line"},
		{"snake_case not italicized", "call list_calendar tool", "call list_calendar tool"},
		{
			"inline code to monospace",
			"run `make build` now",
			"run ```make build``` now",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toWhatsAppMarkup(tt.in); got != tt.want {
				t.Errorf("toWhatsAppMarkup(%q)\n got: %q\nwant: %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestToWhatsAppMarkupFencedCodeUntouched(t *testing.T) {
	in := "here:\n```go\nfmt.Println(\"*not bold*\")\n```\ndone"
	want := "here:\n```fmt.Println(\"*not bold*\")```\ndone"
	if got := toWhatsAppMarkup(in); got != want {
		t.Errorf("fenced code\n got: %q\nwant: %q", got, want)
	}
}

func TestToWhatsAppMarkupBulletedList(t *testing.T) {
	in := "Your reminders:\n- **Gym** at 7am\n- *Dentist* on Friday"
	want := "Your reminders:\n• *Gym* at 7am\n• _Dentist_ on Friday"
	if got := toWhatsAppMarkup(in); got != want {
		t.Errorf("list\n got: %q\nwant: %q", got, want)
	}
}

func TestFormatGrammarReply(t *testing.T) {
	tests := []struct {
		name     string
		original string
		reply    string
		want     string
	}{
		{
			name:     "no grammar block returned unchanged",
			original: "halo",
			reply:    "Halo! Ada yang bisa dibantu?",
			want:     "Halo! Ada yang bisa dibantu?",
		},
		{
			name:     "correction with changed words bolded",
			original: "i has two apple",
			reply:    "[[grammar]]I have two apples[[/grammar]]Sure, I noted that.",
			want:     "📝 **English check**\n~~i has two apple~~\n✅ I **have** two **apples**\n\nSure, I noted that.",
		},
		{
			name:     "consecutive changes merge into one bold run",
			original: "i go store",
			reply:    "[[grammar]]I went to the store[[/grammar]]Got it.",
			want:     "📝 **English check**\n~~i go store~~\n✅ I **went to the** store\n\nGot it.",
		},
		{
			name:     "already correct shows looks-good line",
			original: "I am happy today",
			reply:    "[[grammar]]I am happy today[[/grammar]]Glad to hear it!",
			want:     "📝 **English check**\n✅ I am happy today — looks good! 👍\n\nGlad to hear it!",
		},
		{
			name:     "grammar block with no reply body renders card only",
			original: "i is fine",
			reply:    "[[grammar]]I am fine[[/grammar]]",
			want:     "📝 **English check**\n~~i is fine~~\n✅ I **am** fine",
		},
		{
			name:     "empty grammar block falls back to body",
			original: "x",
			reply:    "[[grammar]][[/grammar]]hello",
			want:     "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatGrammarReply(tt.original, tt.reply); got != tt.want {
				t.Errorf("FormatGrammarReply(%q, %q)\n got: %q\nwant: %q", tt.original, tt.reply, got, tt.want)
			}
		})
	}
}

// TestFormatGrammarReplyThroughMarkup proves the card renders as valid WhatsApp
// markup once SendMessage runs it through toWhatsAppMarkup — the double stars
// become WhatsApp bold and the strikethrough collapses to a single tilde,
// instead of leaking literal Markdown into the chat.
func TestFormatGrammarReplyThroughMarkup(t *testing.T) {
	md := FormatGrammarReply("i has two apple", "[[grammar]]I have two apples[[/grammar]]Sure, I noted that.")
	want := "📝 *English check*\n~i has two apple~\n✅ I *have* two *apples*\n\nSure, I noted that."
	if got := toWhatsAppMarkup(md); got != want {
		t.Errorf("markup\n got: %q\nwant: %q", got, want)
	}
}
