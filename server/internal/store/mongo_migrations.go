package store

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// colMigrations tracks which one-off Mongo data migrations have already run — the
// Mongo analogue of golang-migrate's schema_migrations table on Postgres. Each
// document is {_id: <migration name>, applied_at: <time>}.
const colMigrations = "schema_migrations"

// defaultProjectID is the project every pre-multi-project log document is
// attributed to. Multi-project support (commit f37b0ec / the
// 000006_multi_project_rbac Postgres migration) introduced project scoping and
// added a project_id field to the traces and audit_log documents, but left the
// historical documents — and the message_log / tool_usage / activities
// collections, which never carried the field — unscoped. Project 1 is the first
// project, i.e. the "personal" project that effectively existed before tenancy.
const defaultProjectID = int64(1)

// mongoMigration is a single idempotent data/schema step. Steps run in slice
// order and each runs at most once, guarded by a marker in colMigrations.
type mongoMigration struct {
	name string
	run  func(*MongoStore, context.Context) error
}

// mongoMigrationList is the ordered registry of Mongo migrations. Append new
// steps to the end; never renumber or remove one that has shipped.
var mongoMigrationList = []mongoMigration{
	{name: "0001_backfill_project_id", run: (*MongoStore).backfillProjectID},
}

// runMigrations applies every not-yet-applied migration in order, recording each
// success in colMigrations so it never re-runs. It is idempotent and safe to
// call on every startup, mirroring runPostgresMigrations.
func (m *MongoStore) runMigrations(ctx context.Context) error {
	for _, mig := range mongoMigrationList {
		applied, err := m.col(colMigrations).CountDocuments(ctx, bson.M{"_id": mig.name})
		if err != nil {
			return fmt.Errorf("check migration %s: %w", mig.name, err)
		}
		if applied > 0 {
			continue
		}
		if err := mig.run(m, ctx); err != nil {
			return fmt.Errorf("run migration %s: %w", mig.name, err)
		}
		if _, err := m.col(colMigrations).InsertOne(ctx, bson.M{
			"_id":        mig.name,
			"applied_at": time.Now().UTC(),
		}); err != nil {
			return fmt.Errorf("record migration %s: %w", mig.name, err)
		}
	}
	return nil
}

// backfillProjectID stamps project_id = defaultProjectID on every log document
// that predates multi-project support: those missing the field entirely
// (message_log, tool_usage, activities, and any traces/audit_log written before
// the field existed) or carrying the pre-migration sentinel 0. Documents that
// already name a real project are left untouched. This is the Mongo counterpart
// of the project_id backfill performed for the Postgres tables in the
// 000006_multi_project_rbac migration.
func (m *MongoStore) backfillProjectID(ctx context.Context) error {
	// Missing field OR the legacy 0 sentinel. Mongo compares numbers by value, so
	// {project_id: 0} also matches documents that stored it as int32/double 0.
	filter := bson.M{"$or": bson.A{
		bson.M{"project_id": bson.M{"$exists": false}},
		bson.M{"project_id": int64(0)},
	}}
	update := bson.M{"$set": bson.M{"project_id": defaultProjectID}}
	for _, coll := range []string{colMessageLog, colToolUsage, colActivities, colTraces, colAudit} {
		if _, err := m.col(coll).UpdateMany(ctx, filter, update); err != nil {
			return fmt.Errorf("backfill project_id on %s: %w", coll, err)
		}
	}
	return nil
}
