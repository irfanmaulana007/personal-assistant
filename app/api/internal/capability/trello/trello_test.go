package trello

import (
	"reflect"
	"strings"
	"testing"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/trello"
)

func TestMatchCardsPrefersExact(t *testing.T) {
	cards := []trello.Card{
		{ID: "1", Name: "Add dark mode"},
		{ID: "2", Name: "Add dark mode toggle to settings"},
		{ID: "3", Name: "Fix login"},
	}

	// Exact (case-insensitive) title wins over the substring match.
	got := matchCards(cards, "add dark mode")
	if len(got) != 1 || got[0].ID != "1" {
		t.Fatalf("exact match = %+v, want just card 1", got)
	}

	// No exact match → all substring matches are returned (ambiguous).
	got = matchCards(cards, "dark")
	if len(got) != 2 {
		t.Fatalf("substring match count = %d, want 2", len(got))
	}

	// No match at all.
	if got := matchCards(cards, "nonexistent"); len(got) != 0 {
		t.Fatalf("no-match = %+v, want empty", got)
	}

	// Empty query matches nothing (never every card).
	if got := matchCards(cards, "   "); len(got) != 0 {
		t.Fatalf("empty query = %+v, want empty", got)
	}
}

func TestSplitAcceptanceCriteria(t *testing.T) {
	desc := "Some context about the work.\n\n## Acceptance Criteria\n- [ ] First item\n- [x] Second item\n- Third item\n"
	context, criteria := splitAcceptanceCriteria(desc)
	if context != "Some context about the work." {
		t.Errorf("context = %q", context)
	}
	want := []string{"First item", "Second item", "Third item"}
	if !reflect.DeepEqual(criteria, want) {
		t.Errorf("criteria = %v, want %v", criteria, want)
	}

	// No heading → whole body is context, no criteria.
	context, criteria = splitAcceptanceCriteria("Just a description")
	if context != "Just a description" || criteria != nil {
		t.Errorf("no-heading split = (%q, %v)", context, criteria)
	}
}

// Round-tripping a body through split then rebuild must preserve both parts, so
// updating one field never drops the other.
func TestBuildTaskBodyRoundTrip(t *testing.T) {
	orig := "Context line one.\nContext line two.\n\n## Acceptance Criteria\n- [ ] A\n- [ ] B"
	context, criteria := splitAcceptanceCriteria(orig)
	rebuilt := buildTaskBody(context, criteria)
	// Re-split the rebuilt body; it must carry the same parts.
	c2, cr2 := splitAcceptanceCriteria(rebuilt)
	if c2 != context {
		t.Errorf("context drifted: %q -> %q", context, c2)
	}
	if !reflect.DeepEqual(cr2, criteria) {
		t.Errorf("criteria drifted: %v -> %v", criteria, cr2)
	}

	// Replacing just the context keeps the criteria section intact.
	body := buildTaskBody("New context", criteria)
	c3, cr3 := splitAcceptanceCriteria(body)
	if c3 != "New context" || !reflect.DeepEqual(cr3, criteria) {
		t.Errorf("context-only update = (%q, %v)", c3, cr3)
	}

	// No criteria → body is just the context, no dangling heading.
	if got := buildTaskBody("Only context", nil); got != "Only context" {
		t.Errorf("no-criteria body = %q", got)
	}
}

// checkPersisted is the read-after-write gate: a card only counts as saved if it
// was read back, isn't archived, and sits on the list we filed it to.
func TestCheckPersisted(t *testing.T) {
	const wantList = "list1"

	// Happy path: live card on the expected list.
	if err := checkPersisted(&trello.Card{ID: "c1", IDList: wantList}, wantList); err != nil {
		t.Errorf("live card on expected list should pass, got %v", err)
	}

	// Nil card (create returned nothing readable) → not persisted.
	if err := checkPersisted(nil, wantList); err == nil {
		t.Error("nil card should fail verification")
	}

	// Archived card → not persisted, even though it exists.
	if err := checkPersisted(&trello.Card{ID: "c1", IDList: wantList, Closed: true}, wantList); err == nil {
		t.Error("archived card should fail verification")
	}

	// Card landed on a different list (e.g. wrong board/creds) → not persisted.
	if err := checkPersisted(&trello.Card{ID: "c1", IDList: "someOtherList"}, wantList); err == nil {
		t.Error("card on the wrong list should fail verification")
	}

	// Empty wantList skips the list check (still enforces existence + not archived).
	if err := checkPersisted(&trello.Card{ID: "c1", IDList: "anything"}, ""); err != nil {
		t.Errorf("empty wantList should skip the list check, got %v", err)
	}
}

