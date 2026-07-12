package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// mongoScore is the BSON shape of the LLM-as-judge verdict embedded in a trace
// document as its `score` sub-document (nil until judged). It maps to the public
// TraceScore struct. Embedding removes the SQLite traces⟕trace_scores join.
type mongoScore struct {
	TraceID     int64     `bson:"trace_id"`
	Accuracy    int       `bson:"accuracy"`
	Helpfulness int       `bson:"helpfulness"`
	Safety      int       `bson:"safety"`
	Overall     float64   `bson:"overall"`
	Rationale   string    `bson:"rationale"`
	JudgeModel  string    `bson:"judge_model"`
	CreatedAt   time.Time `bson:"created_at"`
}

// mongoTrace is the BSON shape of a traces document — one per agent run. Tools,
// Steps and Skills are stored as native BSON arrays (not JSON strings, which was
// a SQLite limitation), and the judge verdict is embedded as the `score`
// sub-document. It maps to the public Trace struct.
type mongoTrace struct {
	ID               int64            `bson:"id"`
	UserID           int64            `bson:"user_id"`
	Platform         string           `bson:"platform"`
	Source           string           `bson:"source"`
	Input            string           `bson:"input"`
	Output           string           `bson:"output"`
	Model            string           `bson:"model"`
	PromptTokens     int              `bson:"prompt_tokens"`
	CompletionTokens int              `bson:"completion_tokens"`
	TotalTokens      int              `bson:"total_tokens"`
	LatencyMs        int              `bson:"latency_ms"`
	ToolCount        int              `bson:"tool_count"`
	Tools            []ToolInvocation `bson:"tools"`
	Steps            []LLMCall        `bson:"steps"`
	Skills           []string         `bson:"skills"`
	Status           string           `bson:"status"`
	Error            string           `bson:"error"`
	CreatedAt        time.Time        `bson:"created_at"`
	Score            *mongoScore      `bson:"score"`
}

// toTraceScore maps an embedded score sub-document to the public TraceScore.
func (s *mongoScore) toTraceScore() *TraceScore {
	if s == nil {
		return nil
	}
	return &TraceScore{
		TraceID:     s.TraceID,
		Accuracy:    s.Accuracy,
		Helpfulness: s.Helpfulness,
		Safety:      s.Safety,
		Overall:     s.Overall,
		Rationale:   s.Rationale,
		JudgeModel:  s.JudgeModel,
		CreatedAt:   s.CreatedAt,
	}
}

// --- Traces ---

// CreateTrace inserts a new trace and returns its monotonic int64 id. The score
// sub-document is null until the trace is judged.
func (m *MongoStore) CreateTrace(ctx context.Context, t *Trace) (int64, error) {
	id, err := m.nextSeq(ctx, colTraces)
	if err != nil {
		return 0, fmt.Errorf("insert trace: %w", err)
	}
	status := t.Status
	if status == "" {
		status = "ok"
	}
	source := t.Source
	if source == "" {
		source = "chat"
	}
	doc := mongoTrace{
		ID:               id,
		UserID:           t.UserID,
		Platform:         t.Platform,
		Source:           source,
		Input:            t.Input,
		Output:           t.Output,
		Model:            t.Model,
		PromptTokens:     t.PromptTokens,
		CompletionTokens: t.CompletionTokens,
		TotalTokens:      t.TotalTokens,
		LatencyMs:        t.LatencyMs,
		ToolCount:        t.ToolCount,
		Tools:            t.Tools,
		Steps:            t.Steps,
		Skills:           t.Skills,
		Status:           status,
		Error:            t.Error,
		CreatedAt:        time.Now().UTC(),
		Score:            nil,
	}
	if _, err := m.col(colTraces).InsertOne(ctx, doc); err != nil {
		return 0, fmt.Errorf("insert trace: %w", err)
	}
	return id, nil
}

