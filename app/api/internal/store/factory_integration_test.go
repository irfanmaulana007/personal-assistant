//go:build integration

// Verifies the deployment path the docker-compose stack relies on: a hybrid
// DatabaseConfig routed through store.Open must build a working HybridStore
// against real postgres:17 + mongo:7. Requires Docker.
//
//	go test -tags integration ./internal/store/ -run OpenHybrid
package store

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/config"
)

func TestOpenHybrid(t *testing.T) {
	ctx := context.Background()

	pgC, err := postgres.Run(ctx, "postgres:17",
		postgres.WithDatabase("assistant"),
		postgres.WithUsername("assistant"),
		postgres.WithPassword("secret"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(90*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = pgC.Terminate(ctx) })
	dsn, err := pgC.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("pg dsn: %v", err)
	}

	mongoC, err := mongodb.Run(ctx, "mongo:7")
	if err != nil {
		t.Fatalf("start mongo: %v", err)
	}
	t.Cleanup(func() { _ = mongoC.Terminate(ctx) })
	uri, err := mongoC.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("mongo uri: %v", err)
	}

	// The exact path main.go takes: config -> store.Open -> Store.
	db, err := Open(ctx, config.DatabaseConfig{
		PostgresDSN: dsn,
		MongoURI:    uri,
		MongoDB:     "assistant_logs",
	})
	if err != nil {
		t.Fatalf("store.Open(hybrid): %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, ok := db.(*HybridStore); !ok {
		t.Fatalf("expected *HybridStore, got %T", db)
	}

	// Smoke test: a data write (Postgres) and a log write (Mongo) both work
	// through the composed Store, and the cross-backend rollup runs.
	u, err := db.CreateUser(ctx, "deploy@example.com", "hash", "member")
	if err != nil {
		t.Fatalf("create user (postgres half): %v", err)
	}
	if _, err := db.CreateTrace(ctx, &Trace{UserID: u.ID, Platform: "web", TotalTokens: 10, Status: "ok"}); err != nil {
		t.Fatalf("create trace (mongo half): %v", err)
	}
	act, err := db.GetUserActivity(ctx, u.ID)
	if err != nil || act.Runs != 1 {
		t.Fatalf("cross-backend rollup failed: %+v (%v)", act, err)
	}
}
