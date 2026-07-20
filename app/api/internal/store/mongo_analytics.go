package store

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/authctx"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// mongoRangeMatch builds the standard analytics $match: the half-open interval
// [from, to) on created_at, an optional platform filter (nil/empty = all,
// otherwise "$in" the listed platforms), and — when the request carries an
// active project on ctx — a project_id filter so per-project dashboards only see
// their own usage. A fresh map is returned each call so callers may safely add
// more keys (e.g. latency_ms) to it.
func mongoRangeMatch(ctx context.Context, from, to time.Time, platforms []string) bson.M {
	match := bson.M{"created_at": bson.M{"$gte": from.UTC(), "$lt": to.UTC()}}
	if len(platforms) > 0 {
		match["platform"] = bson.M{"$in": platforms}
	}
	if pid := authctx.ProjectID(ctx); pid > 0 {
		match["project_id"] = pid
	}
	return match
}

// UsageStatsBetween aggregates traces in the half-open interval [from, to),
// optionally restricted to a platform ("" = all). It is the MongoDB port of
// SQLiteStore.UsageStatsBetween and is intended to be byte-for-byte equivalent.
func (m *MongoStore) UsageStatsBetween(ctx context.Context, from, to time.Time, platforms []string) (*UsageStats, error) {
	fromUTC := from.UTC()
	toUTC := to.UTC()
	stats := &UsageStats{}
	match := mongoRangeMatch(ctx, fromUTC, toUTC, platforms)

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
	p50, p95, p99, err := m.latencyPercentiles(ctx, fromUTC, toUTC, platforms)
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
	if err := m.usageByBucket(ctx, fromUTC, toUTC, platforms, hourExpr, stats.ByHour[:]); err != nil {
		return nil, fmt.Errorf("usage by hour: %w", err)
	}
	if err := m.usageByBucket(ctx, fromUTC, toUTC, platforms, weekdayExpr, stats.ByWeekday[:]); err != nil {
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

	// Fold image-generation usage (gpt-image-1) into the aggregates as its own
	// model. Image tokens live in dedicated fields and are priced with a much
	// higher rate, so they are summed here rather than through the LLM token
	// fields above: this keeps the summary and per-day token totals combined
	// (LLM + image) while the by-model breakdown still shows the two apart.
	imgMatch := mongoRangeMatch(ctx, fromUTC, toUTC, platforms)
	imgMatch["image_total_tokens"] = bson.M{"$gt": 0}
	imgModelPipe := mongo.Pipeline{
		{{Key: "$match", Value: imgMatch}},
		{{Key: "$group", Value: bson.M{
			"_id":              "$image_model",
			"requests":         bson.M{"$sum": 1},
			"promptTokens":     bson.M{"$sum": "$image_prompt_tokens"},
			"completionTokens": bson.M{"$sum": "$image_completion_tokens"},
			"totalTokens":      bson.M{"$sum": "$image_total_tokens"},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "totalTokens", Value: -1}}}},
	}
	var imgModels []struct {
		Model            string `bson:"_id"`
		Requests         int    `bson:"requests"`
		PromptTokens     int    `bson:"promptTokens"`
		CompletionTokens int    `bson:"completionTokens"`
		TotalTokens      int    `bson:"totalTokens"`
	}
	if err := m.aggregateAll(ctx, colTraces, imgModelPipe, &imgModels); err != nil {
		return nil, fmt.Errorf("usage by image model: %w", err)
	}
	for _, im := range imgModels {
		stats.ByModel = append(stats.ByModel, UsageModel{
			Model:            im.Model,
			Requests:         im.Requests,
			PromptTokens:     im.PromptTokens,
			CompletionTokens: im.CompletionTokens,
			TotalTokens:      im.TotalTokens,
		})
		stats.Summary.PromptTokens += im.PromptTokens
		stats.Summary.CompletionTokens += im.CompletionTokens
		stats.Summary.TotalTokens += im.TotalTokens
	}

	// Image tokens per day, added onto the combined by-day token series so the
	// tokens line tracks the (combined) cost line on the dashboard.
	imgDayPipe := mongo.Pipeline{
		{{Key: "$match", Value: imgMatch}},
		{{Key: "$group", Value: bson.M{
			"_id":         bson.M{"$dateToString": bson.M{"format": "%Y-%m-%d", "date": "$created_at", "timezone": "UTC"}},
			"totalTokens": bson.M{"$sum": "$image_total_tokens"},
		}}},
	}
	var imgDays []struct {
		Date        string `bson:"_id"`
		TotalTokens int    `bson:"totalTokens"`
	}
	if err := m.aggregateAll(ctx, colTraces, imgDayPipe, &imgDays); err != nil {
		return nil, fmt.Errorf("usage image by day: %w", err)
	}
	imgByDay := make(map[string]int, len(imgDays))
	for _, d := range imgDays {
		imgByDay[d.Date] = d.TotalTokens
	}
	for i := range stats.ByDay {
		stats.ByDay[i].TotalTokens += imgByDay[stats.ByDay[i].Date]
	}

	// By platform: deliberately ignores the platform filter (range only) so the
	// split is always visible, but still respects the active-project scope.
	// Ordered by request count desc; "" -> "unknown".
	platMatch := mongoRangeMatch(ctx, fromUTC, toUTC, nil)
	platPipe := mongo.Pipeline{
		{{Key: "$match", Value: platMatch}},
		{{Key: "$group", Value: bson.M{
			"_id":         "$platform",
			"requests":    bson.M{"$sum": 1},
			"totalTokens": bson.M{"$sum": "$total_tokens"},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "requests", Value: -1}}}},
	}
	var platformRows []struct {
		Platform    string `bson:"_id"`
		Requests    int    `bson:"requests"`
		TotalTokens int    `bson:"totalTokens"`
	}
	if err := m.aggregateAll(ctx, colTraces, platPipe, &platformRows); err != nil {
		return nil, fmt.Errorf("usage by platform: %w", err)
	}
	for _, p := range platformRows {
		row := UsagePlatform{Platform: p.Platform, Requests: p.Requests, TotalTokens: p.TotalTokens}
		if row.Platform == "" {
			row.Platform = "unknown"
		}
		stats.ByPlatform = append(stats.ByPlatform, row)
	}

	// Top tools (from tool_usage), range+platform filtered, top 10 by count.
	toolPipe := mongo.Pipeline{
		{{Key: "$match", Value: mongoRangeMatch(ctx, fromUTC, toUTC, platforms)}},
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
func (m *MongoStore) latencyPercentiles(ctx context.Context, from, to time.Time, platforms []string) (p50, p95, p99 int, err error) {
	match := mongoRangeMatch(ctx, from, to, platforms)
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
func (m *MongoStore) usageByBucket(ctx context.Context, from, to time.Time, platforms []string, bucketExpr interface{}, out []int) error {
	pipe := mongo.Pipeline{
		{{Key: "$match", Value: mongoRangeMatch(ctx, from, to, platforms)}},
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
func (m *MongoStore) UsageByDayModel(ctx context.Context, from, to time.Time, platforms []string) ([]DayModelUsage, error) {
	pipe := mongo.Pipeline{
		{{Key: "$match", Value: mongoRangeMatch(ctx, from, to, platforms)}},
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

	// Image-generation usage (gpt-image-1) contributes its own per-day rows so
	// the by-day cost series priced in the API layer covers LLM + image combined.
	imgMatch := mongoRangeMatch(ctx, from, to, platforms)
	imgMatch["image_total_tokens"] = bson.M{"$gt": 0}
	imgPipe := mongo.Pipeline{
		{{Key: "$match", Value: imgMatch}},
		{{Key: "$group", Value: bson.M{
			"_id": bson.M{
				"day":   bson.M{"$dateToString": bson.M{"format": "%Y-%m-%d", "date": "$created_at", "timezone": "UTC"}},
				"model": "$image_model",
			},
			"promptTokens":     bson.M{"$sum": "$image_prompt_tokens"},
			"completionTokens": bson.M{"$sum": "$image_completion_tokens"},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "_id.day", Value: 1}}}},
	}
	var imgRows []struct {
		ID struct {
			Day   string `bson:"day"`
			Model string `bson:"model"`
		} `bson:"_id"`
		PromptTokens     int `bson:"promptTokens"`
		CompletionTokens int `bson:"completionTokens"`
	}
	if err := m.aggregateAll(ctx, colTraces, imgPipe, &imgRows); err != nil {
		return nil, fmt.Errorf("usage image by day/model: %w", err)
	}
	for _, r := range imgRows {
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
func (m *MongoStore) UsageByUserModel(ctx context.Context, from, to time.Time, platforms []string) ([]UserModelUsage, error) {
	pipe := mongo.Pipeline{
		{{Key: "$match", Value: mongoRangeMatch(ctx, from, to, platforms)}},
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

	// Image-generation usage (gpt-image-1) as its own per-user rows so per-user
	// cost includes image generation. Requests/Errors stay zero: the run is
	// already counted by its LLM row above, so counting it again would double the
	// user's request total.
	imgMatch := mongoRangeMatch(ctx, from, to, platforms)
	imgMatch["image_total_tokens"] = bson.M{"$gt": 0}
	imgPipe := mongo.Pipeline{
		{{Key: "$match", Value: imgMatch}},
		{{Key: "$group", Value: bson.M{
			"_id": bson.M{
				"user_id": "$user_id",
				"model":   "$image_model",
			},
			"promptTokens":     bson.M{"$sum": "$image_prompt_tokens"},
			"completionTokens": bson.M{"$sum": "$image_completion_tokens"},
			"totalTokens":      bson.M{"$sum": "$image_total_tokens"},
		}}},
	}
	var imgRows []struct {
		ID struct {
			UserID int64  `bson:"user_id"`
			Model  string `bson:"model"`
		} `bson:"_id"`
		PromptTokens     int `bson:"promptTokens"`
		CompletionTokens int `bson:"completionTokens"`
		TotalTokens      int `bson:"totalTokens"`
	}
	if err := m.aggregateAll(ctx, colTraces, imgPipe, &imgRows); err != nil {
		return nil, fmt.Errorf("usage image by user/model: %w", err)
	}
	for _, r := range imgRows {
		out = append(out, UserModelUsage{
			UserID:           r.ID.UserID,
			Model:            r.ID.Model,
			PromptTokens:     r.PromptTokens,
			CompletionTokens: r.CompletionTokens,
			TotalTokens:      r.TotalTokens,
		})
	}
	return out, nil
}

// UsageByProject aggregates trace usage per (project, model) over [from, to).
// Traces written before multi-project (or outside a project) carry project_id 0
// and are grouped there.
func (m *MongoStore) UsageByProject(ctx context.Context, from, to time.Time) ([]ProjectModelUsage, error) {
	pipe := mongo.Pipeline{
		{{Key: "$match", Value: mongoRangeMatch(ctx, from, to, nil)}},
		{{Key: "$group", Value: bson.M{
			"_id": bson.M{
				"project_id": bson.M{"$ifNull": bson.A{"$project_id", int64(0)}},
				"model":      "$model",
			},
			"requests":         bson.M{"$sum": 1},
			"promptTokens":     bson.M{"$sum": "$prompt_tokens"},
			"completionTokens": bson.M{"$sum": "$completion_tokens"},
			"totalTokens":      bson.M{"$sum": "$total_tokens"},
		}}},
	}
	var rows []struct {
		ID struct {
			ProjectID int64  `bson:"project_id"`
			Model     string `bson:"model"`
		} `bson:"_id"`
		Requests         int `bson:"requests"`
		PromptTokens     int `bson:"promptTokens"`
		CompletionTokens int `bson:"completionTokens"`
		TotalTokens      int `bson:"totalTokens"`
	}
	if err := m.aggregateAll(ctx, colTraces, pipe, &rows); err != nil {
		return nil, fmt.Errorf("usage by project: %w", err)
	}
	out := make([]ProjectModelUsage, 0, len(rows))
	for _, r := range rows {
		out = append(out, ProjectModelUsage{
			ProjectID:        r.ID.ProjectID,
			Model:            r.ID.Model,
			Requests:         r.Requests,
			PromptTokens:     r.PromptTokens,
			CompletionTokens: r.CompletionTokens,
			TotalTokens:      r.TotalTokens,
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
