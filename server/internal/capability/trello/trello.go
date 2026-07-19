// Package trello implements the Trello skills: one that reviews every task and
// bug across the user's project boards, one that files a new card — a task on
// the Task Management "Backlog" or a bug report on the Issue "Bug" list — and
// one that captures an enriched game idea on the "Games" board's Ideas list.
//
// The workspace/board/list/label ids below are fixed to the user's Trello
// workspaces ("Personal Assistant" for tasks/bugs, "Games" for game ideas). The
// handler only reads and writes; credentials
// (API key + token) are resolved per call from encrypted settings, and a missing
// credential is reported back to the model as plain text so it can tell the user
// to configure it on the Integrations page.
package trello

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/irfanmaulana007/personal-assistant/server/internal/intent"
	"github.com/irfanmaulana007/personal-assistant/server/internal/settings"
	"github.com/irfanmaulana007/personal-assistant/server/internal/trello"
)

// Fixed ids for the "Personal Assistant" workspace boards and lists.
const (
	boardTaskManagement = "6a54dd8eecaab3bd510528ba"
	listBacklog         = "6a54dda0cf5b49c7fb6f8b15"

	boardIssue = "6a54edaae21957ab935c81f6"
	listBug    = "6a54edaae21957ab935c820f"

	// The "Games" workspace board and its Ideas list — where captured game
	// ideas are filed.
	listGameIdeas = "6a5a453925d775e49d6972d7"
)

// backlogLabels maps a task type to its Trello label id on the Task Management
// board. The model picks one of these keys when filing a task.
var backlogLabels = map[string]string{
	"feature":     "6a54dd8eecaab3bd510528d4",
	"improvement": "6a54dd8eecaab3bd510528d7",
	"chore":       "6a54dd8eecaab3bd510528d6",
	"refactor":    "6a54dd8eecaab3bd510528d5",
}

const notConfiguredMsg = "Trello is not configured — no Trello API key/token has been set. Ask the user to add their Trello API key and token on the Integrations page."

// Handler answers Trello tool calls (review boards, file a task, report a bug).
type Handler struct {
	client   *trello.Client
	settings *settings.Service
	log      *slog.Logger
}

// New creates a Trello capability handler.
func New(client *trello.Client, settingsSvc *settings.Service, log *slog.Logger) *Handler {
	return &Handler{client: client, settings: settingsSvc, log: log.With("component", "trello")}
}

func (h *Handler) Name() string { return "trello" }

func (h *Handler) Match(result *intent.ParseResult) bool {
	return result.Capability == intent.CapabilityTrello
}

func (h *Handler) Handle(ctx context.Context, result *intent.ParseResult) (string, error) {
	apiKey, token, err := h.settings.TrelloCreds(ctx)
	if err != nil {
		return "", fmt.Errorf("resolve trello creds: %w", err)
	}
	if apiKey == "" || token == "" {
		return notConfiguredMsg, nil
	}

	switch result.Action {
	case intent.ActionTrelloReview:
		return h.review(ctx, apiKey, token)
	case intent.ActionTrelloCreateTask:
		return h.createTask(ctx, apiKey, token, result.Entities)
	case intent.ActionTrelloReportBug:
		return h.reportBug(ctx, apiKey, token, result.Entities)
	case intent.ActionTrelloUpdateCard:
		return h.updateCard(ctx, apiKey, token, result.Entities)
	case intent.ActionTrelloGameIdea:
		return h.createGameIdea(ctx, apiKey, token, result.Entities)
	default:
		return "I understood a Trello request but not which action to take.", nil
	}
}

