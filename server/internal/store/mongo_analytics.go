package store

import (
	"context"
	"fmt"
	"math"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// mongoRangeMatch builds the standard analytics $match: the half-open interval
// [from, to) on created_at, plus an optional platform equality ("" = all). It
// mirrors the SQLite `created_at >= ? AND created_at < ?`+optional platform
// filter used by every usage query. A fresh map is returned each call so callers
// may safely add more keys (e.g. latency_ms) to it.
func mongoRangeMatch(from, to time.Time, platform string) bson.M {
	match := bson.M{"created_at": bson.M{"$gte": from.UTC(), "$lt": to.UTC()}}
	if platform != "" {
		match["platform"] = platform
	}
	return match
}

// UsageStatsBetween aggregates traces in the half-open interval [from, to),
// optionally restricted to a platform ("" = all). It is the MongoDB port of
// SQLiteStore.UsageStatsBetween and is intended to be byte-for-byte equivalent.
func (m *MongoStore) UsageStatsBetween(ctx context.Context, from, to time.Time, platform string) (*UsageStats, error) {
	fromUTC := from.UTC()
	toUTC := to.UTC()
	stats := &UsageStats{}
	match := mongoRangeMatch(fromUTC, toUTC, platform)

	// Summary: COUNT(*), SUM(tokens/tool_count), error count, COUNT(DISTINCT
	// user_id) and AVG(NULLIF(latency_ms,0)). $addToSet+len replicates the
	// distinct user count; the $cond/$$REMOVE on latency drops zero latencies
	// from the average exactly like NULLIF(latency_ms,0).
	summaryPipe := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$group", Value: bson.M{
			"_id":              nil,
			"requests":         bson.M{"$sum": 1},
			"promptTokens":     bson.M{"$sum": "$prompt_tokens"},
			"completionTokens": bson.M{"$sum": "$completion_tokens"},
			"totalTokens":      bson.M{"$sum": "$total_tokens"},
			"toolCalls":        bson.M{"$sum": "$tool_count"},
			"errors": bson.M{"$sum": bson.M{"$cond": bson.A{
				bson.M{"$eq": bson.A{"$status", "error"}}, 1, 0,
			}}},
			"users": bson.M{"$addToSet": "$user_id"},
			"avgLatency": bson.M{"$avg": bson.M{"$cond": bson.A{
				bson.M{"$ne": bson.A{"$latency_ms", 0}}, "$latency_ms", "$$REMOVE",
			}}},
		}}},
	}
	var summary []struct {
		Requests         int      `bson:"requests"`
		PromptTokens     int      `bson:"promptTokens"`
		CompletionTokens int      `bson:"completionTokens"`
		TotalTokens      int      `bson:"totalTokens"`
		ToolCalls        int      `bson:"toolCalls"`
		Errors           int      `bson:"errors"`
		Users            []int64  `bson:"users"`
		AvgLatency       *float64 `bson:"avgLatency"`
	}
	if err := m.aggregateAll(ctx, colTraces, summaryPipe, &summary); err != nil {
		return nil, fmt.Errorf("usage summary: %w", err)
	}
	if len(summary) > 0 {
		s := summary[0]
		stats.Summary.Requests = s.Requests
		stats.Summary.PromptTokens = s.PromptTokens
		stats.Summary.CompletionTokens = s.CompletionTokens
		stats.Summary.TotalTokens = s.TotalTokens
		stats.ToolCalls = s.ToolCalls
		stats.Errors = s.Errors
		stats.ActiveUsers = len(s.Users)
		if s.AvgLatency != nil {
			// SQLite does int(avgLatency.Float64) — truncation toward zero.
			stats.AvgLatencyMs = int(*s.AvgLatency)
		}
	}

	// Latency percentiles (loads the latency column for the range).
	p50, p95, p99, err := m.latencyPercentiles(ctx, fromUTC, toUTC, platform)
	if err != nil {
		return nil, err
	}
	stats.LatencyP50, stats.LatencyP95, stats.LatencyP99 = p50, p95, p99

	// Requests by hour-of-day and day-of-week (UTC; client rotates by display tz).
	// $hour is 0-23, used directly. $dayOfWeek is 1-based (Sunday=1) so we
	// subtract 1 to match SQLite strftime('%w') which is 0-based (Sunday=0).
	hourExpr := bson.M{"$hour": bson.M{"date": "$created_at", "timezone": "UTC"}}
	weekdayExpr := bson.M{"$subtract": bson.A{
		bson.M{"$dayOfWeek": bson.M{"date": "$created_at", "timezone": "UTC"}}, 1,
	}}
	if err := m.usageByBucket(ctx, fromUTC, toUTC, platform, hourExpr, stats.ByHour[:]); err != nil {
		return nil, fmt.Errorf("usage by hour: %w", err)
	}
	if err := m.usageByBucket(ctx, fromUTC, toUTC, platform, weekdayExpr, stats.ByWeekday[:]); err != nil {
		return nil, fmt.Errorf("usage by weekday: %w", err)
	}

	// By day: date(created_at) buckets in UTC, requests/errors/tokens and the
	// zero-excluded avg latency (CAST AS INTEGER == truncation).
	dayPipe := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$group", Value: bson.M{
			"_id":      bson.M{"$dateToString": bson.M{"format": "%Y-%m-%d", "date": "$created_at", "timezone": "UTC"}},
			"requests": bson.M{"$sum": 1},
			"errors": bson.M{"$sum": bson.M{"$cond": bson.A{
				bson.M{"$eq": bson.A{"$status", "error"}}, 1, 0,
			}}},
			"totalTokens": bson.M{"$sum": "$total_tokens"},
			"avgLatency": bson.M{"$avg": bson.M{"$cond": bson.A{
				bson.M{"$ne": bson.A{"$latency_ms", 0}}, "$latency_ms", "$$REMOVE",
			}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
	}
	var days []struct {
		Date        string   `bson:"_id"`
		Requests    int      `bson:"requests"`
		Errors      int      `bson:"errors"`
		TotalTokens int      `bson:"totalTokens"`
		AvgLatency  *float64 `bson:"avgLatency"`
	}
	if err := m.aggregateAll(ctx, colTraces, dayPipe, &days); err != nil {
		return nil, fmt.Errorf("usage by day: %w", err)
	}
	for _, d := range days {
		row := UsageDay{Date: d.Date, Requests: d.Requests, Errors: d.Errors, TotalTokens: d.TotalTokens}
		if d.AvgLatency != nil {
			row.AvgLatencyMs = int(*d.AvgLatency)
		}
		stats.ByDay = append(stats.ByDay, row)
	}

	// By model, ordered by total tokens desc.
	modelPipe := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$group", Value: bson.M{
			"_id":              "$model",
			"requests":         bson.M{"$sum": 1},
			"promptTokens":     bson.M{"$sum": "$prompt_tokens"},
			"completionTokens": bson.M{"$sum": "$completion_tokens"},
			"totalTokens":      bson.M{"$sum": "$total_tokens"},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "totalTokens", Value: -1}}}},
	}
	var models []struct {
		Model            string `bson:"_id"`
		Requests         int    `bson:"requests"`
		PromptTokens     int    `bson:"promptTokens"`
		CompletionTokens int    `bson:"completionTokens"`
		TotalTokens      int    `bson:"totalTokens"`
	}
	if err := m.aggregateAll(ctx, colTraces, modelPipe, &models); err != nil {
		return nil, fmt.Errorf("usage by model: %w", err)
	}
	for _, mm := range models {
		stats.ByModel = append(stats.ByModel, UsageModel{
			Model:            mm.Model,
			Requests:         mm.Requests,
			PromptTokens:     mm.PromptTokens,
			CompletionTokens: mm.CompletionTokens,
			TotalTokens:      mm.TotalTokens,
		})
	}

	// By platform: deliberately ignores the platform filter (range only) so the
	// split is always visible. Ordered by request count desc; "" -> "unknown".
	platMatch := bson.M{"created_at": bson.M{"$gte": fromUTC, "$lt": toUTC}}
	platPipe := mongo.Pipeline{
		{{Key: "$match", Value: platMatch}},
		{{Key: "$group", Value: bson.M{
			"_id":         "$platform",
			"requests":    bson.M{"$sum": 1},
			"totalTokens": bson.M{"$sum": "$total_tokens"},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "requests", Value: -1}}}},
	}
	var platforms []struct {
		Platform    string `bson:"_id"`
		Requests    int    `bson:"requests"`
		TotalTokens int    `bson:"totalTokens"`
	}
	if err := m.aggregateAll(ctx, colTraces, platPipe, &platforms); err != nil {
		return nil, fmt.Errorf("usage by platform: %w", err)
	}
	for _, p := range platforms {
		row := UsagePlatform{Platform: p.Platform, Requests: p.Requests, TotalTokens: p.TotalTokens}
		if row.Platform == "" {
			row.Platform = "unknown"
		}
		stats.ByPlatform = append(stats.ByPlatform, row)
	}

	// Top tools (from tool_usage), range+platform filtered, top 10 by count.
	toolPipe := mongo.Pipeline{
		{{Key: "$match", Value: mongoRangeMatch(fromUTC, toUTC, platform)}},
		{{Key: "$group", Value: bson.M{"_id": "$tool", "count": bson.M{"$sum": 1}}}},
		{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}}}},
		{{Key: "$limit", Value: 10}},
	}
	var tools []struct {
		Tool  string `bson:"_id"`
		Count int    `bson:"count"`
	}
	if err := m.aggregateAll(ctx, colToolUsage, toolPipe, &tools); err != nil {
		return nil, fmt.Errorf("top tools: %w", err)
	}
	for _, t := range tools {
		stats.TopTools = append(stats.TopTools, ToolCount{Tool: t.Tool, Count: t.Count})
	}

	// NB: like the SQLite version, ByUser is left unpopulated here — the API
	// layer fills it separately from UsageByUserModel.
	return stats, nil
}

