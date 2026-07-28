package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/authctx"
)

// mongoMessageLog is the BSON shape of a message_log document. It maps to the
// public MessageLog struct.
type mongoMessageLog struct {
	ID        int64     `bson:"id"`
	UserID    int64     `bson:"user_id"`
	ProjectID int64     `bson:"project_id"`
	Platform  string    `bson:"platform"`
	Direction string    `bson:"direction"`
	Sender    string    `bson:"sender"`
	Body      string    `bson:"body"`
	Intent    string    `bson:"intent"`
	Action    string    `bson:"action"`
	// TraceID links an "out" message to the run trace that produced it (0 when
	// absent — incoming messages and pre-existing replies).
	TraceID   int64     `bson:"trace_id,omitempty"`
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
	// Attribute the message to the active project. Callers rarely set ProjectID on
	// the struct, so fall back to the project carried on the context (same pattern
	// as traces). This keeps a user's chat history scoped per project.
	projectID := log.ProjectID
	if projectID == 0 {
		projectID = authctx.ProjectID(ctx)
	}
	doc := mongoMessageLog{
		ID:        id,
		UserID:    log.UserID,
		ProjectID: projectID,
		Platform:  log.Platform,
		Direction: log.Direction,
		Sender:    log.Sender,
		Body:      log.Body,
		Intent:    log.Intent,
		Action:    log.Action,
		TraceID:   log.TraceID,
		CreatedAt: time.Now().UTC(),
	}
	if _, err := m.col(colMessageLog).InsertOne(ctx, doc); err != nil {
		return err
	}
	return nil
}

func (m *MongoStore) GetMessageHistory(ctx context.Context, userID int64, platform string, limit int) ([]MessageLog, error) {
	// Take the most-recent `limit` docs (created_at desc, id desc), then present
	// them oldest-first. Scope to the active project so a user's chat history is
	// split per project; a zero project id (superadmin / unscoped) matches all.
	filter := bson.M{"user_id": userID, "platform": platform}
	if pid := authctx.ProjectID(ctx); pid != 0 {
		filter["project_id"] = pid
	}
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
			ProjectID: d.ProjectID,
			Platform:  d.Platform,
			Direction: d.Direction,
			Sender:    d.Sender,
			Body:      d.Body,
			Intent:    d.Intent,
			Action:    d.Action,
			TraceID:   d.TraceID,
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

// GetActivity returns one of the user's activities by id, or (nil, nil) when no
// document matches. Used to confirm a just-logged activity actually persisted.
func (m *MongoStore) GetActivity(ctx context.Context, userID, id int64) (*Activity, error) {
	var d mongoActivity
	err := m.col(colActivities).FindOne(ctx, bson.M{"id": id, "user_id": userID}).Decode(&d)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get activity: %w", err)
	}
	return &Activity{
		ID:          d.ID,
		Type:        d.Type,
		Description: d.Description,
		OccurredAt:  d.OccurredAt,
		Source:      d.Source,
		CreatedAt:   d.CreatedAt,
	}, nil
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