// GetTrace returns the full trace by id — including Tools, Steps, Skills and the
// embedded Score — or (nil, nil) if no such trace exists.
func (m *MongoStore) GetTrace(ctx context.Context, id int64) (*Trace, error) {
	var doc mongoTrace
	err := m.col(colTraces).FindOne(ctx, bson.M{"id": id}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get trace: %w", err)
	}
	t := Trace{
		ID:               doc.ID,
		UserID:           doc.UserID,
		Platform:         doc.Platform,
		Source:           doc.Source,
		Input:            doc.Input,
		Output:           doc.Output,
		Model:            doc.Model,
		PromptTokens:     doc.PromptTokens,
		CompletionTokens: doc.CompletionTokens,
		TotalTokens:      doc.TotalTokens,
		LatencyMs:        doc.LatencyMs,
		ToolCount:        doc.ToolCount,
		Tools:            doc.Tools,
		Steps:            doc.Steps,
		Skills:           doc.Skills,
		Status:           doc.Status,
		Error:            doc.Error,
		CreatedAt:        doc.CreatedAt,
		Score:            doc.Score.toTraceScore(),
	}
	return &t, nil
}

// ListTraces returns traces matching the filter, ordered by id descending with
// cursor pagination. Like the SQLite version, the list view does not populate
// Tools/Steps/Skills — only the scalar fields and the embedded Score.
// mongoScoreClauses maps score-state filter values to their per-state match
// documents. Unknown values are ignored. The returned slice is OR-combined by
// the caller ("" / empty input yields no clauses, i.e. "all").
func mongoScoreClauses(states []string) []bson.M {
	var out []bson.M
	for _, st := range states {
		switch st {
		case "scored":
			out = append(out, bson.M{"score": bson.M{"$ne": nil}})
		case "unscored":
			// Only judgeable replies (successful, non-empty) count as "unscored" —
			// error traces are never judged, so surfacing them here would be noise.
			out = append(out, bson.M{"score": nil, "status": "ok", "output": bson.M{"$ne": ""}})
		case "low":
			out = append(out, bson.M{"score.overall": bson.M{"$lt": LowScoreThreshold}})
		}
	}
	return out
}

func (m *MongoStore) ListTraces(ctx context.Context, f TraceFilter) ([]Trace, error) {
	filter := bson.M{
		"created_at": bson.M{"$gte": f.From.UTC(), "$lt": f.To.UTC()},
	}
	if len(f.Platforms) > 0 {
		filter["platform"] = bson.M{"$in": f.Platforms}
	}
	// Each selected score state contributes a sub-filter; a trace matches if it
	// satisfies ANY of them (OR). A single state is applied inline.
	if or := mongoScoreClauses(f.ScoreStates); len(or) == 1 {
		for k, v := range or[0] {
			filter[k] = v
		}
	} else if len(or) > 1 {
		filter["$or"] = or
	}
	if f.Cursor > 0 {
		filter["id"] = bson.M{"$lt": f.Cursor}
	}
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	// Match the SQLite list view, which does not select tools/steps/skills.
	opts := options.Find().
		SetSort(bson.D{{Key: "id", Value: -1}}).
		SetLimit(int64(limit)).
		SetProjection(bson.M{"tools": 0, "steps": 0, "skills": 0})

	cur, err := m.col(colTraces).Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("list traces: %w", err)
	}
	defer cur.Close(ctx)

	var docs []mongoTrace
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("scan trace: %w", err)
	}

	var traces []Trace
	for _, d := range docs {
		traces = append(traces, Trace{
			ID:               d.ID,
			UserID:           d.UserID,
			Platform:         d.Platform,
			Source:           d.Source,
			Input:            d.Input,
			Output:           d.Output,
			Model:            d.Model,
			PromptTokens:     d.PromptTokens,
			CompletionTokens: d.CompletionTokens,
			TotalTokens:      d.TotalTokens,
			LatencyMs:        d.LatencyMs,
			ToolCount:        d.ToolCount,
			Status:           d.Status,
			Error:            d.Error,
			CreatedAt:        d.CreatedAt,
			Score:            d.Score.toTraceScore(),
		})
	}
	return traces, nil
}