// review lists every card on both boards, grouped by board and list.
func (h *Handler) review(ctx context.Context, apiKey, token string) (string, error) {
	var b strings.Builder
	for _, board := range []struct{ name, id string }{
		{"Task Management", boardTaskManagement},
		{"Issue", boardIssue},
	} {
		lists, err := h.client.BoardLists(ctx, apiKey, token, board.id)
		if err != nil {
			h.log.Warn("trello list lists failed", "board", board.name, "error", err)
			return fmt.Sprintf("Couldn't read the %s board: %v", board.name, err), nil
		}
		cards, err := h.client.BoardCards(ctx, apiKey, token, board.id)
		if err != nil {
			h.log.Warn("trello list cards failed", "board", board.name, "error", err)
			return fmt.Sprintf("Couldn't read the %s board: %v", board.name, err), nil
		}
		byList := map[string][]trello.Card{}
		for _, c := range cards {
			byList[c.IDList] = append(byList[c.IDList], c)
		}

		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(fmt.Sprintf("# %s board\n", board.name))
		for _, l := range lists {
			items := byList[l.ID]
			b.WriteString(fmt.Sprintf("\n## %s (%d)\n", l.Name, len(items)))
			if len(items) == 0 {
				b.WriteString("_(empty)_\n")
				continue
			}
			for _, c := range items {
				b.WriteString("- " + strings.TrimSpace(c.Name))
				if labels := labelNames(c.Labels); labels != "" {
					b.WriteString(" [" + labels + "]")
				}
				b.WriteString("\n")
			}
		}
	}
	b.WriteString("\nSummarize this for the user in their language: how many open tasks and bugs, and anything currently in progress. Don't invent cards beyond this list.")
	return b.String(), nil
}

// createTask files a task on the Task Management → Backlog list, with a chosen
// label and an "Acceptance Criteria" checklist.
func (h *Handler) createTask(ctx context.Context, apiKey, token string, e map[string]string) (string, error) {
	title := strings.TrimSpace(e["title"])
	if title == "" {
		return "What's the task? I need a short title to add it to the backlog.", nil
	}
	desc := strings.TrimSpace(e["description"])
	criteria := splitLines(e["acceptance_criteria"])

	body := desc
	if len(criteria) > 0 {
		if body != "" {
			body += "\n\n"
		}
		body += "## Acceptance Criteria\n"
		for _, c := range criteria {
			body += "- [ ] " + c + "\n"
		}
	}

	in := trello.CreateCardInput{ListID: listBacklog, Name: title, Desc: body}
	labelKey := strings.ToLower(strings.TrimSpace(e["label"]))
	labelID, labelOK := backlogLabels[labelKey]
	if labelOK {
		in.LabelIDs = []string{labelID}
	}

	card, err := h.client.CreateCard(ctx, apiKey, token, in)
	if err != nil {
		h.log.Warn("trello create task failed", "error", err)
		return fmt.Sprintf("Couldn't create the task card: %v", err), nil
	}

	// Read-after-write: confirm the card actually exists on Trello before telling
	// the user it was filed.
	if _, err := h.client.GetCard(ctx, apiKey, token, card.ID); err != nil {
		h.log.Warn("trello verify task failed", "card", card.ID, "error", err)
		return fmt.Sprintf("I tried to create the task card but couldn't verify it saved on Trello: %v", err), nil
	}

	// Add the acceptance criteria as a real, trackable checklist too.
	if len(criteria) > 0 {
		if clID, err := h.client.AddChecklist(ctx, apiKey, token, card.ID, "Acceptance Criteria"); err != nil {
			h.log.Warn("trello add checklist failed", "card", card.ID, "error", err)
		} else {
			for _, c := range criteria {
				if err := h.client.AddCheckItem(ctx, apiKey, token, clID, c); err != nil {
					h.log.Warn("trello add check item failed", "card", card.ID, "error", err)
				}
			}
		}
	}

	labelNote := "no label"
	if labelOK {
		labelNote = labelKey
	}
	return fmt.Sprintf("Added task %q to the Task Management → Backlog list (label: %s).\n%s\nConfirm this to the user in their language.", title, labelNote, card.ShortURL), nil
}

// reportBug files a bug on the Issue → Bug list, with Actual/Expected sections.
func (h *Handler) reportBug(ctx context.Context, apiKey, token string, e map[string]string) (string, error) {
	title := strings.TrimSpace(e["title"])
	if title == "" {
		return "What's the bug? I need a short title to file it on the Issue board.", nil
	}
	desc := strings.TrimSpace(e["description"])
	actual := strings.TrimSpace(e["actual_result"])
	expected := strings.TrimSpace(e["expected_result"])

	var parts []string
	if desc != "" {
		parts = append(parts, desc)
	}
	if actual != "" {
		parts = append(parts, "## Actual Result\n"+actual)
	}
	if expected != "" {
		parts = append(parts, "## Expected Result\n"+expected)
	}
	body := strings.Join(parts, "\n\n")

	card, err := h.client.CreateCard(ctx, apiKey, token, trello.CreateCardInput{ListID: listBug, Name: title, Desc: body})
	if err != nil {
		h.log.Warn("trello report bug failed", "error", err)
		return fmt.Sprintf("Couldn't file the bug card: %v", err), nil
	}

	// Read-after-write: confirm the card actually exists on Trello before telling
	// the user it was filed.
	if _, err := h.client.GetCard(ctx, apiKey, token, card.ID); err != nil {
		h.log.Warn("trello verify bug failed", "card", card.ID, "error", err)
		return fmt.Sprintf("I tried to file the bug card but couldn't verify it saved on Trello: %v", err), nil
	}
	return fmt.Sprintf("Filed bug %q on the Issue → Bug list.\n%s\nConfirm this to the user in their language.", title, card.ShortURL), nil
}

