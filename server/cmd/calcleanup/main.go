// Command calcleanup is a one-off maintenance tool that removes the duplicate
// Google Calendar events created by the pre-fix reminder reconciler (which
// re-inserted every reminder on each 5-minute cycle because it couldn't parse
// Composio's create/list responses).
//
// It is SAFE by construction:
//   - Dry-run by default; it only deletes when -apply is passed.
//   - It only considers events whose title exactly matches one of the owner's
//     current reminders, so genuine personal calendar events are never touched.
//   - Within each duplicate group (same title + recurrence + time-of-day) it
//     KEEPS one event (the earliest-starting) and deletes the rest.
//   - -max-delete caps how many it will remove in a single run.
//
// Usage:
//
//	go run -tags sqlite_fts5 ./cmd/calcleanup -config server/config/config.yaml            # dry run
//	go run -tags sqlite_fts5 ./cmd/calcleanup -config server/config/config.yaml -apply      # delete
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"

	calendarsvc "github.com/irfanmaulana007/personal-assistant/server/internal/calendar"
	"github.com/irfanmaulana007/personal-assistant/server/internal/composio"
	"github.com/irfanmaulana007/personal-assistant/server/internal/config"
	"github.com/irfanmaulana007/personal-assistant/server/internal/crypto"
	"github.com/irfanmaulana007/personal-assistant/server/internal/settings"
	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

func main() {
	configPath := flag.String("config", "server/config/config.yaml", "path to config file")
	apply := flag.Bool("apply", false, "actually delete duplicates (default: dry run)")
	windowDays := flag.Int("window-days", 400, "how far forward to scan for events")
	backDays := flag.Int("back-days", 400, "how far back to scan for events")
	maxDelete := flag.Int("max-delete", 5000, "safety cap on number of deletions in one run")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	db, err := store.NewSQLite(cfg.Database.Path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open store: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	ctx := context.Background()
	tz := cfg.Owner.Location()
	encKey, err := crypto.DecodeKey(cfg.Security.EncryptionKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid encryption key: %v\n", err)
		os.Exit(1)
	}
	settingsSvc := settings.New(db, encKey)
	calSvc := calendarsvc.New(composio.NewClient(), settingsSvc, tz, log)

	owner, err := db.FirstAdmin(ctx)
	if err != nil || owner == nil {
		fmt.Fprintf(os.Stderr, "resolve owner: %v (owner=%v)\n", err, owner)
		os.Exit(1)
	}

	// Allowlist of titles: only events whose title matches a current reminder are
	// eligible for pruning. Everything else on the calendar is left untouched.
	reminders, err := db.ListAllForOwner(ctx, owner.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "list reminders: %v\n", err)
		os.Exit(1)
	}
	allow := map[string]bool{}
	for _, r := range reminders {
		if t := normTitle(reminderBody(r)); t != "" {
			allow[t] = true
		}
	}
	if len(allow) == 0 {
		fmt.Println("No reminders found — nothing is eligible for cleanup. Exiting.")
		return
	}

	now := time.Now().In(tz)
	from := now.AddDate(0, 0, -*backDays)
	to := now.AddDate(0, 0, *windowDays)

	fmt.Printf("Scanning calendar events %s … %s for owner #%d (%d reminder titles eligible)\n\n",
		from.Format("2006-01-02"), to.Format("2006-01-02"), owner.ID, len(allow))

	masters, err := calSvc.ListMasters(ctx, owner.ID, from, to)
	if err != nil {
		fmt.Fprintf(os.Stderr, "list calendar events: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Fetched %d calendar events (recurring masters unexpanded).\n", len(masters))

	// Group eligible events by title + recurrence + time-of-day. Each group is a
	// set of duplicates; keep the earliest, delete the rest.
	type key struct{ title, rec, hm string }
	groups := map[key][]calendarsvc.MasterEvent{}
	for _, m := range masters {
		t := normTitle(m.Title)
		if !allow[t] {
			continue
		}
		k := key{t, m.Recurrence, m.Start.In(tz).Format("15:04")}
		groups[k] = append(groups[k], m)
	}

	var toDelete []calendarsvc.MasterEvent
	var groupKeys []key
	for k := range groups {
		groupKeys = append(groupKeys, k)
	}
	sort.Slice(groupKeys, func(i, j int) bool {
		if groupKeys[i].title != groupKeys[j].title {
			return groupKeys[i].title < groupKeys[j].title
		}
		return groupKeys[i].hm < groupKeys[j].hm
	})

	for _, k := range groupKeys {
		g := groups[k]
		if len(g) <= 1 {
			continue // no duplicates
		}
		// Keep the earliest-starting event; delete the rest.
		sort.Slice(g, func(i, j int) bool { return g[i].Start.Before(g[j].Start) })
		keep := g[0]
		dup := g[1:]
		toDelete = append(toDelete, dup...)
		fmt.Printf("• %-30s %s [%s]  %d copies → keep 1 (%s), delete %d\n",
			truncate(keep.Title, 30), k.hm, recLabel(k.rec), len(g),
			keep.Start.In(tz).Format("2006-01-02"), len(dup))
	}

	fmt.Printf("\nTotal duplicate events to delete: %d\n", len(toDelete))
	if len(toDelete) == 0 {
		fmt.Println("Nothing to clean up. ✅")
		return
	}
	if !*apply {
		fmt.Println("\nDRY RUN — no events were deleted. Re-run with -apply to delete them.")
		return
	}
	if len(toDelete) > *maxDelete {
		fmt.Printf("\nRefusing to delete %d events (over -max-delete=%d). Raise the cap to proceed.\n", len(toDelete), *maxDelete)
		os.Exit(1)
	}

	fmt.Printf("\nDeleting %d duplicate events…\n", len(toDelete))
	deleted, failed := 0, 0
	for i, m := range toDelete {
		if err := calSvc.DeleteEvent(ctx, owner.ID, m.Account, m.ID); err != nil {
			failed++
			log.Warn("delete failed", "id", m.ID, "title", m.Title, "error", err)
			continue
		}
		deleted++
		if (i+1)%50 == 0 {
			fmt.Printf("  … %d/%d\n", i+1, len(toDelete))
		}
	}
	fmt.Printf("\nDone. Deleted %d, failed %d.\n", deleted, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

// reminderBody mirrors the reminder package: the title, or the legacy message.
func reminderBody(r store.Reminder) string {
	if strings.TrimSpace(r.Title) != "" {
		return r.Title
	}
	return r.Message
}

func normTitle(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func recLabel(rec string) string {
	if rec == "" {
		return "one-off"
	}
	return "recurring"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