func TestMatchList(t *testing.T) {
	lists := []trello.List{
		{ID: "l1", Name: "Backlog"},
		{ID: "l2", Name: "Todo"},
		{ID: "l3", Name: "In Progress"},
		{ID: "l4", Name: "Done"},
	}

	// Exact, case-insensitive.
	if id, name, ok := matchList(lists, "in progress"); !ok || id != "l3" || name != "In Progress" {
		t.Errorf("exact = (%q, %q, %v)", id, name, ok)
	}
	// Substring fallback ("progress" -> "In Progress").
	if id, _, ok := matchList(lists, "progress"); !ok || id != "l3" {
		t.Errorf("substring = (%q, %v)", id, ok)
	}
	// Unknown list.
	if _, _, ok := matchList(lists, "archive"); ok {
		t.Error("archive should not match any list")
	}
	// Empty name matches nothing.
	if _, _, ok := matchList(lists, ""); ok {
		t.Error("empty name should not match")
	}
}

func TestMatchBoard(t *testing.T) {
	boards := []boardRef{
		{ID: "b1", Name: "Task Management"},
		{ID: "b2", Name: "Issue Tracker"},
		{ID: "b3", Name: "Side Projects"},
	}

	// Exact, case-insensitive.
	if b, ok := matchBoard(boards, "issue tracker"); !ok || b.ID != "b2" {
		t.Errorf("exact = (%+v, %v)", b, ok)
	}
	// Substring fallback ("side" -> "Side Projects").
	if b, ok := matchBoard(boards, "side"); !ok || b.ID != "b3" {
		t.Errorf("substring = (%+v, %v)", b, ok)
	}
	// Unknown board.
	if _, ok := matchBoard(boards, "roadmap"); ok {
		t.Error("roadmap should not match any board")
	}
	// Empty query matches nothing.
	if _, ok := matchBoard(boards, "  "); ok {
		t.Error("empty query should not match")
	}
	// An unnamed fallback board is never matched by name.
	if _, ok := matchBoard([]boardRef{{ID: "b0"}}, "anything"); ok {
		t.Error("unnamed board should not match by name")
	}
}

func TestPickBoard(t *testing.T) {
	one := []boardRef{{ID: "b1", Name: "Task Management"}}
	many := []boardRef{
		{ID: "b1", Name: "Task Management"},
		{ID: "b2", Name: "Issue Tracker"},
	}

	// No boards linked → the not-configured message, no board.
	if board, msg := pickBoard(nil, ""); msg != boardNotConfiguredMsg || board.ID != "" {
		t.Errorf("no boards = (%+v, %q)", board, msg)
	}

	// Exactly one board, none named → use it, no message.
	if board, msg := pickBoard(one, ""); msg != "" || board.ID != "b1" {
		t.Errorf("single = (%+v, %q)", board, msg)
	}

	// Several boards, none named → ask which (message set, no board chosen).
	board, msg := pickBoard(many, "")
	if board.ID != "" || msg == "" {
		t.Fatalf("ambiguous = (%+v, %q), want a prompt and no board", board, msg)
	}
	if !strings.Contains(msg, "Task Management") || !strings.Contains(msg, "Issue Tracker") {
		t.Errorf("ambiguous prompt should list both boards: %q", msg)
	}

	// Several boards, one named → resolve to it.
	if board, msg := pickBoard(many, "issue"); msg != "" || board.ID != "b2" {
		t.Errorf("named = (%+v, %q)", board, msg)
	}

	// Named board that isn't linked → a prompt naming the linked boards, no board.
	if board, msg := pickBoard(many, "Roadmap"); board.ID != "" || !strings.Contains(msg, "Roadmap") {
		t.Errorf("unknown named = (%+v, %q)", board, msg)
	}
}