// acceptanceHeader is the Markdown heading under which a task card's acceptance
// criteria live in its description body (matching how createTask writes them).
const acceptanceHeader = "## Acceptance Criteria"

// updateCard edits an existing task card on the Task Management board — its
// title, description, acceptance criteria, type label, or the list it sits in.
// The card is identified by (part of) its current title, since the review tool
// surfaces titles rather than ids. Only the fields the model actually supplied
// are changed; everything else is left untouched.
func (h *Handler) updateCard(ctx context.Context, apiKey, token string, e map[string]string) (string, error) {
	query := strings.TrimSpace(e["card"])
	if query == "" {
		return "Which card should I update? Tell me its title (or part of it).", nil
	}

	cards, err := h.client.BoardCards(ctx, apiKey, token, boardTaskManagement)
	if err != nil {
		h.log.Warn("trello list cards failed", "error", err)
		return fmt.Sprintf("Couldn't read the Task Management board to find that card: %v", err), nil
	}
	matches := matchCards(cards, query)
	switch len(matches) {
	case 0:
		return fmt.Sprintf("I couldn't find a card matching %q on the Task Management board. Try the exact title from a board review.", query), nil
	case 1:
		// exactly one — proceed
	default:
		var names []string
		for _, c := range matches {
			names = append(names, fmt.Sprintf("%q", strings.TrimSpace(c.Name)))
		}
		return fmt.Sprintf("That matches %d cards: %s. Which one? Give me a more specific title.", len(matches), strings.Join(names, ", ")), nil
	}
	card := matches[0]

	var (
		in      trello.UpdateCardInput
		changed []string
	)

	// Title.
	if v, ok := e["title"]; ok {
		if t := strings.TrimSpace(v); t != "" {
			in.Name = &t
			changed = append(changed, "title")
		}
	}

	// Description and acceptance criteria both live in the card body; split the
	// current body so we can replace one without dropping the other.
	_, descGiven := e["description"]
	rawCriteria, critGiven := e["acceptance_criteria"]
	var newCriteria []string
	if descGiven || critGiven {
		curContext, curCriteria := splitAcceptanceCriteria(card.Desc)
		newContext := curContext
		if descGiven {
			newContext = strings.TrimSpace(e["description"])
			changed = append(changed, "description")
		}
		newCriteria = curCriteria
		if critGiven {
			newCriteria = splitLines(rawCriteria)
			changed = append(changed, "acceptance criteria")
		}
		body := buildTaskBody(newContext, newCriteria)
		in.Desc = &body
	}

	// Label: set a new type label, or clear all labels with "none".
	if v, ok := e["label"]; ok {
		key := strings.ToLower(strings.TrimSpace(v))
		switch {
		case key == "" || key == "none" || key == "remove":
			empty := []string{}
			in.LabelIDs = &empty
			changed = append(changed, "label (removed)")
		default:
			id, valid := backlogLabels[key]
			if !valid {
				return fmt.Sprintf("%q isn't a valid label. Use one of: feature, improvement, chore, refactor (or none to clear).", v), nil
			}
			ids := []string{id}
			in.LabelIDs = &ids
			changed = append(changed, "label ("+key+")")
		}
	}

	// Move to a different list on the same board (e.g. Backlog → In Progress).
	if v, ok := e["list"]; ok {
		if name := strings.TrimSpace(v); name != "" {
			lists, err := h.client.BoardLists(ctx, apiKey, token, boardTaskManagement)
			if err != nil {
				h.log.Warn("trello list lists failed", "error", err)
				return fmt.Sprintf("Couldn't read the board's lists to move the card: %v", err), nil
			}
			listID, listName, found := matchList(lists, name)
			if !found {
				return fmt.Sprintf("There's no %q list on the Task Management board. Available lists: %s.", name, listNames(lists)), nil
			}
			if listID != card.IDList {
				in.IDList = &listID
				changed = append(changed, "moved to "+listName)
			}
		}
	}

	if in.IsEmpty() {
		return "Nothing to change on that card — tell me what to update (title, description, acceptance criteria, label, or which list to move it to).", nil
	}

	updated, err := h.client.UpdateCard(ctx, apiKey, token, card.ID, in)
	if err != nil {
		h.log.Warn("trello update card failed", "card", card.ID, "error", err)
		return fmt.Sprintf("Couldn't update the card: %v", err), nil
	}

	// Keep the trackable "Acceptance Criteria" checklist in sync when the criteria
	// changed: drop the old checklist and rebuild it from the new items.
	if critGiven {
		h.replaceAcceptanceChecklist(ctx, apiKey, token, card.ID, newCriteria)
	}

	// Read-after-write: only report success once Trello confirms the change.
	if _, err := h.client.GetCard(ctx, apiKey, token, card.ID); err != nil {
		h.log.Warn("trello verify update failed", "card", card.ID, "error", err)
		return fmt.Sprintf("I updated the card but couldn't verify it saved on Trello: %v", err), nil
	}

	name := strings.TrimSpace(updated.Name)
	if name == "" {
		name = strings.TrimSpace(card.Name)
	}
	shortURL := updated.ShortURL
	if shortURL == "" {
		shortURL = card.ShortURL
	}
	return fmt.Sprintf("Updated %q (%s).\n%s\nConfirm this to the user in their language.", name, strings.Join(changed, ", "), shortURL), nil
}

