package translate

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

// fakeTraceRecorder captures the trace a translation logs.
type fakeTraceRecorder struct {
	trace *store.Trace
	id    int64
	err   error
	calls int
}

func (f *fakeTraceRecorder) CreateTrace(_ context.Context, t *store.Trace) (int64, error) {
	f.calls++
	f.trace = t
	return f.id, f.err
}

// fakeJudge records the trace ids handed to it for scoring.
type fakeJudge struct{ judged []int64 }

func (f *fakeJudge) JudgeInline(_ context.Context, traceID int64) {
	f.judged = append(f.judged, traceID)
}

func quietGroup(rec traceRecorder, judge traceJudge) *GroupService {
	return &GroupService{traces: rec, judge: judge, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestRecordTranslationLogsAndJudges(t *testing.T) {
	rec := &fakeTraceRecorder{id: 42}
	judge := &fakeJudge{}
	g := quietGroup(rec, judge)

	res := BetweenResult{
		Source: "Indonesian", Translated: "お元気ですか？", Model: "deepseek-v4-flash",
		PromptTokens: 12, CompletionTokens: 5, TotalTokens: 17,
	}
	g.recordTranslation(context.Background(), 7, "123@g.us", "Apa kabar?", res, nil, 123)

	if rec.calls != 1 {
		t.Fatalf("CreateTrace calls = %d, want 1", rec.calls)
	}
	tr := rec.trace
	if tr.UserID != 7 || tr.Platform != "whatsapp" || tr.Source != "chat" {
		t.Errorf("identity = (user %d, platform %q, source %q), want (7, whatsapp, chat)", tr.UserID, tr.Platform, tr.Source)
	}
	if tr.Input != "Apa kabar?" || tr.Output != "お元気ですか？" {
		t.Errorf("io = (%q -> %q), want (Apa kabar? -> お元気ですか？)", tr.Input, tr.Output)
	}
	if tr.Model != "deepseek-v4-flash" || tr.PromptTokens != 12 || tr.CompletionTokens != 5 || tr.TotalTokens != 17 {
		t.Errorf("usage = (%q, %d/%d/%d), want (deepseek-v4-flash, 12/5/17)", tr.Model, tr.PromptTokens, tr.CompletionTokens, tr.TotalTokens)
	}
	if tr.LatencyMs != 123 || tr.Status != "ok" || tr.Error != "" {
		t.Errorf("meta = (latency %d, status %q, err %q), want (123, ok, )", tr.LatencyMs, tr.Status, tr.Error)
	}
	if len(tr.Skills) != 1 || tr.Skills[0] != SkillKey {
		t.Errorf("skills = %v, want [%q]", tr.Skills, SkillKey)
	}
	if len(judge.judged) != 1 || judge.judged[0] != 42 {
		t.Errorf("judged = %v, want [42]", judge.judged)
	}
}

func TestRecordTranslationErrorIsLoggedButNotJudged(t *testing.T) {
	rec := &fakeTraceRecorder{id: 9}
	judge := &fakeJudge{}
	g := quietGroup(rec, judge)

	res := BetweenResult{Model: "deepseek-v4-flash", PromptTokens: 3}
	g.recordTranslation(context.Background(), 1, "123@g.us", "halo", res, io.ErrUnexpectedEOF, 50)

	if rec.calls != 1 {
		t.Fatalf("CreateTrace calls = %d, want 1", rec.calls)
	}
	if rec.trace.Status != "error" || rec.trace.Error != io.ErrUnexpectedEOF.Error() {
		t.Errorf("failed trace = (status %q, err %q), want (error, %q)", rec.trace.Status, rec.trace.Error, io.ErrUnexpectedEOF)
	}
	if len(judge.judged) != 0 {
		t.Errorf("judged = %v, want none for a failed translation", judge.judged)
	}
}

func TestRecordTranslationNoRecorderIsNoop(t *testing.T) {
	// With no trace recorder wired, recording must not panic and must not judge.
	judge := &fakeJudge{}
	g := quietGroup(nil, judge)
	g.recordTranslation(context.Background(), 1, "123@g.us", "halo", BetweenResult{Translated: "hello"}, nil, 1)
	if len(judge.judged) != 0 {
		t.Errorf("judged = %v, want none without a recorder", judge.judged)
	}
}

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

		{"mode without arg", "/t mode", true, cmdMode, "", "", ""},
		{"mode both", "/t mode both", true, cmdMode, "", "", ""},
		{"mode only", "/t mode only", true, cmdMode, "", "", ""},
		{"display alias", "/t display translation", true, cmdMode, "", "", ""},
		{"formality without arg", "/t formality", true, cmdFormality, "", "", ""},
		{"formality casual", "/t formality casual", true, cmdFormality, "", "", ""},
		{"tone alias formal", "/t tone formal", true, cmdFormality, "", "", ""},
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

func TestIsCommand(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"plain prompt is not a command", "hello there", false},
		{"general prompt with mention is not a command", "@6281234567890 what's the weather?", false},
		{"t as a word is not a command", "the quick brown fox", false},
		{"bare /t is a command", "/t", true},
		{"translate is a command", "/t Apa kabar?", true},
		{"translate without mention is a command", "/t Halo Pak Tanaka", true},
		{"long form is a command", "/translate お元気ですか？", true},
		{"set is a command", "/t set Indonesian Japanese", true},
		{"command after a mention is a command", "@6281234567890 /t Apa kabar?", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsCommand(tt.in); got != tt.want {
				t.Errorf("IsCommand(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseMode(t *testing.T) {
	cases := []struct {
		in       string
		wantMode string
		wantHas  bool
	}{
		{"", "", false},
		{"both", modeBoth, true},
		{"BOTH", modeBoth, true},
		{"input", modeBoth, true},
		{"only", modeOnly, true},
		{"translation", modeOnly, true},
		{"translated-only", modeOnly, true},
		{"nonsense", "", true}, // arg given but unrecognised
	}
	for _, c := range cases {
		mode, has := parseMode(c.in)
		if mode != c.wantMode || has != c.wantHas {
			t.Errorf("parseMode(%q) = (%q,%v), want (%q,%v)", c.in, mode, has, c.wantMode, c.wantHas)
		}
	}
}

func TestParseFormality(t *testing.T) {
	cases := []struct {
		in       string
		wantForm string
		wantHas  bool
	}{
		{"", "", false},
		{"asis", FormalityAsIs, true},
		{"as-is", FormalityAsIs, true},
		{"casual", FormalityCasual, true},
		{"informal", FormalityCasual, true},
		{"formal", FormalityFormal, true},
		{"sopan", FormalityFormal, true},
		{"weird", "", true}, // arg given but unrecognised
	}
	for _, c := range cases {
		form, has := parseFormality(c.in)
		if form != c.wantForm || has != c.wantHas {
			t.Errorf("parseFormality(%q) = (%q,%v), want (%q,%v)", c.in, form, has, c.wantForm, c.wantHas)
		}
	}
}

func TestFormatTranslation(t *testing.T) {
	// modeOnly with a known source → target flag + translation only, no original.
	got := formatTranslation("Indonesian", "Japanese", "Indonesian", "Apa kabar?", "お元気ですか？", modeOnly)
	if want := "🇯🇵 お元気ですか？"; got != want {
		t.Errorf("only: got %q, want %q", got, want)
	}
	// Empty mode defaults to translation-only.
	got = formatTranslation("Indonesian", "Japanese", "Indonesian", "Apa kabar?", "お元気ですか？", "")
	if want := "🇯🇵 お元気ですか？"; got != want {
		t.Errorf("default only: got %q, want %q", got, want)
	}
	// modeBoth shows both lines (source first) like formatBoth.
	got = formatTranslation("Indonesian", "Japanese", "Indonesian", "Apa kabar?", "お元気ですか？", modeBoth)
	if want := "🇮🇩 Apa kabar?\n🇯🇵 お元気ですか？"; got != want {
		t.Errorf("both: got %q, want %q", got, want)
	}
	// modeOnly with unknown source falls back to the globe.
	got = formatTranslation("Indonesian", "Japanese", "", "whatever", "translated", modeOnly)
	if want := "🌐 translated"; got != want {
		t.Errorf("only unknown: got %q, want %q", got, want)
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
