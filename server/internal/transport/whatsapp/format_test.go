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
