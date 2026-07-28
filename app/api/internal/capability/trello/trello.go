// Package trello implements the Trello skills: one that reviews every task and
// bug across the project's boards, one that files a new card (a task or a bug
// report), one that edits an existing card, and one that captures an enriched
// game idea.
//
// A project can link many Trello workspaces and boards (managed on the
// Integrations → Trello page). The review skill reads across all of the
// project's linked boards; the create/update skills act on one board — the board
// named in the request, the sole linked board when there's only one, or (when
// several are linked and none is named) the model is asked to pick one. Tasks,
// bugs, and ideas are routed to a sensible column by list name (Backlog/Todo,
// Bug, Ideas — see trello.PickList), and task-type labels are resolved by name
// against the board's own labels, so the same skills work on whatever board a
// project points at. Credentials (API key + token) are resolved per call from
// encrypted settings. A missing credential — or a project with no board linked —
// is reported back to the model as plain text so it can tell the user to
// configure it on the Integrations → Trello page.
package trello

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/intent"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/settings"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/store"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/trello"
)

const notConfiguredMsg = "Trello is not configured — no Trello API key/token has been set. Ask the user to add their Trello API key and token on the Integrations page."

const boardNotConfiguredMsg = "This project has no Trello board linked, so I can't read or file cards for it. Ask the user to link a Trello workspace and board on the Integrations → Trello page."

// boardLister supplies the boards linked to the active project. *store.Store
// (the app database) satisfies it; kept as a narrow interface so the handler
// depends only on what it uses.
type boardLister interface {
	ListLinkedTrelloBoards(ctx context.Context) ([]store.TrelloBoardLink, error)
}

// boardRef is a linked Trello board the skills can act on: its Trello board id
// and display name (name may be empty for the legacy settings-based fallback).
type boardRef struct {
	ID   string
	Name string
}

// Handler answers Trello tool calls (review the boards, file a task, report a bug).
type Handler struct {
	client   *trello.Client
	store    boardLister
	settings *settings.Service
	log      *slog.Logger
}

// New creates a Trello capability handler.
func New(client *trello.Client, boards boardLister, settingsSvc *settings.Service, log *slog.Logger) *Handler {
	return &Handler{client: client, store: boards, settings: settingsSvc, log: log.With("component", "trello")}
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

	// Resolve the boards this project has linked. Boards are per-project, so a
	// project with none linked has the skills disabled for it.
	boards, err := h.resolveBoards(ctx)
	if err != nil {
		return "", fmt.Errorf("resolve trello boards: %w", err)
	}
	if len(boards) == 0 {
		return boardNotConfiguredMsg, nil
	}

	// Review reads across every linked board; the rest act on a single board.
	if result.Action == intent.ActionTrelloReview {
		return h.review(ctx, apiKey, token, boards)
	}

	// Pick the target board for a write: the one named in the request, the sole
	// linked board, or (several linked, none named) ask the model to choose.
	board, msg := pickBoard(boards, result.Entities["board"])
	if msg != "" {
		return msg, nil
	}

	switch result.Action {
	case intent.ActionTrelloCreateTask:
		return h.createTask(ctx, apiKey, token, board, result.Entities)
	case intent.ActionTrelloReportBug:
		return h.reportBug(ctx, apiKey, token, board, result.Entities)
	case intent.ActionTrelloUpdateCard:
		return h.updateCard(ctx, apiKey, token, board, result.Entities)
	case intent.ActionTrelloGameIdea:
		return h.createGameIdea(ctx, apiKey, token, board, result.Entities)
	default:
		return "I understood a Trello request but not which action to take.", nil
	}
}

