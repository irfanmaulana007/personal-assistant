// Package travel implements the Travel Control skill: starting trips, logging
// expenses against them, and summarizing spend by category vs budget.
package travel

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/irfanmaulana007/personal-assistant/server/internal/authctx"
	"github.com/irfanmaulana007/personal-assistant/server/internal/intent"
	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

// Handler manages trips and their expenses.
type Handler struct {
	store    store.Store
	timezone *time.Location
	log      *slog.Logger
}

// New creates a travel handler.
func New(s store.Store, timezone *time.Location, log *slog.Logger) *Handler {
	return &Handler{store: s, timezone: timezone, log: log.With("component", "travel")}
}

func (h *Handler) Name() string { return "travel" }

func (h *Handler) Match(result *intent.ParseResult) bool {
	return result.Capability == intent.CapabilityTravel
}

func (h *Handler) Handle(ctx context.Context, result *intent.ParseResult) (string, error) {
	switch result.Action {
	case intent.ActionTripCreate:
		return h.createTrip(ctx, result)
	case intent.ActionExpenseAdd:
		return h.addExpense(ctx, result)
	case intent.ActionTripSummary:
		return h.summary(ctx, result)
	default:
		return "I can start a trip, log an expense, or summarize a trip.", nil
	}
}

func (h *Handler) createTrip(ctx context.Context, result *intent.ParseResult) (string, error) {
	name := strings.TrimSpace(result.Entities["name"])
	if name == "" {
		return "What should I call this trip?", nil
	}
	destination := strings.TrimSpace(result.Entities["destination"])
	currency := strings.TrimSpace(result.Entities["currency"])
	budget, _ := strconv.ParseFloat(strings.TrimSpace(result.Entities["budget"]), 64)

	t, err := h.store.CreateTrip(ctx, authctx.UserID(ctx), name, destination, currency, budget)
	if err != nil {
		return "", fmt.Errorf("create trip: %w", err)
	}

	msg := fmt.Sprintf("Started trip *%s*", t.Name)
	if t.Destination != "" {
		msg += " to " + t.Destination
	}
	if t.Budget > 0 {
		msg += fmt.Sprintf(" (budget %s)", money(t.Budget, t.Currency))
	}
	msg += ". Log expenses and I'll keep the running total."
	return msg, nil
}

// resolveTrip picks the named trip if given, else the active trip.
func (h *Handler) resolveTrip(ctx context.Context, userID int64, name string) (*store.Trip, error) {
	if name != "" {
		return h.store.FindTrip(ctx, userID, name)
	}
	return h.store.ActiveTrip(ctx, userID)
}

func (h *Handler) addExpense(ctx context.Context, result *intent.ParseResult) (string, error) {
	userID := authctx.UserID(ctx)
	amount, err := strconv.ParseFloat(strings.TrimSpace(result.Entities["amount"]), 64)
	if err != nil || amount <= 0 {
		return "How much was the expense? Please give a numeric amount.", nil
	}

	trip, err := h.resolveTrip(ctx, userID, strings.TrimSpace(result.Entities["trip"]))
	if err != nil {
		return "", fmt.Errorf("resolve trip: %w", err)
	}
	if trip == nil {
		return "Start a trip first, e.g. _start a trip to Bali_.", nil
	}

	category := strings.TrimSpace(result.Entities["category"])
	note := strings.TrimSpace(result.Entities["note"])
	currency := strings.TrimSpace(result.Entities["currency"])
	if currency == "" {
		currency = trip.Currency
	}

	e, err := h.store.AddExpense(ctx, userID, trip.ID, amount, currency, category, note, time.Now())
	if err != nil {
		return "", fmt.Errorf("add expense: %w", err)
	}

	// Running total.
	expenses, _ := h.store.ListTripExpenses(ctx, userID, trip.ID)
	var total float64
	for _, x := range expenses {
		total += x.Amount
	}
	msg := fmt.Sprintf("Logged %s for *%s* on _%s_.", money(e.Amount, currency), trip.Name, e.Category)
	msg += fmt.Sprintf("\nTrip total so far: %s", money(total, trip.Currency))
	if trip.Budget > 0 {
		msg += fmt.Sprintf(" of %s (%s left)", money(trip.Budget, trip.Currency), money(trip.Budget-total, trip.Currency))
	}
	return msg, nil
}

func (h *Handler) summary(ctx context.Context, result *intent.ParseResult) (string, error) {
	userID := authctx.UserID(ctx)
	trip, err := h.resolveTrip(ctx, userID, strings.TrimSpace(result.Entities["trip"]))
	if err != nil {
		return "", fmt.Errorf("resolve trip: %w", err)
	}
	if trip == nil {
		return "You don't have an active trip. Start one with _start a trip to …_.", nil
	}

	expenses, err := h.store.ListTripExpenses(ctx, userID, trip.ID)
	if err != nil {
		return "", fmt.Errorf("list expenses: %w", err)
	}

	byCat := map[string]float64{}
	var total float64
	for _, e := range expenses {
		byCat[e.Category] += e.Amount
		total += e.Amount
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*%s*", trip.Name))
	if trip.Destination != "" {
		sb.WriteString(" — " + trip.Destination)
	}
	sb.WriteString(fmt.Sprintf("\n%d expense(s), total %s", len(expenses), money(total, trip.Currency)))
	if trip.Budget > 0 {
		sb.WriteString(fmt.Sprintf(" / %s budget (%s left)", money(trip.Budget, trip.Currency), money(trip.Budget-total, trip.Currency)))
	}

	if len(byCat) > 0 {
		type kv struct {
			k string
			v float64
		}
		var ranked []kv
		for k, v := range byCat {
			ranked = append(ranked, kv{k, v})
		}
		sort.Slice(ranked, func(i, j int) bool { return ranked[i].v > ranked[j].v })
		sb.WriteString("\n\nBy category:")
		for _, r := range ranked {
			sb.WriteString(fmt.Sprintf("\n• %s: %s", r.k, money(r.v, trip.Currency)))
		}
	}
	return sb.String(), nil
}

func money(amount float64, currency string) string {
	s := strconv.FormatFloat(amount, 'f', -1, 64)
	if currency != "" {
		return currency + " " + s
	}
	return s
}