// replaceAcceptanceChecklist rebuilds the card's "Acceptance Criteria" checklist
// so the trackable checklist matches the new criteria: it deletes any existing
// checklist by that name, then creates a fresh one. It is best-effort — the card
// body has already been updated, so failures are logged rather than surfaced.
func (h *Handler) replaceAcceptanceChecklist(ctx context.Context, apiKey, token, cardID string, criteria []string) {
	existing, err := h.client.CardChecklists(ctx, apiKey, token, cardID)
	if err != nil {
		h.log.Warn("trello read checklists failed", "card", cardID, "error", err)
	} else {
		for _, cl := range existing {
			if strings.EqualFold(strings.TrimSpace(cl.Name), "Acceptance Criteria") {
				if err := h.client.DeleteChecklist(ctx, apiKey, token, cl.ID); err != nil {
					h.log.Warn("trello delete checklist failed", "card", cardID, "error", err)
				}
			}
		}
	}
	if len(criteria) == 0 {
		return
	}
	clID, err := h.client.AddChecklist(ctx, apiKey, token, cardID, "Acceptance Criteria")
	if err != nil {
		h.log.Warn("trello add checklist failed", "card", cardID, "error", err)
		return
	}
	for _, c := range criteria {
		if err := h.client.AddCheckItem(ctx, apiKey, token, clID, c); err != nil {
			h.log.Warn("trello add check item failed", "card", cardID, "error", err)
		}
	}
}

// createGameIdea files an enriched game-idea card on the "Games" board's Ideas
// list, composing the concept, genre, core mechanics, references, and notes into
// a single well-formed brief.
func (h *Handler) createGameIdea(ctx context.Context, apiKey, token string, e map[string]string) (string, error) {
	title := strings.TrimSpace(e["title"])
	if title == "" {
		return "What's the game idea? I need a short title to add it to your Ideas list.", nil
	}
	concept := strings.TrimSpace(e["concept"])
	genre := strings.TrimSpace(e["genre"])
	mechanics := splitLines(e["core_mechanics"])
	references := splitLines(e["references"])
	notes := strings.TrimSpace(e["notes"])

	var parts []string
	if concept != "" {
		parts = append(parts, concept)
	}
	if genre != "" {
		parts = append(parts, "## Genre\n"+genre)
	}
	if len(mechanics) > 0 {
		parts = append(parts, "## Core Mechanics\n"+bulletList(mechanics))
	}
	if len(references) > 0 {
		parts = append(parts, "## References & Inspiration\n"+bulletList(references))
	}
	if notes != "" {
		parts = append(parts, "## Notes\n"+notes)
	}
	body := strings.Join(parts, "\n\n")

	card, err := h.client.CreateCard(ctx, apiKey, token, trello.CreateCardInput{ListID: listGameIdeas, Name: title, Desc: body})
	if err != nil {
		h.log.Warn("trello save game idea failed", "error", err)
		return fmt.Sprintf("Couldn't save the game idea card: %v", err), nil
	}

	// Read-after-write: confirm the card actually exists on Trello before telling
	// the user it was saved.
	if _, err := h.client.GetCard(ctx, apiKey, token, card.ID); err != nil {
		h.log.Warn("trello verify game idea failed", "card", card.ID, "error", err)
		return fmt.Sprintf("I tried to save the game idea but couldn't verify it saved on Trello: %v", err), nil
	}
	return fmt.Sprintf("Saved game idea %q to your Games board → Ideas list.\n%s\nConfirm this to the user in their language.", title, card.ShortURL), nil
}