// latencyPercentiles loads latency_ms (> 0) for the range and returns p50/p95/p99
// using the SAME index math as the SQLite version: pull all non-zero latencies
// sorted ascending, then pick index ceil(p*n)-1 with clamping. Empty -> zeros.
func (m *MongoStore) latencyPercentiles(ctx context.Context, from, to time.Time, platform string) (p50, p95, p99 int, err error) {
	match := mongoRangeMatch(from, to, platform)
	match["latency_ms"] = bson.M{"$gt": 0}
	pipe := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$sort", Value: bson.D{{Key: "latency_ms", Value: 1}}}},
		{{Key: "$group", Value: bson.M{"_id": nil, "arr": bson.M{"$push": "$latency_ms"}}}},
	}
	var rows []struct {
		Arr []int `bson:"arr"`
	}
	if err := m.aggregateAll(ctx, colTraces, pipe, &rows); err != nil {
		return 0, 0, 0, fmt.Errorf("latency percentiles: %w", err)
	}
	var lat []int
	if len(rows) > 0 {
		lat = rows[0].Arr
	}
	pick := func(p float64) int {
		if len(lat) == 0 {
			return 0
		}
		idx := int(math.Ceil(p*float64(len(lat)))) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= len(lat) {
			idx = len(lat) - 1
		}
		return lat[idx]
	}
	return pick(0.50), pick(0.95), pick(0.99), nil
}

