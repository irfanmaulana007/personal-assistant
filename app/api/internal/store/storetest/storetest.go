// Package storetest provides a real hybrid store (PostgreSQL + MongoDB) for
// unit tests, backed by throwaway Docker containers via testcontainers.
//
// It replaces the old in-process SQLite test store: since SQLite was dropped as
// an application backend, tests exercise the same PostgreSQL + MongoDB code the
// server runs in production. Docker must be available to run any test that calls
// New; the containers are started once per test binary and reaped automatically
// when the test process exits (via the testcontainers Ryuk reaper). Each New
// call gets its own freshly-migrated, isolated database so tests never see each
// other's data.
package storetest

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // "pgx" database/sql driver, for CREATE DATABASE

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/store"
)

var (
	bootOnce sync.Once
	bootErr  error

	// Kept in package scope so the containers are not garbage-collected for the
	// lifetime of the test binary; they are reaped by Ryuk on process exit.
	pgContainer    *postgres.PostgresContainer
	mongoContainer *mongodb.MongoDBContainer

	adminDSN string // DSN to the container's bootstrap database (for CREATE DATABASE)
	mongoURI string

	dbCounter int64
)

func boot() {
	ctx := context.Background()

	pgC, err := postgres.Run(ctx, "postgres:17",
		postgres.WithDatabase("bootstrap"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(90*time.Second),
		),
	)
	if err != nil {
		bootErr = fmt.Errorf("start postgres: %w", err)
		return
	}
	pgContainer = pgC
	adminDSN, err = pgC.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		bootErr = fmt.Errorf("postgres dsn: %w", err)
		return
	}

	mongoC, err := mongodb.Run(ctx, "mongo:7")
	if err != nil {
		bootErr = fmt.Errorf("start mongo: %w", err)
		return
	}
	mongoContainer = mongoC
	mongoURI, err = mongoC.ConnectionString(ctx)
	if err != nil {
		bootErr = fmt.Errorf("mongo uri: %w", err)
		return
	}
}

// New returns a fresh, isolated hybrid store (Postgres + Mongo) for a test.
// The store is migrated and seeded, and closed automatically when the test
// finishes. Requires Docker; the test fails if the containers cannot start.
func New(t testing.TB) store.Store {
	t.Helper()
	bootOnce.Do(boot)
	if bootErr != nil {
		t.Fatalf("storetest: start containers (is Docker running?): %v", bootErr)
	}

	ctx := context.Background()
	n := atomic.AddInt64(&dbCounter, 1)
	dbName := fmt.Sprintf("test_%d", n)

	// Create a dedicated database on the shared Postgres container so each test
	// starts from a clean, freshly-migrated schema.
	admin, err := sql.Open("pgx", adminDSN)
	if err != nil {
		t.Fatalf("storetest: open admin db: %v", err)
	}
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+dbName); err != nil {
		_ = admin.Close()
		t.Fatalf("storetest: create database %s: %v", dbName, err)
	}
	_ = admin.Close()

	u, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatalf("storetest: parse dsn: %v", err)
	}
	u.Path = "/" + dbName
	dsn := u.String()

	pg, err := store.NewPostgres(ctx, dsn)
	if err != nil {
		t.Fatalf("storetest: NewPostgres: %v", err)
	}
	mongo, err := store.NewMongo(ctx, mongoURI, dbName)
	if err != nil {
		_ = pg.Close()
		t.Fatalf("storetest: NewMongo: %v", err)
	}

	db := store.NewHybrid(pg, mongo)
	t.Cleanup(func() { _ = db.Close() })
	return db
}