// resolveBoards returns the boards the active project can act on: every board
// linked via the Integrations page. As a fallback (a project that set an Active
// board in settings but predates board-linking, or whose board wasn't backfilled
// because it had no workspace id), it uses that single settings board so the
// skills keep working.
func (h *Handler) resolveBoards(ctx context.Context) ([]boardRef, error) {
	links, err := h.store.ListLinkedTrelloBoards(ctx)
	if err != nil {
		return nil, err
	}
	var boards []boardRef
	for _, l := range links {
		if id := strings.TrimSpace(l.TrelloID); id != "" {
			boards = append(boards, boardRef{ID: id, Name: strings.TrimSpace(l.Name)})
		}
	}
	if len(boards) > 0 {
		return boards, nil
	}
	// Fallback: the legacy single Active board stored in settings.
	_, boardID, err := h.settings.TrelloBoard(ctx)
	if err != nil {
		return nil, err
	}
	if id := strings.TrimSpace(boardID); id != "" {
		boards = append(boards, boardRef{ID: id})
	}
	return boards, nil
}

// pickBoard resolves which single board a write should target. It returns either
// a chosen board (msg == "") or a message to relay to the model (msg != "") when
// it must ask the user which board or report that the named one isn't linked.
func pickBoard(boards []boardRef, query string) (board boardRef, msg string) {
	switch {
	case len(boards) == 0:
		return boardRef{}, boardNotConfiguredMsg
	case strings.TrimSpace(query) != "":
		if b, ok := matchBoard(boards, query); ok {
			return b, ""
		}
		return boardRef{}, fmt.Sprintf("This project has no linked Trello board called %q. Linked boards: %s. Which one should I use?", strings.TrimSpace(query), boardNamesList(boards))
	case len(boards) == 1:
		return boards[0], ""
	default:
		return boardRef{}, fmt.Sprintf("This project has %d Trello boards linked: %s. Which one should I use? Ask the user, then pass the board name.", len(boards), boardNamesList(boards))
	}
}

// matchBoard finds a linked board by name (case-insensitive; exact match first,
// then a substring match), so the model can target a board by the name it saw in
// a review.
func matchBoard(boards []boardRef, query string) (boardRef, bool) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return boardRef{}, false
	}
	for _, b := range boards {
		if strings.ToLower(strings.TrimSpace(b.Name)) == q {
			return b, true
		}
	}
	for _, b := range boards {
		if b.Name != "" && strings.Contains(strings.ToLower(b.Name), q) {
			return b, true
		}
	}
	return boardRef{}, false
}

// boardNamesList joins the linked board names for a helpful prompt, quoting each
// (an unnamed fallback board is shown as "(unnamed board)").
func boardNamesList(boards []boardRef) string {
	var names []string
	for _, b := range boards {
		if n := strings.TrimSpace(b.Name); n != "" {
			names = append(names, fmt.Sprintf("%q", n))
		} else {
			names = append(names, "(unnamed board)")
		}
	}
	return strings.Join(names, ", ")
}

// review lists every card across all of the project's linked boards, grouped by
// board and then by list. A board that can't be read is noted and skipped so one
// unreadable board doesn't sink the whole review.
func (h *Handler) review(ctx context.Context, apiKey, token string, boards []boardRef) (string, error) {
	var b strings.Builder
	multi := len(boards) > 1
	for i, board := range boards {
		if i > 0 {
			b.WriteString("\n")
		}
		h.renderBoard(ctx, &b, apiKey, token, board, multi)
	}
	if multi {
		b.WriteString("\nSummarize this for the user in their language: per board (name each), how many open tasks and bugs, and anything currently in progress. Don't invent cards beyond this list.")
	} else {
		b.WriteString("\nSummarize this for the user in their language: how many open tasks and bugs, and anything currently in progress. Don't invent cards beyond this list.")
	}
	return b.String(), nil
}

