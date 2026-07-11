// Command migrate-db is a one-time ETL that copies every table from the
// single-file SQLite store into the hybrid Postgres+Mongo backend, preserving
// ids so foreign keys stay intact. Main data lands in Postgres, append-only logs
// in Mongo.
//
// Connection details come from a config file (--config, the same config.yaml the
// server uses) or from the individual --sqlite/--postgres-dsn/--mongo-uri/
// --mongo-db flags, which override any config values.
//
// Usage:
//
//	go run -tags sqlite_fts5 ./cmd/migrate-db --config server/config/config.yaml --truncate --verify
//	go run -tags sqlite_fts5 ./cmd/migrate-db \
//	  --sqlite data/assistant.db \
//	  --postgres-dsn "postgres://user:pass@localhost:5432/assistant" \
//	  --mongo-uri "mongodb://localhost:27017" --mongo-db assistant_logs \
//	  --truncate --verify
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"gopkg.in/yaml.v3"

	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

// yamlDatabase mirrors just the database section of config.yaml. We parse it
// directly (rather than via config.Load) so the migration tool doesn't require
// the unrelated owner/web validation to pass.
type yamlDatabase struct {
	Database struct {
		Path        string `yaml:"path"`
		PostgresDSN string `yaml:"postgres_dsn"`
		MongoURI    string `yaml:"mongo_uri"`
		MongoDB     string `yaml:"mongo_db"`
	} `yaml:"database"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "migrate-db: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		configPath  = flag.String("config", "config.yaml", "path to config.yaml (used unless overridden by the individual flags)")
		sqlitePath  = flag.String("sqlite", "", "SQLite database path (overrides config)")
		postgresDSN = flag.String("postgres-dsn", "", "PostgreSQL DSN (overrides config)")
		mongoURI    = flag.String("mongo-uri", "", "MongoDB URI (overrides config)")
		mongoDB     = flag.String("mongo-db", "", "MongoDB database name (overrides config)")
		truncate    = flag.Bool("truncate", false, "clear destination tables/collections before migrating")
		verify      = flag.Bool("verify", false, "fail with a non-zero exit if any source/dest counts differ")
	)
	flag.Parse()

	// Start from the config file when present, then let explicit flags override.
	path, dsn, uri, db := *sqlitePath, *postgresDSN, *mongoURI, *mongoDB
	if data, err := os.ReadFile(*configPath); err == nil {
		var yc yamlDatabase
		if err := yaml.Unmarshal([]byte(os.ExpandEnv(string(data))), &yc); err != nil {
			return fmt.Errorf("parse config %s: %w", *configPath, err)
		}
		if path == "" {
			path = yc.Database.Path
		}
		if dsn == "" {
			dsn = yc.Database.PostgresDSN
		}
		if uri == "" {
			uri = yc.Database.MongoURI
		}
		if db == "" {
			db = yc.Database.MongoDB
		}
	}

	switch {
	case path == "":
		return fmt.Errorf("missing SQLite path (set --sqlite or database.path in config)")
	case dsn == "":
		return fmt.Errorf("missing Postgres DSN (set --postgres-dsn or database.postgres_dsn in config)")
	case uri == "":
		return fmt.Errorf("missing Mongo URI (set --mongo-uri or database.mongo_uri in config)")
	case db == "":
		return fmt.Errorf("missing Mongo DB name (set --mongo-db or database.mongo_db in config)")
	}

	ctx := context.Background()

	src, err := store.NewSQLite(path)
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	defer src.Close()

	pg, err := store.NewPostgres(ctx, dsn)
	if err != nil {
		return fmt.Errorf("open postgres: %w", err)
	}
	mongo, err := store.NewMongo(ctx, uri, db)
	if err != nil {
		_ = pg.Close()
		return fmt.Errorf("open mongo: %w", err)
	}
	dst := store.NewHybrid(pg, mongo)
	defer dst.Close()

	report, err := store.MigrateSQLiteToHybrid(ctx, src, dst, store.MigrateOptions{Truncate: *truncate})
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	printReport(report)

	if *verify {
		if err := report.Verify(); err != nil {
			return fmt.Errorf("verification failed: %w", err)
		}
		fmt.Println("\nverify: OK — all source and destination counts match")
	}
	return nil
}

func printReport(r *store.MigrateReport) {
	tables := make([]string, 0, len(r.Counts))
	for t := range r.Counts {
		tables = append(tables, t)
	}
	sort.Strings(tables)

	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "TABLE\tSOURCE\tDEST\tSTATUS")
	for _, t := range tables {
		c := r.Counts[t]
		status := "ok"
		if c[0] != c[1] {
			status = "MISMATCH"
		}
		fmt.Fprintf(w, "%s\t%d\t%d\t%s\n", t, c[0], c[1], status)
	}
	_ = w.Flush()
}
