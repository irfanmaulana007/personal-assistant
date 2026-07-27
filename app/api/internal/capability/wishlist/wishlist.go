// Package wishlist implements the Wishlist skill: a buy/shopping list of things
// the user plans to purchase (a shower, a pegboard, a marketplace item), each
// with an estimated price, a target month to buy, a priority, and a reference
// link, so purchases can be grouped and checked out after payroll.
package wishlist

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/authctx"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/intent"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/store"
)

// Handler creates, lists, updates, checks off, and deletes wishlist items.
type Handler struct {
	store store.Store
	log   *slog.Logger
}

// New creates a wishlist handler.
func New(s store.Store, log *slog.Logger) *Handler {
	return &Handler{store: s, log: log.With("component", "wishlist")}
}

func (h *Handler) Name() string { return "wishlist" }

func (h *Handler) Match(result *intent.ParseResult) bool {
	return result.Capability == intent.CapabilityWishlist
}

func (h *Handler) Handle(ctx context.Context, result *intent.ParseResult) (string, error) {
	switch result.Action {
	case intent.ActionWishlistAdd:
		return h.add(ctx, result)
	case intent.ActionWishlistList:
		return h.list(ctx, result)
	case intent.ActionWishlistUpdate:
		return h.update(ctx, result)
	case intent.ActionWishlistCheck:
		return h.check(ctx, result)
	case intent.ActionWishlistDelete:
		return h.remove(ctx, result)
	default:
		return "I can add a wishlist item, list your wishlist, update one, mark one as bought, or delete one.", nil
	}
}

func (h *Handler) add(ctx context.Context, result *intent.ParseResult) (string, error) {
	name := strings.TrimSpace(result.Entities["name"])
	if name == "" {
		return "What would you like to add to your wishlist?", nil
	}
	price := parsePrice(result.Entities["estimated_price"])
	buyMonth := normalizeMonth(result.Entities["buy_month"])
	priority := store.NormalizeWishPriority(result.Entities["priority"])
	link := strings.TrimSpace(result.Entities["link"])
	note := strings.TrimSpace(result.Entities["note"])

	userID := authctx.UserID(ctx)
	w, err := h.store.CreateWishItem(ctx, userID, name, price, buyMonth, priority, link, note)
	if err != nil {
		return "", fmt.Errorf("create wish item: %w", err)
	}

	// Read-after-write: confirm the item actually persisted before telling the
	// user it was added, and build the confirmation from the re-read record.
	w, err = h.store.GetWishItem(ctx, userID, w.ID)
	if err != nil {
		return "", fmt.Errorf("verify wish item saved: %w", err)
	}
	if w == nil {
		return "", fmt.Errorf("verify wish item saved: item not found after create")
	}
	return "Added to your wishlist: " + describe(*w), nil
}

