package store

import (
	"context"
	"fmt"

	"github.com/irfanmaulana007/personal-assistant/server/internal/config"
)

// Open constructs the Store backend selected by cfg.Driver. This is the single
// place that decides which persistence implementation the rest of the app runs
// against; every caller downstream depends only on the store.Store interface,
// so switching backends is a config change, not a code change.
//
//   - "sqlite" (default): the single-file SQLiteStore.
//   - "hybrid": PostgreSQL for main data + MongoDB for logs, composed into a
//     HybridStore. Wired up in a later change; until then it returns an error.
func Open(ctx context.Context, cfg config.DatabaseConfig) (Store, error) {
	switch cfg.Driver {
	case config.DriverSQLite, "":
		return NewSQLite(cfg.Path)
	case config.DriverHybrid:
		pg, err := NewPostgres(ctx, cfg.PostgresDSN)
		if err != nil {
			return nil, fmt.Errorf("postgres: %w", err)
		}
		mongo, err := NewMongo(ctx, cfg.MongoURI, cfg.MongoDB)
		if err != nil {
			_ = pg.Close()
			return nil, fmt.Errorf("mongo: %w", err)
		}
		return NewHybrid(pg, mongo), nil
	default:
		return nil, fmt.Errorf("unknown database driver %q", cfg.Driver)
	}
}
