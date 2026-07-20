package store

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoStore is the LogStore half of the hybrid backend, backed by MongoDB. It
// owns the append-only logs and the usage analytics computed over them; the main
// data lives in PostgresStore. On its own it satisfies LogStore; composed with a
// PostgresStore into a HybridStore it helps satisfy the full Store interface.
//
// Collection layout:
//   - message_log, tool_usage, activities: one document per event.
//   - traces: one document per agent run, with the LLM-judge verdict embedded as
//     a `score` sub-document (nil until judged) — this removes the SQLite
//     traces⟕trace_scores join.
//   - counters: {_id: <collection>, seq: <int64>} backing monotonic int64 ids so
//     the domain structs keep their int64 ID contract (Mongo's native _id is an
//     ObjectID, which the API's id-based trace cursor can't use).
var _ LogStore = (*MongoStore)(nil)

const (
	colMessageLog = "message_log"
	colTraces     = "traces"
	colToolUsage  = "tool_usage"
	colActivities = "activities"
	colAudit      = "audit_log"
	colCounters   = "counters"
)

type MongoStore struct {
	client *mongo.Client
	db     *mongo.Database
}

// NewMongo connects to MongoDB, verifies the connection, ensures the
// collections' indexes exist, and runs any pending data migrations (all
// idempotent).
func NewMongo(ctx context.Context, uri, dbName string) (*MongoStore, error) {
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("connect mongo: %w", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(ctx)
		return nil, fmt.Errorf("ping mongo: %w", err)
	}

	m := &MongoStore{client: client, db: client.Database(dbName)}
	if err := m.ensureIndexes(ctx); err != nil {
		_ = client.Disconnect(ctx)
		return nil, fmt.Errorf("ensure indexes: %w", err)
	}
	if err := m.runMigrations(ctx); err != nil {
		_ = client.Disconnect(ctx)
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	return m, nil
}

func (m *MongoStore) col(name string) *mongo.Collection { return m.db.Collection(name) }

// nextSeq returns the next monotonic int64 id for a collection, using an atomic
// findOneAndUpdate($inc) on the counters collection.
func (m *MongoStore) nextSeq(ctx context.Context, name string) (int64, error) {
	res := m.col(colCounters).FindOneAndUpdate(
		ctx,
		bson.M{"_id": name},
		bson.M{"$inc": bson.M{"seq": int64(1)}},
		options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After),
	)
	var doc struct {
		Seq int64 `bson:"seq"`
	}
	if err := res.Decode(&doc); err != nil {
		return 0, fmt.Errorf("next seq %s: %w", name, err)
	}
	return doc.Seq, nil
}

// ensureIndexes creates the query indexes used by the log collections. Creating
// an existing index is a no-op, so this is safe to run on every startup.
func (m *MongoStore) ensureIndexes(ctx context.Context) error {
	specs := map[string][]mongo.IndexModel{
		colTraces: {
			{Keys: bson.D{{Key: "created_at", Value: 1}}},
			{Keys: bson.D{{Key: "id", Value: -1}}},
			{Keys: bson.D{{Key: "user_id", Value: 1}}},
			{Keys: bson.D{{Key: "platform", Value: 1}, {Key: "created_at", Value: 1}}},
			{Keys: bson.D{{Key: "score.overall", Value: 1}}},
		},
		colToolUsage: {
			{Keys: bson.D{{Key: "created_at", Value: 1}}},
			{Keys: bson.D{{Key: "tool", Value: 1}}},
		},
		colMessageLog: {
			{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "platform", Value: 1}, {Key: "created_at", Value: -1}}},
		},
		colActivities: {
			{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "occurred_at", Value: 1}}},
		},
		colAudit: {
			{Keys: bson.D{{Key: "project_id", Value: 1}, {Key: "created_at", Value: -1}}},
		},
	}
	for coll, models := range specs {
		if _, err := m.col(coll).Indexes().CreateMany(ctx, models); err != nil {
			return fmt.Errorf("create indexes on %s: %w", coll, err)
		}
	}
	return nil
}

func (m *MongoStore) Close() error {
	return m.client.Disconnect(context.Background())
}
