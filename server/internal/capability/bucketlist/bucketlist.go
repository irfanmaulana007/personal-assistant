// Package bucketlist implements the Bucket List skill: a categorized checklist
// of things the user wants to do in life ("Take a swimming course", "Visit
// Japan"), any of which can be flagged as a resolution for a given year.
package bucketlist

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/irfanmaulana007/personal-assistant/server/internal/authctx"
	"github.com/irfanmaulana007/personal-assistant/server/internal/intent"
	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

// Handler creates, lists, checks off, and deletes the user's bucket-list items.
type Handler struct {
	store store.Store
	log   *slog.Logger
}

// New creates a bucket-list handler.
func New(s store.Store, log *slog.Logger) *Handler {
	return &Handler{store: s, log: log.With("component", "bucketlist")}
}

func (h *Handler) Name() string { return "bucket_list" }

func (h *Handler) Match(result *intent.ParseResult) bool {
	return result.Capability == intent.CapabilityBucketList
}

func (h *Handler) Handle(ctx context.Context, result *intent.ParseResult) (string, error) {
	switch result.Action {
	case intent.ActionBucketListAdd:
		return h.add(ctx, result)
	case intent.ActionBucketListList:
		return h.list(ctx, result)
	case intent.ActionBucketListCheck:
		return h.check(ctx, result)
	case intent.ActionBucketListDelete:
		return h.remove(ctx, result)
	default:
		return "I can add a bucket-list item, list your bucket list, check one off, or delete one.", nil
	}
}

func (h *Handler) add(ctx context.Context, result *intent.ParseResult) (string, error) {
	title := strings.TrimSpace(result.Entities["title"])
	if title == "" {
		return "What would you like to add to your bucket list?", nil
	}
	description := strings.TrimSpace(result.Entities["description"])
	note := strings.TrimSpace(result.Entities["note"])
	category := store.NormalizeCategory(result.Entities["category"])
	g, err := h.store.CreateBucketItem(ctx, authctx.UserID(ctx), title, description, note, category, nil)
	if err != nil {
		return "", fmt.Errorf("create bucket item: %w", err)
	}
	msg := fmt.Sprintf("Added to your bucket list: *%s* ☐", g.Title)
	if g.Description != "" {
		msg += "\n" + g.Description
	}
	if g.Note != "" {
		msg += "\n" + g.Note
	}
	return msg, nil
}

func (h *Handler) list(ctx context.Context, _ *intent.ParseResult) (string, error) {
	items, err := h.store.ListBucketItems(ctx, authctx.UserID(ctx))
	if err != nil {
		return "", fmt.Errorf("list bucket items: %w", err)
	}
	if len(items) == 0 {
		return "Your bucket list is empty. Tell me something you want to do, like \"add visit Japan to my bucket list\".", nil
	}
	done := 0
	for _, g := range items {
		if g.Done {
			done++
		}
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*Your bucket list* — %d of %d done:\n", done, len(items)))
	for i, g := range items {
		box := "☐"
		if g.Done {
			box = "☑"
		}
		sb.WriteString(fmt.Sprintf("\n%d. %s %s", i+1, box, g.Title))
		if g.Description != "" {
			sb.WriteString("\n   " + g.Description)
		}
		if g.Note != "" {
			sb.WriteString("\n   " + g.Note)
		}
	}
	return sb.String(), nil
}

func (h *Handler) check(ctx context.Context, result *intent.ParseResult) (string, error) {
	g, err := h.find(ctx, result.Entities["item"])
	if err != nil {
		return "", err
	}
	if g == nil {
		return "I couldn't find that on your bucket list. Try \"list my bucket list\" to see it.", nil
	}
	if g.Done {
		return fmt.Sprintf("*%s* is already checked off. 🎉", g.Title), nil
	}
	if err := h.store.SetBucketItemDone(ctx, authctx.UserID(ctx), g.ID, true, nil); err != nil {
		return "", fmt.Errorf("check bucket item: %w", err)
	}
	return fmt.Sprintf("Checked off *%s* ☑ — nice one! 🎉", g.Title), nil
}

func (h *Handler) remove(ctx context.Context, result *intent.ParseResult) (string, error) {
	g, err := h.find(ctx, result.Entities["item"])
	if err != nil {
		return "", err
	}
	if g == nil {
		return "I couldn't find that on your bucket list.", nil
	}
	if err := h.store.DeleteBucketItem(ctx, authctx.UserID(ctx), g.ID); err != nil {
		return "", fmt.Errorf("delete bucket item: %w", err)
	}
	return fmt.Sprintf("Removed *%s* from your bucket list.", g.Title), nil
}

// find resolves an item reference — either a 1-based position from the last
// listing, a database id, or a case-insensitive title match.
func (h *Handler) find(ctx context.Context, ref string) (*store.BucketItem, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, nil
	}
	items, err := h.store.ListBucketItems(ctx, authctx.UserID(ctx))
	if err != nil {
		return nil, fmt.Errorf("list bucket items: %w", err)
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
	// Title match: prefer an exact (case-insensitive) hit, else a substring.
	lower := strings.ToLower(ref)
	for i := range items {
		if strings.EqualFold(items[i].Title, ref) {
			return &items[i], nil
		}
	}
	for i := range items {
		if strings.Contains(strings.ToLower(items[i].Title), lower) {
			return &items[i], nil
		}
	}
	return nil, nil
}