// usageByBucket fills out[] with request counts grouped by a date-part expression
// (bucketExpr must evaluate to an integer bucket), in UTC. Buckets outside
// [0, len(out)) are ignored, exactly like the SQLite bounds check.
func (m *MongoStore) usageByBucket(ctx context.Context, from, to time.Time, platform string, bucketExpr interface{}, out []int) error {
	pipe := mongo.Pipeline{
		{{Key: "$match", Value: mongoRangeMatch(from, to, platform)}},
		{{Key: "$group", Value: bson.M{"_id": bucketExpr, "count": bson.M{"$sum": 1}}}},
	}
	var rows []struct {
		Bucket int `bson:"_id"`
		Count  int `bson:"count"`
	}
	if err := m.aggregateAll(ctx, colTraces, pipe, &rows); err != nil {
		return err
	}
	for _, r := range rows {
		if r.Bucket >= 0 && r.Bucket < len(out) {
			out[r.Bucket] = r.Count
		}
	}
	return nil
}

// UsageByDayModel returns per-day, per-model token sums for a cost time series.
// MongoDB port of SQLiteStore.UsageByDayModel.
func (m *MongoStore) UsageByDayModel(ctx context.Context, from, to time.Time, platform string) ([]DayModelUsage, error) {
	pipe := mongo.Pipeline{
		{{Key: "$match", Value: mongoRangeMatch(from, to, platform)}},
		{{Key: "$group", Value: bson.M{
			"_id": bson.M{
				"day":   bson.M{"$dateToString": bson.M{"format": "%Y-%m-%d", "date": "$created_at", "timezone": "UTC"}},
				"model": "$model",
			},
			"promptTokens":     bson.M{"$sum": "$prompt_tokens"},
			"completionTokens": bson.M{"$sum": "$completion_tokens"},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "_id.day", Value: 1}}}},
	}
	var rows []struct {
		ID struct {
			Day   string `bson:"day"`
			Model string `bson:"model"`
		} `bson:"_id"`
		PromptTokens     int `bson:"promptTokens"`
		CompletionTokens int `bson:"completionTokens"`
	}
	if err := m.aggregateAll(ctx, colTraces, pipe, &rows); err != nil {
		return nil, fmt.Errorf("usage by day/model: %w", err)
	}
	var out []DayModelUsage
	for _, r := range rows {
		out = append(out, DayModelUsage{
			Date:             r.ID.Day,
			Model:            r.ID.Model,
			PromptTokens:     r.PromptTokens,
			CompletionTokens: r.CompletionTokens,
		})
	}
	return out, nil
}

