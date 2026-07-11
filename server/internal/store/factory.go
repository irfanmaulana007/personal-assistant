package store

import (
	"context"
	"fmt"

	"github.com/irfanmaulana007/personal-assistant/server/internal/config"
)

// Open constructs the application's storage backend: the hybrid store, with
// PostgreSQL for main data and MongoDB for logs. This is the single construction
// point; every caller downstream depends only on the store.Store interface.
//
// SQLite is no longer an application backend — NewSQLite survives solely as the
// read source for the migrate-db ETL and is not reachable from here.
func Open(ctx context.Context, cfg config.DatabaseConfig) (Store, error) {
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
}
