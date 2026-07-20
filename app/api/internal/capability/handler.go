package capability

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/intent"
)

// Handler defines the interface for capability handlers.
type Handler interface {
	// Name returns the capability name.
	Name() string

	// Match returns true if this handler can process the parsed intent.
	Match(result *intent.ParseResult) bool

	// Handle processes the intent and returns a response string.
	Handle(ctx context.Context, result *intent.ParseResult) (string, error)
}

// Router routes parsed intents to the appropriate handler.
type Router struct {
	handlers []Handler
	log      *slog.Logger
}

// NewRouter creates a new capability router.
func NewRouter(log *slog.Logger, handlers ...Handler) *Router {
	return &Router{
		handlers: handlers,
		log:      log.With("component", "router"),
	}
}

// Route finds and executes the matching handler for a parsed intent.
func (r *Router) Route(ctx context.Context, result *intent.ParseResult) string {
	if result.Capability == intent.CapabilityHelp {
		return r.helpMessage()
	}

	if result.Capability == intent.CapabilityUnknown || result.Confidence < 0.5 {
		return "I'm not sure what you mean. Type *help* to see what I can do."
	}

	for _, h := range r.handlers {
		if h.Match(result) {
			response, err := h.Handle(ctx, result)
			if err != nil {
				r.log.Error("handler error",
					"handler", h.Name(),
					"action", result.Action,
					"error", err,
				)
				return fmt.Sprintf("Sorry, something went wrong: %s", err.Error())
			}
			return response
		}
	}

	return "I understood your request but don't have that capability yet. Type *help* to see what I can do."
}

func (r *Router) helpMessage() string {
	var sb strings.Builder
	sb.WriteString("*Personal Assistant* — Here's what I can do:\n\n")

	sb.WriteString("*Calendar*\n")
	sb.WriteString("• _show my calendar_ — view today's events\n")
	sb.WriteString("• _schedule meeting Team Sync at 3pm_ — create event\n")
	sb.WriteString("• _reschedule meeting X to 4pm_ — update event\n")
	sb.WriteString("• _cancel meeting X_ — delete event\n\n")

	sb.WriteString("*Email*\n")
	sb.WriteString("• _check my email_ — view unread inbox\n")
	sb.WriteString("• _read email 1_ — read a specific email\n")
	sb.WriteString("• _search email about project_ — search emails\n")
	sb.WriteString("• _draft reply to John: sounds good_ — create draft\n\n")

	sb.WriteString("*Reminders*\n")
	sb.WriteString("• _remind me to call mom at 5pm_ — set reminder\n")
	sb.WriteString("• _list reminders_ — view active reminders\n")
	sb.WriteString("• _cancel reminder 1_ — cancel a reminder\n\n")

	sb.WriteString("*Notes*\n")
	sb.WriteString("• _save note Meeting Notes: discussed roadmap_ — save a note\n")
	sb.WriteString("• _search notes about roadmap_ — search notes\n")
	sb.WriteString("• _list notes_ — list all notes\n")
	sb.WriteString("• _delete note 1_ — delete a note\n")

	return sb.String()
}