func (h *Handler) list(ctx context.Context, _ *intent.ParseResult) (string, error) {
	items, err := h.store.ListWishItems(ctx, authctx.UserID(ctx))
	if err != nil {
		return "", fmt.Errorf("list wish items: %w", err)
	}
	if len(items) == 0 {
		return "Your wishlist is empty. Tell me something you want to buy, like \"add a pegboard to my wishlist, around 300k, buy in September\".", nil
	}
	done := 0
	var total, pending int64
	for _, w := range items {
		if w.Done {
			done++
		} else {
			pending += w.EstimatedPrice
		}
		total += w.EstimatedPrice
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*Your wishlist* — %d of %d bought", done, len(items)))
	sb.WriteString(fmt.Sprintf("\n_Estimated: %s total, %s still to buy_\n", formatPrice(total), formatPrice(pending)))

	// Group by target month; undated items go under a trailing "No month yet".
	lastGroup := "\x00"
	n := 0
	for _, w := range items {
		group := monthLabel(w.BuyMonth)
		if group != lastGroup {
			sb.WriteString("\n*" + group + "*")
			lastGroup = group
		}
		n++
		box := "☐"
		if w.Done {
			box = "☑"
		}
		sb.WriteString(fmt.Sprintf("\n%d. %s %s", n, box, w.Name))
		var meta []string
		if w.EstimatedPrice > 0 {
			meta = append(meta, formatPrice(w.EstimatedPrice))
		}
		if w.Priority != "" && w.Priority != store.PriorityMedium {
			meta = append(meta, w.Priority+" priority")
		}
		if len(meta) > 0 {
			sb.WriteString(" — " + strings.Join(meta, ", "))
		}
		if w.Link != "" {
			sb.WriteString("\n   " + w.Link)
		}
		if w.Note != "" {
			sb.WriteString("\n   " + w.Note)
		}
	}
	return sb.String(), nil
}

func (h *Handler) update(ctx context.Context, result *intent.ParseResult) (string, error) {
	userID := authctx.UserID(ctx)
	w, err := h.find(ctx, result.Entities["item"])
	if err != nil {
		return "", err
	}
	if w == nil {
		return "I couldn't find that on your wishlist. Try \"list my wishlist\" to see it.", nil
	}
	// Only overwrite fields the model actually provided; keep the rest as-is.
	name := w.Name
	if v, ok := result.Entities["name"]; ok && strings.TrimSpace(v) != "" {
		name = strings.TrimSpace(v)
	}
	price := w.EstimatedPrice
	if v, ok := result.Entities["estimated_price"]; ok && strings.TrimSpace(v) != "" {
		price = parsePrice(v)
	}
	buyMonth := w.BuyMonth
	if v, ok := result.Entities["buy_month"]; ok {
		buyMonth = normalizeMonth(v)
	}
	priority := w.Priority
	if v, ok := result.Entities["priority"]; ok && strings.TrimSpace(v) != "" {
		priority = store.NormalizeWishPriority(v)
	}
	link := w.Link
	if v, ok := result.Entities["link"]; ok {
		link = strings.TrimSpace(v)
	}
	note := w.Note
	if v, ok := result.Entities["note"]; ok {
		note = strings.TrimSpace(v)
	}
	if err := h.store.UpdateWishItem(ctx, userID, w.ID, name, price, buyMonth, priority, link, note); err != nil {
		return "", fmt.Errorf("update wish item: %w", err)
	}
	w, err = h.store.GetWishItem(ctx, userID, w.ID)
	if err != nil || w == nil {
		return "", fmt.Errorf("verify wish item updated")
	}
	return "Updated: " + describe(*w), nil
}

func (h *Handler) check(ctx context.Context, result *intent.ParseResult) (string, error) {
	w, err := h.find(ctx, result.Entities["item"])
	if err != nil {
		return "", err
	}
	if w == nil {
		return "I couldn't find that on your wishlist. Try \"list my wishlist\" to see it.", nil
	}
	if w.Done {
		return fmt.Sprintf("*%s* is already marked as bought. 🛒", w.Name), nil
	}
	if err := h.store.SetWishItemDone(ctx, authctx.UserID(ctx), w.ID, true, nil); err != nil {
		return "", fmt.Errorf("check wish item: %w", err)
	}
	return fmt.Sprintf("Marked *%s* as bought ☑ — enjoy! 🛒", w.Name), nil
}

func (h *Handler) remove(ctx context.Context, result *intent.ParseResult) (string, error) {
	w, err := h.find(ctx, result.Entities["item"])
	if err != nil {
		return "", err
	}
	if w == nil {
		return "I couldn't find that on your wishlist.", nil
	}
	if err := h.store.DeleteWishItem(ctx, authctx.UserID(ctx), w.ID); err != nil {
		return "", fmt.Errorf("delete wish item: %w", err)
	}
	return fmt.Sprintf("Removed *%s* from your wishlist.", w.Name), nil
}

// find resolves an item reference — either a 1-based position from the last
// listing, a database id, or a case-insensitive name match.
func (h *Handler) find(ctx context.Context, ref string) (*store.WishItem, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, nil
	}
	items, err := h.store.ListWishItems(ctx, authctx.UserID(ctx))
	if err != nil {
		return nil, fmt.Errorf("list wish items: %w", err)
	}
	// Numeric ref: try list position first, then fall back to a database id.
	if n, err := strconv.Atoi(ref); err == nil {
		if n >= 1 && n <= len(items) {
			return &items[n-1], nil
		}
		for i := range items {
			if items[i].ID == int64(n) {
				return &items[i], nil
			}
		}
	}
	// Name match: prefer an exact (case-insensitive) hit, else a substring.
	lower := strings.ToLower(ref)
	for i := range items {
		if strings.EqualFold(items[i].Name, ref) {
			return &items[i], nil
		}
	}
	for i := range items {
		if strings.Contains(strings.ToLower(items[i].Name), lower) {
			return &items[i], nil
		}
	}
	return nil, nil
}

// describe renders a one-line confirmation for a single item.
func describe(w store.WishItem) string {
	box := "☐"
	if w.Done {
		box = "☑"
	}
	msg := fmt.Sprintf("*%s* %s", w.Name, box)
	var meta []string
	if w.EstimatedPrice > 0 {
		meta = append(meta, formatPrice(w.EstimatedPrice))
	}
	if w.BuyMonth != "" {
		meta = append(meta, "buy "+monthLabel(w.BuyMonth))
	}
	if w.Priority != "" && w.Priority != store.PriorityMedium {
		meta = append(meta, w.Priority+" priority")
	}
	if len(meta) > 0 {
		msg += " — " + strings.Join(meta, ", ")
	}
	if w.Link != "" {
		msg += "\n" + w.Link
	}
	if w.Note != "" {
		msg += "\n" + w.Note
	}
	return msg
}

// parsePrice extracts a whole-number amount from free-form text, honoring the
// common "k"/"jt"/"m" shorthands (e.g. "300k" → 300000, "1.5jt" → 1500000).
func parsePrice(s string) int64 {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return 0
	}
	mult := 1.0
	switch {
	case strings.HasSuffix(s, "jt"):
		mult, s = 1_000_000, strings.TrimSuffix(s, "jt")
	case strings.HasSuffix(s, "m"):
		mult, s = 1_000_000, strings.TrimSuffix(s, "m")
	case strings.HasSuffix(s, "k"):
		mult, s = 1_000, strings.TrimSuffix(s, "k")
	}
	// Strip currency symbols, spaces, and thousands separators.
	var b strings.Builder
	for _, r := range s {
		if (r >= '0' && r <= '9') || r == '.' {
			b.WriteRune(r)
		}
	}
	f, err := strconv.ParseFloat(b.String(), 64)
	if err != nil {
		return 0
	}
	v := int64(f * mult)
	if v < 0 {
		return 0
	}
	return v
}

// normalizeMonth accepts a "YYYY-MM" value and returns it verbatim, or "" for
// anything that is not a valid month.
func normalizeMonth(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if _, err := time.Parse("2006-01", s); err != nil {
		return ""
	}
	return s
}

// monthLabel renders a "YYYY-MM" value as a human month, e.g. "September 2026".
func monthLabel(s string) string {
	if s == "" {
		return "No month yet"
	}
	t, err := time.Parse("2006-01", s)
	if err != nil {
		return s
	}
	return t.Format("January 2006")
}

// formatPrice renders a whole-number amount with thousands separators, prefixed
// with "Rp" (the user's currency).
func formatPrice(v int64) string {
	s := strconv.FormatInt(v, 10)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, c)
	}
	res := "Rp" + string(out)
	if neg {
		res = "-" + res
	}
	return res
}
