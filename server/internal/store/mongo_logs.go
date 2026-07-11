package store

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// mongoMessageLog is the BSON shape of a message_log document. It maps to the
// public MessageLog struct.
type mongoMessageLog struct {
	ID        int64     `bson:"id"`
	UserID    int64     `bson:"user_id"`
	Platform  string    `bson:"platform"`
	Direction string    `bson:"direction"`
	Sender    string    `bson:"sender"`
	Body      string    `bson:"body"`
	Intent    string    `bson:"intent"`
	Action    string    `bson:"action"`
	CreatedAt time.Time `bson:"created_at"`
}

// mongoActivity is the BSON shape of an activities document. It maps to the
// public Activity struct.
type mongoActivity struct {
	ID          int64     `bson:"id"`
	UserID      int64     `bson:"user_id"`
	Type        string    `bson:"type"`
	Description string    `bson:"description"`
	OccurredAt  time.Time `bson:"occurred_at"`
	Source      string    `bson:"source"`
	CreatedAt   time.Time `bson:"created_at"`
}

// --- Message Log ---

func (m *MongoStore) LogMessage(ctx context.Context, log *MessageLog) error {
	id, err := m.nextSeq(ctx, colMessageLog)
	if err != nil {
		return err
	}
	doc := mongoMessageLog{
		ID:        id,
		UserID:    log.UserID,
		Platform:  log.Platform,
		Direction: log.Direction,
		Sender:    log.Sender,
		Body:      log.Body,
		Intent:    log.Intent,
		Action:    log.Action,
		CreatedAt: time.Now().UTC(),
	}
	if _, err := m.col(colMessageLog).InsertOne(ctx, doc); err != nil {
		return err
	}
	return nil
}

func (m *MongoStore) GetMessageHistory(ctx context.Context, userID int64, platform string, limit int) ([]MessageLog, error) {
	// Take the most-recent `limit` docs (created_at desc, id desc), then present
	// them oldest-first.
	filter := bson.M{"user_id": userID, "platform": platform}
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}, {Key: "id", Value: -1}}).
		SetLimit(int64(limit))
	cur, err := m.col(colMessageLog).Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("get message history: %w", err)
	}
	defer cur.Close(ctx)

	var docs []mongoMessageLog
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("scan message log: %w", err)
	}

	// Reverse into oldest-first (created_at asc, id asc).
	logs := make([]MessageLog, 0, len(docs))
	for i := len(docs) - 1; i >= 0; i-- {
		d := docs[i]
		logs = append(logs, MessageLog{
			ID:        d.ID,
			UserID:    d.UserID,
			Platform:  d.Platform,
			Direction: d.Direction,
			Sender:    d.Sender,
			Body:      d.Body,
			Intent:    d.Intent,
			Action:    d.Action,
			CreatedAt: d.CreatedAt,
		})
	}
	return logs, nil
}

// --- Tool usage ---

func (m *MongoStore) LogToolUsage(ctx context.Context, userID int64, tool, platform string) error {
	doc := bson.M{
		"user_id":    userID,
		"tool":       tool,
		"platform":   platform,
		"created_at": time.Now().UTC(),
	}
	if _, err := m.col(colToolUsage).InsertOne(ctx, doc); err != nil {
		return err
	}
	return nil
}

// --- Activities ---

func (m *MongoStore) CreateActivity(ctx context.Context, userID int64, actType, description string, occurredAt time.Time, source string) (*Activity, error) {
	now := time.Now().UTC()
	if source == "" {
		source = "chat"
	}
	id, err := m.nextSeq(ctx, colActivities)
	if err != nil {
		return nil, fmt.Errorf("insert activity: %w", err)
	}
	doc := mongoActivity{
		ID:          id,
		UserID:      userID,
		Type:        actType,
		Description: description,
		OccurredAt:  occurredAt.UTC(),
		Source:      source,
		CreatedAt:   now,
	}
	if _, err := m.col(colActivities).InsertOne(ctx, doc); err != nil {
		return nil, fmt.Errorf("insert activity: %w", err)
	}
	return &Activity{ID: id, Type: actType, Description: description, OccurredAt: occurredAt, Source: source, CreatedAt: now}, nil
}

// ListActivitiesSince returns the user's activities on or after since, newest first.
func (m *MongoStore) ListActivitiesSince(ctx context.Context, userID int64, since time.Time) ([]Activity, error) {
	filter := bson.M{"user_id": userID, "occurred_at": bson.M{"$gte": since.UTC()}}
	opts := options.Find().SetSort(bson.D{{Key: "occurred_at", Value: -1}})
	cur, err := m.col(colActivities).Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("list activities: %w", err)
	}
	defer cur.Close(ctx)

	var docs []mongoActivity
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("scan activity: %w", err)
	}

	var out []Activity
	for _, d := range docs {
		out = append(out, Activity{
			ID:          d.ID,
			Type:        d.Type,
			Description: d.Description,
			OccurredAt:  d.OccurredAt,
			Source:      d.Source,
			CreatedAt:   d.CreatedAt,
		})
	}
	return out, nil
}
