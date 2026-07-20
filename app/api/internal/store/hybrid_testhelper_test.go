//go:build integration

package store

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// newTestHybrid starts throwaway postgres:17 + mongo:7 containers and returns a
// migrated, seeded HybridStore (Postgres data + Mongo logs) — the same backend
// the server runs in production. Used by tests that span both halves (traces,
// usage, self-tuning). Requires Docker.
func newTestHybrid(t *testing.T) *HybridStore {
	t.Helper()
	ctx := context.Background()

	pgC, err := postgres.Run(ctx, "postgres:17",
		postgres.WithDatabase("assistant"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
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

	pg, err := NewPostgres(ctx, dsn)
	if err != nil {
		t.Fatalf("NewPostgres: %v", err)
	}
	mongo, err := NewMongo(ctx, uri, "assistant_logs")
	if err != nil {
		t.Fatalf("NewMongo: %v", err)
	}
	h := NewHybrid(pg, mongo)
	t.Cleanup(func() { _ = h.Close() })
	return h
}
