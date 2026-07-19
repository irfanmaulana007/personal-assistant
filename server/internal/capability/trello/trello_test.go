package trello

import (
	"reflect"
	"testing"

	"github.com/irfanmaulana007/personal-assistant/server/internal/trello"
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