// bulletList renders trimmed lines as a Markdown bullet list.
func bulletList(items []string) string {
	var b strings.Builder
	for _, it := range items {
		b.WriteString("- " + it + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// splitLines splits a newline-separated field into trimmed, non-empty lines,
// tolerating leading bullet markers the model may include.
func splitLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimPrefix(line, "* ")
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// matchCards returns the cards whose name matches query — preferring exact
// (case-insensitive) title matches, and otherwise any card whose title contains
// the query — so the model can target a card by the title it saw in a review.
func matchCards(cards []trello.Card, query string) []trello.Card {
	q := strings.ToLower(strings.TrimSpace(query))
	var exact, partial []trello.Card
	for _, c := range cards {
		name := strings.ToLower(strings.TrimSpace(c.Name))
		switch {
		case name == q:
			exact = append(exact, c)
		case q != "" && strings.Contains(name, q):
			partial = append(partial, c)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	return partial
}

// splitAcceptanceCriteria splits a task card body into its context (everything
// before the "## Acceptance Criteria" heading) and the criteria lines under it,
// with their "- [ ]" / "- [x]" markers stripped. A body with no such heading
// returns the whole body as context and no criteria.
func splitAcceptanceCriteria(desc string) (string, []string) {
	idx := strings.Index(desc, acceptanceHeader)
	if idx < 0 {
		return strings.TrimSpace(desc), nil
	}
	context := strings.TrimSpace(desc[:idx])
	var criteria []string
	for _, line := range strings.Split(desc[idx+len(acceptanceHeader):], "\n") {
		line = strings.TrimSpace(line)
		for _, p := range []string{"- [ ] ", "- [x] ", "- [X] ", "- ", "* "} {
			if strings.HasPrefix(line, p) {
				line = strings.TrimSpace(line[len(p):])
				break
			}
		}
		if line != "" {
			criteria = append(criteria, line)
		}
	}
	return context, criteria
}

// buildTaskBody reassembles a task card body from its context and acceptance
// criteria, mirroring how createTask composes a new card's description.
func buildTaskBody(context string, criteria []string) string {
	body := strings.TrimSpace(context)
	if len(criteria) > 0 {
		if body != "" {
			body += "\n\n"
		}
		body += acceptanceHeader + "\n"
		for _, c := range criteria {
			body += "- [ ] " + c + "\n"
		}
	}
	return strings.TrimRight(body, "\n")
}

// matchList finds a list on the board by name (case-insensitive; exact match
// first, then a substring match so "progress" resolves to "In Progress"),
// returning its id and canonical name.
func matchList(lists []trello.List, name string) (id, canonical string, found bool) {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return "", "", false
	}
	for _, l := range lists {
		if strings.ToLower(strings.TrimSpace(l.Name)) == n {
			return l.ID, l.Name, true
		}
	}
	for _, l := range lists {
		if strings.Contains(strings.ToLower(l.Name), n) {
			return l.ID, l.Name, true
		}
	}
	return "", "", false
}

// listNames joins the list names on a board, for a helpful error hint.
func listNames(lists []trello.List) string {
	var names []string
	for _, l := range lists {
		if n := strings.TrimSpace(l.Name); n != "" {
			names = append(names, n)
		}
	}
	return strings.Join(names, ", ")
}

// labelNames joins the names of the non-empty labels on a card.
func labelNames(labels []trello.Label) string {
	var names []string
	for _, l := range labels {
		if n := strings.TrimSpace(l.Name); n != "" {
			names = append(names, n)
		}
	}
	return strings.Join(names, ", ")
}
