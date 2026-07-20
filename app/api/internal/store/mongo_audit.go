package store

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// mongoAuditEvent is the BSON shape of an audit_log document. It maps to the
// public AuditEvent struct.
type mongoAuditEvent struct {
	ID          int64     `bson:"id"`
	ProjectID   int64     `bson:"project_id"`
	ActorUserID int64     `bson:"actor_user_id"`
	ActorEmail  string    `bson:"actor_email"`
	Action      string    `bson:"action"`
	Target      string    `bson:"target"`
	Metadata    string    `bson:"metadata"`
	CreatedAt   time.Time `bson:"created_at"`
}

// RecordAudit appends a project-level action to the audit log.
func (m *MongoStore) RecordAudit(ctx context.Context, e *AuditEvent) error {
	id, err := m.nextSeq(ctx, colAudit)
	if err != nil {
		return err
	}
	doc := mongoAuditEvent{
		ID:          id,
		ProjectID:   e.ProjectID,
		ActorUserID: e.ActorUserID,
		ActorEmail:  e.ActorEmail,
		Action:      e.Action,
		Target:      e.Target,
		Metadata:    e.Metadata,
		CreatedAt:   time.Now().UTC(),
	}
	if _, err := m.col(colAudit).InsertOne(ctx, doc); err != nil {
		return fmt.Errorf("record audit: %w", err)
	}
	e.ID = id
	e.CreatedAt = doc.CreatedAt
	return nil
}

// ListAuditEvents returns the most-recent audit events for a project,
// newest-first.
func (m *MongoStore) ListAuditEvents(ctx context.Context, projectID int64, limit int) ([]AuditEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}, {Key: "id", Value: -1}}).
		SetLimit(int64(limit))
	cur, err := m.col(colAudit).Find(ctx, bson.M{"project_id": projectID}, opts)
	if err != nil {
		return nil, fmt.Errorf("list audit events: %w", err)
	}
	defer cur.Close(ctx)

	var docs []mongoAuditEvent
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("scan audit events: %w", err)
	}
	out := make([]AuditEvent, 0, len(docs))
	for _, d := range docs {
		out = append(out, AuditEvent{
			ID:          d.ID,
			ProjectID:   d.ProjectID,
			ActorUserID: d.ActorUserID,
			ActorEmail:  d.ActorEmail,
			Action:      d.Action,
			Target:      d.Target,
			Metadata:    d.Metadata,
			CreatedAt:   d.CreatedAt,
		})
	}
	return out, nil
}