// ListUnscoredTraces returns successful traces created at/after since that have
// no score yet, oldest first, capped at limit. Error traces are skipped — there
// is no useful reply to judge.
func (m *MongoStore) ListUnscoredTraces(ctx context.Context, since time.Time, limit int) ([]Trace, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	filter := bson.M{
		"score":      nil,
		"status":     "ok",
		"output":     bson.M{"$ne": ""},
		"created_at": bson.M{"$gte": since.UTC()},
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "id", Value: 1}}).
		SetLimit(int64(limit)).
		SetProjection(bson.M{"tools": 0, "steps": 0, "skills": 0})

	cur, err := m.col(colTraces).Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("list unscored traces: %w", err)
	}
	defer cur.Close(ctx)

	var docs []mongoTrace
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("scan unscored trace: %w", err)
	}

	var traces []Trace
	for _, d := range docs {
		traces = append(traces, Trace{
			ID:               d.ID,
			UserID:           d.UserID,
			Platform:         d.Platform,
			Input:            d.Input,
			Output:           d.Output,
			Model:            d.Model,
			PromptTokens:     d.PromptTokens,
			CompletionTokens: d.CompletionTokens,
			TotalTokens:      d.TotalTokens,
			LatencyMs:        d.LatencyMs,
			ToolCount:        d.ToolCount,
			Status:           d.Status,
			Error:            d.Error,
			CreatedAt:        d.CreatedAt,
		})
	}
	return traces, nil
}

// --- Trace scores (LLM-as-judge) ---

// SaveTraceScore upserts the judge verdict onto its trace as the embedded
// `score` sub-document (one score per trace; re-judging overwrites).
func (m *MongoStore) SaveTraceScore(ctx context.Context, sc *TraceScore) error {
	score := mongoScore{
		TraceID:     sc.TraceID,
		Accuracy:    sc.Accuracy,
		Helpfulness: sc.Helpfulness,
		Safety:      sc.Safety,
		Overall:     sc.Overall,
		Rationale:   sc.Rationale,
		JudgeModel:  sc.JudgeModel,
		CreatedAt:   time.Now().UTC(),
	}
	_, err := m.col(colTraces).UpdateOne(ctx,
		bson.M{"id": sc.TraceID},
		bson.M{"$set": bson.M{"score": score}},
	)
	if err != nil {
		return fmt.Errorf("save trace score: %w", err)
	}
	return nil
}

// GetTraceScore returns the score for a trace, or nil if it hasn't been judged
// (or the trace doesn't exist).
func (m *MongoStore) GetTraceScore(ctx context.Context, traceID int64) (*TraceScore, error) {
	var doc struct {
		Score *mongoScore `bson:"score"`
	}
	err := m.col(colTraces).FindOne(ctx,
		bson.M{"id": traceID},
		options.FindOne().SetProjection(bson.M{"score": 1}),
	).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get trace score: %w", err)
	}
	return doc.Score.toTraceScore(), nil
}

// userRunTotals aggregates the user's trace count and total token usage, mirroring
// the SQLite `SELECT COUNT(*), COALESCE(SUM(total_tokens), 0) FROM traces WHERE
// user_id = ?`. It backs the hybrid backend's GetUserActivity.
func (m *MongoStore) userRunTotals(ctx context.Context, userID int64) (runs int, totalTokens int, err error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"user_id": userID}}},
		{{Key: "$group", Value: bson.M{
			"_id":          nil,
			"runs":         bson.M{"$sum": 1},
			"total_tokens": bson.M{"$sum": "$total_tokens"},
		}}},
	}
	cur, err := m.col(colTraces).Aggregate(ctx, pipeline)
	if err != nil {
		return 0, 0, fmt.Errorf("user runs: %w", err)
	}
	defer cur.Close(ctx)

	var res []struct {
		Runs        int `bson:"runs"`
		TotalTokens int `bson:"total_tokens"`
	}
	if err := cur.All(ctx, &res); err != nil {
		return 0, 0, fmt.Errorf("user runs: %w", err)
	}
	if len(res) == 0 {
		return 0, 0, nil
	}
	return res[0].Runs, res[0].TotalTokens, nil
}
