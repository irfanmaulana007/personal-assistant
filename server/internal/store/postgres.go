package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver "pgx", used only to run migrations
)

//go:embed migrations/postgres/*.sql
var postgresMigrations embed.FS

// PostgresStore is the DataStore half of the hybrid backend, backed by
// PostgreSQL. It owns the operational/main data; logs live in MongoStore. It is
// composed with a MongoStore into a HybridStore to satisfy the full Store
// interface — on its own it satisfies DataStore.
var _ DataStore = (*PostgresStore)(nil)

type PostgresStore struct {
	pool       *pgxpool.Pool
	translator Translator
}

// NewPostgres runs the embedded schema migrations, then opens a connection pool.
func NewPostgres(ctx context.Context, dsn string) (*PostgresStore, error) {
	if err := runPostgresMigrations(dsn); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	s := &PostgresStore{pool: pool}
	if err := s.seedSkills(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("seed skills: %w", err)
	}

	return s, nil
}

// runPostgresMigrations applies all up-migrations embedded under
// migrations/postgres using golang-migrate. It is idempotent: an already
// up-to-date database is a no-op.
func runPostgresMigrations(dsn string) error {
	src, err := iofs.New(postgresMigrations, "migrations/postgres")
	if err != nil {
		return fmt.Errorf("open migration source: %w", err)
	}
	defer src.Close()

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open migration db: %w", err)
	}
	defer db.Close()

	driver, err := migratepgx.WithInstance(db, &migratepgx.Config{})
	if err != nil {
		return fmt.Errorf("migration driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "pgx5", driver)
	if err != nil {
		return fmt.Errorf("migrate instance: %w", err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

// SetTranslator injects the optional English-normalization translator, applied
// to reminder/bucket-list text before it is persisted.
func (s *PostgresStore) SetTranslator(t Translator) {
	s.translator = t
}

// enTitle normalizes a title/name to English, or returns it unchanged when no
// translator is configured.
func (s *PostgresStore) enTitle(ctx context.Context, text string) string {
	if s.translator == nil {
		return text
	}
	return s.translator.Title(ctx, text)
}

// enText normalizes free-form text to English, or returns it unchanged when no
// translator is configured.
func (s *PostgresStore) enText(ctx context.Context, text string) string {
	if s.translator == nil {
		return text
	}
	return s.translator.Text(ctx, text)
}

func (s *PostgresStore) Close() error {
	s.pool.Close()
	return nil
}