// renderBoard appends one board's cards (grouped by list) to b. Read failures are
// written inline as a note rather than returned, so review can continue to the
// next board.
func (h *Handler) renderBoard(ctx context.Context, b *strings.Builder, apiKey, token string, board boardRef, multi bool) {
	heading := strings.TrimSpace(board.Name)
	if heading == "" {
		heading = "Trello board"
	}
	b.WriteString("# " + heading + "\n")

	lists, err := h.client.BoardLists(ctx, apiKey, token, board.ID)
	if err != nil {
		h.log.Warn("trello list lists failed", "board", board.ID, "error", err)
		b.WriteString(fmt.Sprintf("_(couldn't read this board: %v)_\n", err))
		return
	}
	cards, err := h.client.BoardCards(ctx, apiKey, token, board.ID)
	if err != nil {
		h.log.Warn("trello list cards failed", "board", board.ID, "error", err)
		b.WriteString(fmt.Sprintf("_(couldn't read this board: %v)_\n", err))
		return
	}
	byList := map[string][]trello.Card{}
	for _, c := range cards {
		byList[c.IDList] = append(byList[c.IDList], c)
	}
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

// onBoard renders a ` on the "X" board` suffix for confirmations, or "" for an
// unnamed (legacy single) board, so single-board projects still read naturally.
func onBoard(b boardRef) string {
	if n := strings.TrimSpace(b.Name); n != "" {
		return fmt.Sprintf(" on the %q board", n)
	}
	return ""
}

// createTask files a task on the board's Backlog/Todo list, with a chosen label
// and an "Acceptance Criteria" checklist.
func (h *Handler) createTask(ctx context.Context, apiKey, token string, board boardRef, e map[string]string) (string, error) {
	boardID := board.ID
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

	lists, err := h.client.BoardLists(ctx, apiKey, token, boardID)
	if err != nil {
		h.log.Warn("trello list lists failed", "error", err)
		return fmt.Sprintf("Couldn't read the board's lists to add the task: %v", err), nil
	}
	listID, listName := trello.PickList(lists, "backlog", "todo", "to do")
	if listID == "" {
		return "The configured Trello board has no lists to add a task to.", nil
	}

	in := trello.CreateCardInput{ListID: listID, Name: title, Desc: body}
	labelKey := strings.ToLower(strings.TrimSpace(e["label"]))
	labelNote := "no label"
	if labelKey != "" {
		if id, _, err := h.matchBoardLabel(ctx, apiKey, token, boardID, labelKey); err != nil {
			h.log.Warn("trello resolve label failed", "label", labelKey, "error", err)
		} else if id != "" {
			in.LabelIDs = []string{id}
			labelNote = labelKey
		}
	}

	card, err := h.client.CreateCard(ctx, apiKey, token, in)
	if err != nil {
		h.log.Warn("trello create task failed", "error", err)
		return fmt.Sprintf("Couldn't create the task card: %v", err), nil
	}

	// Read-after-write: confirm the card actually persisted — that it exists, sits
	// on the list we filed it to, and isn't archived — before telling the user it
	// was filed.
	if err := h.verifyCard(ctx, apiKey, token, card.ID, listID); err != nil {
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

	return fmt.Sprintf("Added task %q to the %q list%s (label: %s).\n%s\nConfirm this to the user in their language.", title, listName, onBoard(board), labelNote, card.ShortURL), nil
}

// verifyCard is the read-after-write check behind every create and update: it
// re-reads the card straight from Trello and confirms it truly persisted —
// nudging past the mistake where CreateCard returns a card object but nothing
// visible landed for the user. It fails if the card can't be read back, is
// archived, or sits on a different list than the one we filed it to (wantList;
// pass "" to skip the list check).
func (h *Handler) verifyCard(ctx context.Context, apiKey, token, cardID, wantList string) error {
	got, err := h.client.GetCard(ctx, apiKey, token, cardID)
	if err != nil {
		return err
	}
	return checkPersisted(got, wantList)
}

// checkPersisted validates a card read back after a write against what we meant
// to leave on Trello. It is split out from verifyCard so the verification rules
// are unit-testable without a live Trello. A nil card, an archived card, or a
// card on the wrong list all count as "did not persist".
func checkPersisted(got *trello.Card, wantList string) error {
	if got == nil {
		return fmt.Errorf("the card wasn't found after creating it")
	}
	if got.Closed {
		return fmt.Errorf("the card was created but is archived")
	}
	if wantList != "" && got.IDList != wantList {
		return fmt.Errorf("the card was created but landed on a different list")
	}
	return nil
}

// reportBug files a bug on the board's Bug list (falling back to the first list
// when the board has no dedicated Bug column), with Actual/Expected sections.
func (h *Handler) reportBug(ctx context.Context, apiKey, token string, board boardRef, e map[string]string) (string, error) {
	boardID := board.ID
	title := strings.TrimSpace(e["title"])
	if title == "" {
		return "What's the bug? I need a short title to file it on the board.", nil
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

	lists, err := h.client.BoardLists(ctx, apiKey, token, boardID)
	if err != nil {
		h.log.Warn("trello list lists failed", "error", err)
		return fmt.Sprintf("Couldn't read the board's lists to file the bug: %v", err), nil
	}
	listID, listName := trello.PickList(lists, "bug", "bugs", "issue", "issues")
	if listID == "" {
		return "The configured Trello board has no lists to file a bug on.", nil
	}

	card, err := h.client.CreateCard(ctx, apiKey, token, trello.CreateCardInput{ListID: listID, Name: title, Desc: body})
	if err != nil {
		h.log.Warn("trello report bug failed", "error", err)
		return fmt.Sprintf("Couldn't file the bug card: %v", err), nil
	}

	// Read-after-write: confirm the card actually persisted on the Bug list before
	// telling the user it was filed.
	if err := h.verifyCard(ctx, apiKey, token, card.ID, listID); err != nil {
		h.log.Warn("trello verify bug failed", "card", card.ID, "error", err)
		return fmt.Sprintf("I tried to file the bug card but couldn't verify it saved on Trello: %v", err), nil
	}
	return fmt.Sprintf("Filed bug %q on the %q list%s.\n%s\nConfirm this to the user in their language.", title, listName, onBoard(board), card.ShortURL), nil
}

// acceptanceHeader is the Markdown heading under which a task card's acceptance
// criteria live in its description body (matching how createTask writes them).
const acceptanceHeader = "## Acceptance Criteria"

// updateCard edits an existing task card on the project's board — its title,
// description, acceptance criteria, type label, or the list it sits in. The card
// is identified by (part of) its current title, since the review tool surfaces
// titles rather than ids. Only the fields the model actually supplied are
// changed; everything else is left untouched.
func (h *Handler) updateCard(ctx context.Context, apiKey, token string, board boardRef, e map[string]string) (string, error) {
	boardID := board.ID
	query := strings.TrimSpace(e["card"])
	if query == "" {
		return "Which card should I update? Tell me its title (or part of it).", nil
	}

	cards, err := h.client.BoardCards(ctx, apiKey, token, boardID)
	if err != nil {
		h.log.Warn("trello list cards failed", "error", err)
		return fmt.Sprintf("Couldn't read the Trello board to find that card: %v", err), nil
	}
	matches := matchCards(cards, query)
	switch len(matches) {
	case 0:
		return fmt.Sprintf("I couldn't find a card matching %q on the Trello board. Try the exact title from a board review.", query), nil
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

	// Label: set a new type label (resolved by name against the board's own
	// labels), or clear all labels with "none".
	if v, ok := e["label"]; ok {
		key := strings.ToLower(strings.TrimSpace(v))
		switch {
		case key == "" || key == "none" || key == "remove":
			empty := []string{}
			in.LabelIDs = &empty
			changed = append(changed, "label (removed)")
		default:
			id, available, err := h.matchBoardLabel(ctx, apiKey, token, boardID, key)
			if err != nil {
				h.log.Warn("trello resolve label failed", "label", key, "error", err)
				return fmt.Sprintf("Couldn't read the board's labels to set that label: %v", err), nil
			}
			if id == "" {
				hint := strings.Join(available, ", ")
				if hint == "" {
					hint = "(this board has no named labels)"
				}
				return fmt.Sprintf("%q isn't a label on this board. Available labels: %s (or 'none' to clear).", v, hint), nil
			}
			ids := []string{id}
			in.LabelIDs = &ids
			changed = append(changed, "label ("+key+")")
		}
	}

	// Move to a different list on the same board (e.g. Backlog → In Progress).
	if v, ok := e["list"]; ok {
		if name := strings.TrimSpace(v); name != "" {
			lists, err := h.client.BoardLists(ctx, apiKey, token, boardID)
			if err != nil {
				h.log.Warn("trello list lists failed", "error", err)
				return fmt.Sprintf("Couldn't read the board's lists to move the card: %v", err), nil
			}
			listID, listName, found := matchList(lists, name)
			if !found {
				return fmt.Sprintf("There's no %q list on the Trello board. Available lists: %s.", name, listNames(lists)), nil
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

	// Read-after-write: only report success once Trello confirms the change. The
	// card must still be live on its expected list — the one we moved it to, or
	// its current list if this edit didn't move it.
	wantList := card.IDList
	if in.IDList != nil {
		wantList = *in.IDList
	}
	if err := h.verifyCard(ctx, apiKey, token, card.ID, wantList); err != nil {
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
	return fmt.Sprintf("Updated %q (%s)%s.\n%s\nConfirm this to the user in their language.", name, strings.Join(changed, ", "), onBoard(board), shortURL), nil
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

// createGameIdea files an enriched game-idea card on the board's Ideas list
// (falling back to the first list when the board has no Ideas column), composing
// the concept, genre, core mechanics, references, and notes into a single
// well-formed brief.
func (h *Handler) createGameIdea(ctx context.Context, apiKey, token string, board boardRef, e map[string]string) (string, error) {
	boardID := board.ID
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

	lists, err := h.client.BoardLists(ctx, apiKey, token, boardID)
	if err != nil {
		h.log.Warn("trello list lists failed", "error", err)
		return fmt.Sprintf("Couldn't read the board's lists to save the idea: %v", err), nil
	}
	listID, listName := trello.PickList(lists, "ideas", "idea", "game ideas")
	if listID == "" {
		return "The configured Trello board has no lists to save the idea on.", nil
	}

	card, err := h.client.CreateCard(ctx, apiKey, token, trello.CreateCardInput{ListID: listID, Name: title, Desc: body})
	if err != nil {
		h.log.Warn("trello save game idea failed", "error", err)
		return fmt.Sprintf("Couldn't save the game idea card: %v", err), nil
	}

	// Read-after-write: confirm the card actually persisted on the Ideas list
	// before telling the user it was saved.
	if err := h.verifyCard(ctx, apiKey, token, card.ID, listID); err != nil {
		h.log.Warn("trello verify game idea failed", "card", card.ID, "error", err)
		return fmt.Sprintf("I tried to save the game idea but couldn't verify it saved on Trello: %v", err), nil
	}
	return fmt.Sprintf("Saved game idea %q to the %q list%s.\n%s\nConfirm this to the user in their language.", title, listName, onBoard(board), card.ShortURL), nil
}

// matchBoardLabel resolves a label name against the board's own labels,
// returning the matching label id (case-insensitive; "" when none matches) plus
// the names of every named label on the board (for a helpful error hint). This
// replaces fixed label ids so task types work on whatever board a project maps
// to.
func (h *Handler) matchBoardLabel(ctx context.Context, apiKey, token, boardID, name string) (id string, available []string, err error) {
	labels, err := h.client.BoardLabels(ctx, apiKey, token, boardID)
	if err != nil {
		return "", nil, err
	}
	want := strings.ToLower(strings.TrimSpace(name))
	for _, l := range labels {
		nm := strings.TrimSpace(l.Name)
		if nm == "" {
			continue
		}
		available = append(available, nm)
		if id == "" && strings.ToLower(nm) == want {
			id = l.ID
		}
	}
	return id, available, nil
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