// UsageByUserModel returns per-user, per-model usage for the Users section
// (cost is priced in the API layer). MongoDB port of
// SQLiteStore.UsageByUserModel.
func (m *MongoStore) UsageByUserModel(ctx context.Context, from, to time.Time, platform string) ([]UserModelUsage, error) {
	pipe := mongo.Pipeline{
		{{Key: "$match", Value: mongoRangeMatch(from, to, platform)}},
		{{Key: "$group", Value: bson.M{
			"_id": bson.M{
				"user_id": "$user_id",
				"model":   "$model",
			},
			"requests":         bson.M{"$sum": 1},
			"promptTokens":     bson.M{"$sum": "$prompt_tokens"},
			"completionTokens": bson.M{"$sum": "$completion_tokens"},
			"totalTokens":      bson.M{"$sum": "$total_tokens"},
			"errors": bson.M{"$sum": bson.M{"$cond": bson.A{
				bson.M{"$eq": bson.A{"$status", "error"}}, 1, 0,
			}}},
		}}},
	}
	var rows []struct {
		ID struct {
			UserID int64  `bson:"user_id"`
			Model  string `bson:"model"`
		} `bson:"_id"`
		Requests         int `bson:"requests"`
		PromptTokens     int `bson:"promptTokens"`
		CompletionTokens int `bson:"completionTokens"`
		TotalTokens      int `bson:"totalTokens"`
		Errors           int `bson:"errors"`
	}
	if err := m.aggregateAll(ctx, colTraces, pipe, &rows); err != nil {
		return nil, fmt.Errorf("usage by user/model: %w", err)
	}
	var out []UserModelUsage
	for _, r := range rows {
		out = append(out, UserModelUsage{
			UserID:           r.ID.UserID,
			Model:            r.ID.Model,
			Requests:         r.Requests,
			PromptTokens:     r.PromptTokens,
			CompletionTokens: r.CompletionTokens,
			TotalTokens:      r.TotalTokens,
			Errors:           r.Errors,
		})
	}
	return out, nil
}

// aggregateAll runs an aggregation pipeline on the named collection and decodes
// every result document into out (a pointer to a slice). Empty result sets leave
// out as its zero value (an empty/nil slice), never an error.
func (m *MongoStore) aggregateAll(ctx context.Context, coll string, pipe mongo.Pipeline, out interface{}) error {
	cur, err := m.col(coll).Aggregate(ctx, pipe)
	if err != nil {
		return fmt.Errorf("aggregate %s: %w", coll, err)
	}
	defer cur.Close(ctx)
	if err := cur.All(ctx, out); err != nil {
		return fmt.Errorf("decode %s aggregation: %w", coll, err)
	}
	return nil
}
