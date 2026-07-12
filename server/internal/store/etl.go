package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MigrateOptions controls the one-time SQLite -> hybrid ETL.
type MigrateOptions struct {
	// Truncate clears the destination Postgres data tables (except the seeded
	// skills table) and the Mongo log collections before migrating. Without it a
	// second run may hit unique/PK conflicts — see MigrateSQLiteToHybrid.
	Truncate bool
}

// MigrateReport records, per logical table, the [sourceCount, destCount] pair so
// a caller can eyeball (or Verify) that every row made it across.
type MigrateReport struct {
	Counts map[string][2]int
}

// Verify returns a non-nil error listing every table whose source and
// destination counts differ. A nil error means every table matched.
func (r *MigrateReport) Verify() error {
	var bad []string
	tables := make([]string, 0, len(r.Counts))
	for t := range r.Counts {
		tables = append(tables, t)
	}
	sort.Strings(tables)
	for _, t := range tables {
		c := r.Counts[t]
		if c[0] != c[1] {
			bad = append(bad, fmt.Sprintf("%s: src=%d dst=%d", t, c[0], c[1]))
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("row count mismatch: %s", strings.Join(bad, "; "))
	}
	return nil
}

// MigrateSQLiteToHybrid copies every table from the single-file SQLite store into
// the split Postgres+Mongo (hybrid) backend, preserving original ids so foreign
// keys stay intact. Main data goes to Postgres, append-only logs to Mongo.
//
// Ordering: users first, then user-scoped and global data, then the Mongo logs.
// Original ids are preserved on the Postgres identity tables via OVERRIDING
// SYSTEM VALUE (with a sequence reset afterwards) and carried in the Mongo `id`
// field on the log documents.
//
// Idempotency: pass opts.Truncate to clear the destination first. Without it, a
// second run will conflict on primary/unique keys — this ETL is a one-shot.
func MigrateSQLiteToHybrid(ctx context.Context, src *SQLiteStore, dst *HybridStore, opts MigrateOptions) (*MigrateReport, error) {
	m := &etl{
		ctx:    ctx,
		src:    src.db,
		dst:    dst,
		report: &MigrateReport{Counts: map[string][2]int{}},
	}

	if opts.Truncate {
		if err := m.truncate(); err != nil {
			return nil, fmt.Errorf("truncate: %w", err)
		}
	}

	// Postgres main data, in FK-safe order.
	if err := m.users(); err != nil {
		return nil, err
	}
	skillMap, err := m.skills()
	if err != nil {
		return nil, err
	}
	if err := m.userSkills(skillMap); err != nil {
		return nil, err
	}
	if err := m.contacts(); err != nil {
		return nil, err
	}
	if err := m.bucketItems(); err != nil {
		return nil, err
	}
	if err := m.trips(); err != nil {
		return nil, err
	}
	if err := m.tripExpenses(); err != nil {
		return nil, err
	}
	if err := m.hikeMountains(); err != nil {
		return nil, err
	}
	if err := m.hikeTracks(); err != nil {
		return nil, err
	}
	if err := m.hikeParticipants(); err != nil {
		return nil, err
	}
	if err := m.hikes(); err != nil {
		return nil, err
	}
	if err := m.hikeHikers(); err != nil {
		return nil, err
	}
	if err := m.reminders(); err != nil {
		return nil, err
	}
	if err := m.userPersonas(); err != nil {
		return nil, err
	}
	if err := m.memories(); err != nil {
		return nil, err
	}
	if err := m.notes(); err != nil {
		return nil, err
	}
	if err := m.oauthTokens(); err != nil {
		return nil, err
	}
	if err := m.settings(); err != nil {
		return nil, err
	}
	if err := m.modelPrices(); err != nil {
		return nil, err
	}

	// Mongo logs last.
	if err := m.messageLog(); err != nil {
		return nil, err
	}
	if err := m.toolUsage(); err != nil {
		return nil, err
	}
	if err := m.activities(); err != nil {
		return nil, err
	}
	if err := m.traces(); err != nil {
		return nil, err
	}

	return m.report, nil
}

// etl carries the shared handles through the per-table migration steps.
type etl struct {
	ctx    context.Context
	src    *sql.DB
	dst    *HybridStore
	report *MigrateReport
}

func boolFromInt(i int64) bool { return i != 0 }

// anyToBool coerces a value scanned from a SQLite column that go-sqlite3 may
// return as either a bool (declared-BOOLEAN columns written from a Go bool) or
// an int64 (defaulted / INTEGER-stored rows). Used for the reminders BOOLEAN
// columns, whose runtime type varies by how each row was written.
func anyToBool(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case int64:
		return x != 0
	default:
		return false
	}
}

// pgExec runs a single write against the Postgres pool.
func (m *etl) pgExec(sqlStr string, args ...any) error {
	_, err := m.dst.PostgresStore.pool.Exec(m.ctx, sqlStr, args...)
	return err
}

// pgCount returns COUNT(*) for a Postgres table.
func (m *etl) pgCount(table string) (int, error) {
	var n int
	err := m.dst.PostgresStore.pool.QueryRow(m.ctx, `SELECT COUNT(*) FROM `+table).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count %s: %w", table, err)
	}
	return n, nil
}

// resetSequence advances a Postgres identity sequence past the max migrated id
// so future inserts don't collide with the ids we forced in. The table name is
// bound for the pg_get_serial_sequence lookup; the FROM clause needs the literal.
func (m *etl) resetSequence(table string) error {
	_, err := m.dst.PostgresStore.pool.Exec(m.ctx,
		`SELECT setval(pg_get_serial_sequence($1, 'id'), (SELECT COALESCE(MAX(id), 1) FROM `+table+`))`,
		table)
	if err != nil {
		return fmt.Errorf("reset sequence %s: %w", table, err)
	}
	return nil
}

func (m *etl) mongoCol(name string) *mongo.Collection { return m.dst.MongoStore.col(name) }

func (m *etl) mongoCount(name string) (int, error) {
	n, err := m.mongoCol(name).CountDocuments(m.ctx, bson.M{})
	if err != nil {
		return 0, fmt.Errorf("count %s: %w", name, err)
	}
	return int(n), nil
}

// truncate clears the destination before a fresh migration. The skills table is
// left intact because NewPostgres seeds it; everything else is emptied. The
// Postgres schema declares no foreign keys, so a plain RESTART IDENTITY is
// enough. Mongo counters are cleared too so nextSeq restarts cleanly (the ETL
// re-seeds them at the end).
func (m *etl) truncate() error {
	pgTables := []string{
		"users", "contacts", "bucket_list_items", "trips", "trip_expenses",
		"hike_mountains", "hike_tracks", "hike_participants", "hikes", "hike_hikers",
		"user_skills", "reminders", "user_personas", "memories", "notes",
		"oauth_tokens", "settings", "model_prices",
	}
	if err := m.pgExec(`TRUNCATE ` + strings.Join(pgTables, ", ") + ` RESTART IDENTITY`); err != nil {
		return fmt.Errorf("truncate postgres: %w", err)
	}
	for _, c := range []string{colMessageLog, colToolUsage, colActivities, colTraces, colCounters} {
		if _, err := m.mongoCol(c).DeleteMany(m.ctx, bson.M{}); err != nil {
			return fmt.Errorf("clear mongo %s: %w", c, err)
		}
	}
	return nil
}

// --- Postgres identity tables (id preserved via OVERRIDING SYSTEM VALUE) ---

func (m *etl) users() error {
	rows, err := m.src.QueryContext(m.ctx,
		`SELECT id, email, name, password_hash, role, created_at FROM users`)
	if err != nil {
		return fmt.Errorf("read users: %w", err)
	}
	defer rows.Close()

	src := 0
	for rows.Next() {
		var (
			id                              int64
			email, name, passwordHash, role string
			created                         sql.NullTime
		)
		if err := rows.Scan(&id, &email, &name, &passwordHash, &role, &created); err != nil {
			return fmt.Errorf("scan user: %w", err)
		}
		if err := m.pgExec(
			`INSERT INTO users (id, email, name, password_hash, role, created_at)
			 OVERRIDING SYSTEM VALUE VALUES ($1, $2, $3, $4, $5, $6)`,
			id, email, name, passwordHash, role, nullTime(created),
		); err != nil {
			return fmt.Errorf("insert user %d: %w", id, err)
		}
		src++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read users: %w", err)
	}
	if err := m.resetSequence("users"); err != nil {
		return err
	}
	return m.record("users", src, "users")
}

func (m *etl) contacts() error {
	rows, err := m.src.QueryContext(m.ctx,
		`SELECT id, user_id, name, phone, email, note, created_at FROM contacts`)
	if err != nil {
		return fmt.Errorf("read contacts: %w", err)
	}
	defer rows.Close()

	src := 0
	for rows.Next() {
		var (
			id, userID               int64
			name, phone, email, note string
			created                  sql.NullTime
		)
		if err := rows.Scan(&id, &userID, &name, &phone, &email, &note, &created); err != nil {
			return fmt.Errorf("scan contact: %w", err)
		}
		if err := m.pgExec(
			`INSERT INTO contacts (id, user_id, name, phone, email, note, created_at)
			 OVERRIDING SYSTEM VALUE VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			id, userID, name, phone, email, note, nullTime(created),
		); err != nil {
			return fmt.Errorf("insert contact %d: %w", id, err)
		}
		src++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read contacts: %w", err)
	}
	if err := m.resetSequence("contacts"); err != nil {
		return err
	}
	return m.record("contacts", src, "contacts")
}

func (m *etl) bucketItems() error {
	rows, err := m.src.QueryContext(m.ctx,
		`SELECT id, user_id, title, description, note, category, resolution_year, done, created_at, done_at FROM bucket_list_items`)
	if err != nil {
		return fmt.Errorf("read bucket_list_items: %w", err)
	}
	defer rows.Close()

	src := 0
	for rows.Next() {
		var (
			id, userID               int64
			title, description, note string
			category                 string
			resolutionYear           sql.NullInt64
			done                     int64
			created                  sql.NullTime
			doneAt                   sql.NullTime
		)
		if err := rows.Scan(&id, &userID, &title, &description, &note, &category, &resolutionYear, &done, &created, &doneAt); err != nil {
			return fmt.Errorf("scan bucket_list_item: %w", err)
		}
		var resYear any
		if resolutionYear.Valid {
			resYear = resolutionYear.Int64
		}
		if err := m.pgExec(
			`INSERT INTO bucket_list_items (id, user_id, title, description, note, category, resolution_year, done, created_at, done_at)
			 OVERRIDING SYSTEM VALUE VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			id, userID, title, description, note, category, resYear, boolFromInt(done), nullTime(created), nullTime(doneAt),
		); err != nil {
			return fmt.Errorf("insert bucket_list_item %d: %w", id, err)
		}
		src++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read bucket_list_items: %w", err)
	}
	if err := m.resetSequence("bucket_list_items"); err != nil {
		return err
	}
	return m.record("bucket_list_items", src, "bucket_list_items")
}

func (m *etl) trips() error {
	rows, err := m.src.QueryContext(m.ctx,
		`SELECT id, user_id, name, destination, budget, currency, active, started_at FROM trips`)
	if err != nil {
		return fmt.Errorf("read trips: %w", err)
	}
	defer rows.Close()

	src := 0
	for rows.Next() {
		var (
			id, userID             int64
			name, destination, cur string
			budget                 float64
			active                 int64
			started                sql.NullTime
		)
		if err := rows.Scan(&id, &userID, &name, &destination, &budget, &cur, &active, &started); err != nil {
			return fmt.Errorf("scan trip: %w", err)
		}
		if err := m.pgExec(
			`INSERT INTO trips (id, user_id, name, destination, budget, currency, active, started_at)
			 OVERRIDING SYSTEM VALUE VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			id, userID, name, destination, budget, cur, boolFromInt(active), nullTime(started),
		); err != nil {
			return fmt.Errorf("insert trip %d: %w", id, err)
		}
		src++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read trips: %w", err)
	}
	if err := m.resetSequence("trips"); err != nil {
		return err
	}
	return m.record("trips", src, "trips")
}

func (m *etl) tripExpenses() error {
	rows, err := m.src.QueryContext(m.ctx,
		`SELECT id, user_id, trip_id, amount, currency, category, note, spent_at FROM trip_expenses`)
	if err != nil {
		return fmt.Errorf("read trip_expenses: %w", err)
	}
	defer rows.Close()

	src := 0
	for rows.Next() {
		var (
			id, userID, tripID  int64
			amount              float64
			cur, category, note string
			spent               sql.NullTime
		)
		if err := rows.Scan(&id, &userID, &tripID, &amount, &cur, &category, &note, &spent); err != nil {
			return fmt.Errorf("scan trip_expense: %w", err)
		}
		if err := m.pgExec(
			`INSERT INTO trip_expenses (id, user_id, trip_id, amount, currency, category, note, spent_at)
			 OVERRIDING SYSTEM VALUE VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			id, userID, tripID, amount, cur, category, note, nullTime(spent),
		); err != nil {
			return fmt.Errorf("insert trip_expense %d: %w", id, err)
		}
		src++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read trip_expenses: %w", err)
	}
	if err := m.resetSequence("trip_expenses"); err != nil {
		return err
	}
	return m.record("trip_expenses", src, "trip_expenses")
}

func (m *etl) hikeMountains() error {
	rows, err := m.src.QueryContext(m.ctx,
		`SELECT id, user_id, name, created_at FROM hike_mountains`)
	if err != nil {
		return fmt.Errorf("read hike_mountains: %w", err)
	}
	defer rows.Close()

	src := 0
	for rows.Next() {
		var (
			id, userID int64
			name       string
			created    sql.NullTime
		)
		if err := rows.Scan(&id, &userID, &name, &created); err != nil {
			return fmt.Errorf("scan hike_mountain: %w", err)
		}
		if err := m.pgExec(
			`INSERT INTO hike_mountains (id, user_id, name, created_at)
			 OVERRIDING SYSTEM VALUE VALUES ($1, $2, $3, $4)`,
			id, userID, name, nullTime(created),
		); err != nil {
			return fmt.Errorf("insert hike_mountain %d: %w", id, err)
		}
		src++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read hike_mountains: %w", err)
	}
	if err := m.resetSequence("hike_mountains"); err != nil {
		return err
	}
	return m.record("hike_mountains", src, "hike_mountains")
}

func (m *etl) hikeTracks() error {
	rows, err := m.src.QueryContext(m.ctx,
		`SELECT id, user_id, mountain_id, name, created_at FROM hike_tracks`)
	if err != nil {
		return fmt.Errorf("read hike_tracks: %w", err)
	}
	defer rows.Close()

	src := 0
	for rows.Next() {
		var (
			id, userID, mountainID int64
			name                   string
			created                sql.NullTime
		)
		if err := rows.Scan(&id, &userID, &mountainID, &name, &created); err != nil {
			return fmt.Errorf("scan hike_track: %w", err)
		}
		if err := m.pgExec(
			`INSERT INTO hike_tracks (id, user_id, mountain_id, name, created_at)
			 OVERRIDING SYSTEM VALUE VALUES ($1, $2, $3, $4, $5)`,
			id, userID, mountainID, name, nullTime(created),
		); err != nil {
			return fmt.Errorf("insert hike_track %d: %w", id, err)
		}
		src++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read hike_tracks: %w", err)
	}
	if err := m.resetSequence("hike_tracks"); err != nil {
		return err
	}
	return m.record("hike_tracks", src, "hike_tracks")
}

func (m *etl) hikeParticipants() error {
	rows, err := m.src.QueryContext(m.ctx,
		`SELECT id, user_id, name, created_at FROM hike_participants`)
	if err != nil {
		return fmt.Errorf("read hike_participants: %w", err)
	}
	defer rows.Close()

	src := 0
	for rows.Next() {
		var (
			id, userID int64
			name       string
			created    sql.NullTime
		)
		if err := rows.Scan(&id, &userID, &name, &created); err != nil {
			return fmt.Errorf("scan hike_participant: %w", err)
		}
		if err := m.pgExec(
			`INSERT INTO hike_participants (id, user_id, name, created_at)
			 OVERRIDING SYSTEM VALUE VALUES ($1, $2, $3, $4)`,
			id, userID, name, nullTime(created),
		); err != nil {
			return fmt.Errorf("insert hike_participant %d: %w", id, err)
		}
		src++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read hike_participants: %w", err)
	}
	if err := m.resetSequence("hike_participants"); err != nil {
		return err
	}
	return m.record("hike_participants", src, "hike_participants")
}

func (m *etl) hikes() error {
	rows, err := m.src.QueryContext(m.ctx,
		`SELECT id, user_id, mountain_id, camped, up_track_id, down_track_id, days, nights, hiked_on, created_at FROM hikes`)
	if err != nil {
		return fmt.Errorf("read hikes: %w", err)
	}
	defer rows.Close()

	src := 0
	for rows.Next() {
		var (
			id, userID, mountainID int64
			camped                 int64
			upTrackID, downTrackID int64
			days, nights           int64
			hikedOn                sql.NullTime
			created                sql.NullTime
		)
		if err := rows.Scan(&id, &userID, &mountainID, &camped, &upTrackID, &downTrackID, &days, &nights, &hikedOn, &created); err != nil {
			return fmt.Errorf("scan hike: %w", err)
		}
		if err := m.pgExec(
			`INSERT INTO hikes (id, user_id, mountain_id, camped, up_track_id, down_track_id, days, nights, hiked_on, created_at)
			 OVERRIDING SYSTEM VALUE VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			id, userID, mountainID, boolFromInt(camped), upTrackID, downTrackID, days, nights, nullTime(hikedOn), nullTime(created),
		); err != nil {
			return fmt.Errorf("insert hike %d: %w", id, err)
		}
		src++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read hikes: %w", err)
	}
	if err := m.resetSequence("hikes"); err != nil {
		return err
	}
	return m.record("hikes", src, "hikes")
}

func (m *etl) reminders() error {
	rows, err := m.src.QueryContext(m.ctx, `SELECT
		id, user_id, title, message, remind_at, repeat_mode, times, weekdays, day_of_month,
		once_date, event_at, offsets, enabled, last_fired_at, calendar_conn, calendar_event_ids,
		calendar_hash, notified, cancelled, created_at
		FROM reminders`)
	if err != nil {
		return fmt.Errorf("read reminders: %w", err)
	}
	defer rows.Close()

	src := 0
	for rows.Next() {
		var (
			id, userID                    int64
			title, message                string
			remindAt                      sql.NullTime
			repeatMode, times, weekdays   string
			dayOfMonth                    int64
			onceDate, eventAt, offsets    string
			enabled                       any
			lastFired                     sql.NullTime
			calConn, calEventIDs, calHash string
			notified, cancelled           any
			created                       sql.NullTime
		)
		if err := rows.Scan(&id, &userID, &title, &message, &remindAt, &repeatMode, &times, &weekdays,
			&dayOfMonth, &onceDate, &eventAt, &offsets, &enabled, &lastFired, &calConn, &calEventIDs,
			&calHash, &notified, &cancelled, &created); err != nil {
			return fmt.Errorf("scan reminder: %w", err)
		}
		if err := m.pgExec(
			`INSERT INTO reminders (id, user_id, title, message, remind_at, repeat_mode, times, weekdays,
			   day_of_month, once_date, event_at, offsets, enabled, last_fired_at, calendar_conn,
			   calendar_event_ids, calendar_hash, notified, cancelled, created_at)
			 OVERRIDING SYSTEM VALUE VALUES
			   ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)`,
			id, userID, title, message, nullTime(remindAt), repeatMode, times, weekdays,
			dayOfMonth, onceDate, eventAt, offsets, anyToBool(enabled), nullTime(lastFired), calConn,
			calEventIDs, calHash, anyToBool(notified), anyToBool(cancelled), nullTime(created),
		); err != nil {
			return fmt.Errorf("insert reminder %d: %w", id, err)
		}
		src++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read reminders: %w", err)
	}
	if err := m.resetSequence("reminders"); err != nil {
		return err
	}
	return m.record("reminders", src, "reminders")
}

func (m *etl) memories() error {
	rows, err := m.src.QueryContext(m.ctx,
		`SELECT id, user_id, content, kind, created_at FROM memories`)
	if err != nil {
		return fmt.Errorf("read memories: %w", err)
	}
	defer rows.Close()

	src := 0
	for rows.Next() {
		var (
			id, userID    int64
			content, kind string
			created       sql.NullTime
		)
		if err := rows.Scan(&id, &userID, &content, &kind, &created); err != nil {
			return fmt.Errorf("scan memory: %w", err)
		}
		if err := m.pgExec(
			`INSERT INTO memories (id, user_id, content, kind, created_at)
			 OVERRIDING SYSTEM VALUE VALUES ($1, $2, $3, $4, $5)`,
			id, userID, content, kind, nullTime(created),
		); err != nil {
			return fmt.Errorf("insert memory %d: %w", id, err)
		}
		src++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read memories: %w", err)
	}
	if err := m.resetSequence("memories"); err != nil {
		return err
	}
	return m.record("memories", src, "memories")
}

func (m *etl) notes() error {
	rows, err := m.src.QueryContext(m.ctx,
		`SELECT id, user_id, title, content, tags, created_at, updated_at FROM notes`)
	if err != nil {
		return fmt.Errorf("read notes: %w", err)
	}
	defer rows.Close()

	src := 0
	for rows.Next() {
		var (
			id, userID           int64
			title, content, tags string
			created, updated     sql.NullTime
		)
		if err := rows.Scan(&id, &userID, &title, &content, &tags, &created, &updated); err != nil {
			return fmt.Errorf("scan note: %w", err)
		}
		if err := m.pgExec(
			`INSERT INTO notes (id, user_id, title, content, tags, created_at, updated_at)
			 OVERRIDING SYSTEM VALUE VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			id, userID, title, content, tags, nullTime(created), nullTime(updated),
		); err != nil {
			return fmt.Errorf("insert note %d: %w", id, err)
		}
		src++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read notes: %w", err)
	}
	if err := m.resetSequence("notes"); err != nil {
		return err
	}
	return m.record("notes", src, "notes")
}

// --- Postgres natural-key tables (no identity id) ---

func (m *etl) hikeHikers() error {
	rows, err := m.src.QueryContext(m.ctx, `SELECT hike_id, participant_id FROM hike_hikers`)
	if err != nil {
		return fmt.Errorf("read hike_hikers: %w", err)
	}
	defer rows.Close()

	src := 0
	for rows.Next() {
		var hikeID, participantID int64
		if err := rows.Scan(&hikeID, &participantID); err != nil {
			return fmt.Errorf("scan hike_hiker: %w", err)
		}
		if err := m.pgExec(
			`INSERT INTO hike_hikers (hike_id, participant_id) VALUES ($1, $2)`,
			hikeID, participantID,
		); err != nil {
			return fmt.Errorf("insert hike_hiker (%d,%d): %w", hikeID, participantID, err)
		}
		src++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read hike_hikers: %w", err)
	}
	return m.record("hike_hikers", src, "hike_hikers")
}

func (m *etl) userPersonas() error {
	rows, err := m.src.QueryContext(m.ctx,
		`SELECT user_id, tone, emoji, length, personality, name, custom, updated_at FROM user_personas`)
	if err != nil {
		return fmt.Errorf("read user_personas: %w", err)
	}
	defer rows.Close()

	src := 0
	for rows.Next() {
		var (
			userID                                         int64
			tone, emoji, length, personality, name, custom string
			updated                                        sql.NullTime
		)
		if err := rows.Scan(&userID, &tone, &emoji, &length, &personality, &name, &custom, &updated); err != nil {
			return fmt.Errorf("scan user_persona: %w", err)
		}
		if err := m.pgExec(
			`INSERT INTO user_personas (user_id, tone, emoji, length, personality, name, custom, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			userID, tone, emoji, length, personality, name, custom, nullTime(updated),
		); err != nil {
			return fmt.Errorf("insert user_persona %d: %w", userID, err)
		}
		src++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read user_personas: %w", err)
	}
	return m.record("user_personas", src, "user_personas")
}

func (m *etl) oauthTokens() error {
	rows, err := m.src.QueryContext(m.ctx,
		`SELECT service, token_data, updated_at FROM oauth_tokens`)
	if err != nil {
		return fmt.Errorf("read oauth_tokens: %w", err)
	}
	defer rows.Close()

	src := 0
	for rows.Next() {
		var (
			service   string
			tokenData []byte
			updated   sql.NullTime
		)
		if err := rows.Scan(&service, &tokenData, &updated); err != nil {
			return fmt.Errorf("scan oauth_token: %w", err)
		}
		if err := m.pgExec(
			`INSERT INTO oauth_tokens (service, token_data, updated_at) VALUES ($1, $2, $3)`,
			service, tokenData, nullTime(updated),
		); err != nil {
			return fmt.Errorf("insert oauth_token %s: %w", service, err)
		}
		src++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read oauth_tokens: %w", err)
	}
	return m.record("oauth_tokens", src, "oauth_tokens")
}

func (m *etl) settings() error {
	rows, err := m.src.QueryContext(m.ctx, `SELECT key, value, updated_at FROM settings`)
	if err != nil {
		return fmt.Errorf("read settings: %w", err)
	}
	defer rows.Close()

	src := 0
	for rows.Next() {
		var (
			key     string
			value   []byte
			updated sql.NullTime
		)
		if err := rows.Scan(&key, &value, &updated); err != nil {
			return fmt.Errorf("scan setting: %w", err)
		}
		if err := m.pgExec(
			`INSERT INTO settings (key, value, updated_at) VALUES ($1, $2, $3)`,
			key, value, nullTime(updated),
		); err != nil {
			return fmt.Errorf("insert setting %s: %w", key, err)
		}
		src++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read settings: %w", err)
	}
	return m.record("settings", src, "settings")
}

func (m *etl) modelPrices() error {
	rows, err := m.src.QueryContext(m.ctx,
		`SELECT model, input_per_1m, output_per_1m, updated_at FROM model_prices`)
	if err != nil {
		return fmt.Errorf("read model_prices: %w", err)
	}
	defer rows.Close()

	src := 0
	for rows.Next() {
		var (
			model                   string
			inputPer1M, outputPer1M float64
			updated                 sql.NullTime
		)
		if err := rows.Scan(&model, &inputPer1M, &outputPer1M, &updated); err != nil {
			return fmt.Errorf("scan model_price: %w", err)
		}
		if err := m.pgExec(
			`INSERT INTO model_prices (model, input_per_1m, output_per_1m, updated_at) VALUES ($1, $2, $3, $4)`,
			model, inputPer1M, outputPer1M, nullTime(updated),
		); err != nil {
			return fmt.Errorf("insert model_price %s: %w", model, err)
		}
		src++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read model_prices: %w", err)
	}
	return m.record("model_prices", src, "model_prices")
}

// --- Skills (special: mapped, not inserted) ---

// skills builds a map from SQLite skill id -> Postgres skill id by matching the
// natural `key`. The Postgres skills table is already seeded by NewPostgres with
// the same key set (but possibly different auto-assigned ids), so we do NOT
// re-insert matched keys — we only insert a skill whose key is somehow missing
// downstream. The returned map is used to translate user_skills.skill_id.
func (m *etl) skills() (map[int64]int64, error) {
	// SQLite skills (full row, so we can insert any missing key).
	type sqliteSkill struct {
		id             int64
		key            string
		name           string
		description    string
		prompt         string
		category       string
		defaultEnabled int64
		sortOrder      int64
	}
	rows, err := m.src.QueryContext(m.ctx,
		`SELECT id, key, name, description, prompt, category, default_enabled, sort_order FROM skills`)
	if err != nil {
		return nil, fmt.Errorf("read sqlite skills: %w", err)
	}
	defer rows.Close()

	var sqliteSkills []sqliteSkill
	for rows.Next() {
		var s sqliteSkill
		if err := rows.Scan(&s.id, &s.key, &s.name, &s.description, &s.prompt, &s.category, &s.defaultEnabled, &s.sortOrder); err != nil {
			return nil, fmt.Errorf("scan sqlite skill: %w", err)
		}
		sqliteSkills = append(sqliteSkills, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read sqlite skills: %w", err)
	}

	// Postgres skills already seeded: key -> id.
	pgByKey, err := m.pgSkillKeys()
	if err != nil {
		return nil, err
	}

	idMap := make(map[int64]int64, len(sqliteSkills))
	for _, s := range sqliteSkills {
		pgID, ok := pgByKey[s.key]
		if !ok {
			// Key not seeded in Postgres — insert it (natural columns only), then
			// re-read its assigned id. Normally this branch is never taken.
			if err := m.pgExec(
				`INSERT INTO skills (key, name, description, prompt, category, default_enabled, sort_order)
				 VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT (key) DO NOTHING`,
				s.key, s.name, s.description, s.prompt, s.category, boolFromInt(s.defaultEnabled), s.sortOrder,
			); err != nil {
				return nil, fmt.Errorf("insert missing skill %s: %w", s.key, err)
			}
			if err := m.dst.PostgresStore.pool.QueryRow(m.ctx,
				`SELECT id FROM skills WHERE key = $1`, s.key).Scan(&pgID); err != nil {
				return nil, fmt.Errorf("reread skill %s: %w", s.key, err)
			}
		}
		idMap[s.id] = pgID
	}

	pgCount, err := m.pgCount("skills")
	if err != nil {
		return nil, err
	}
	m.report.Counts["skills"] = [2]int{len(sqliteSkills), pgCount}
	return idMap, nil
}

func (m *etl) pgSkillKeys() (map[string]int64, error) {
	rows, err := m.dst.PostgresStore.pool.Query(m.ctx, `SELECT id, key FROM skills`)
	if err != nil {
		return nil, fmt.Errorf("read postgres skills: %w", err)
	}
	defer rows.Close()

	out := map[string]int64{}
	for rows.Next() {
		var id int64
		var key string
		if err := rows.Scan(&id, &key); err != nil {
			return nil, fmt.Errorf("scan postgres skill: %w", err)
		}
		out[key] = id
	}
	return out, rows.Err()
}

func (m *etl) userSkills(skillMap map[int64]int64) error {
	rows, err := m.src.QueryContext(m.ctx, `SELECT user_id, skill_id, enabled FROM user_skills`)
	if err != nil {
		return fmt.Errorf("read user_skills: %w", err)
	}
	defer rows.Close()

	src := 0
	for rows.Next() {
		var (
			userID, skillID int64
			enabled         int64
		)
		if err := rows.Scan(&userID, &skillID, &enabled); err != nil {
			return fmt.Errorf("scan user_skill: %w", err)
		}
		pgSkillID, ok := skillMap[skillID]
		if !ok {
			return fmt.Errorf("user_skill references unknown skill id %d", skillID)
		}
		if err := m.pgExec(
			`INSERT INTO user_skills (user_id, skill_id, enabled) VALUES ($1, $2, $3)`,
			userID, pgSkillID, boolFromInt(enabled),
		); err != nil {
			return fmt.Errorf("insert user_skill (%d,%d): %w", userID, skillID, err)
		}
		src++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read user_skills: %w", err)
	}
	return m.record("user_skills", src, "user_skills")
}

// --- Mongo log collections ---

func (m *etl) messageLog() error {
	rows, err := m.src.QueryContext(m.ctx,
		`SELECT id, user_id, platform, direction, sender, body, intent, action, created_at FROM message_log`)
	if err != nil {
		return fmt.Errorf("read message_log: %w", err)
	}
	defer rows.Close()

	var docs []any
	var maxID int64
	for rows.Next() {
		var (
			d       mongoMessageLog
			created sql.NullTime
		)
		if err := rows.Scan(&d.ID, &d.UserID, &d.Platform, &d.Direction, &d.Sender, &d.Body, &d.Intent, &d.Action, &created); err != nil {
			return fmt.Errorf("scan message_log: %w", err)
		}
		if created.Valid {
			d.CreatedAt = created.Time.UTC()
		}
		if d.ID > maxID {
			maxID = d.ID
		}
		docs = append(docs, d)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read message_log: %w", err)
	}
	if err := insertMany(m.ctx, m.mongoCol(colMessageLog), docs); err != nil {
		return fmt.Errorf("insert message_log: %w", err)
	}
	if err := m.seedCounter(colMessageLog, maxID); err != nil {
		return err
	}
	return m.recordMongo("message_log", len(docs), colMessageLog)
}

func (m *etl) toolUsage() error {
	// tool_usage has no id in either backend, so no counter seeding.
	rows, err := m.src.QueryContext(m.ctx,
		`SELECT user_id, tool, platform, created_at FROM tool_usage`)
	if err != nil {
		return fmt.Errorf("read tool_usage: %w", err)
	}
	defer rows.Close()

	var docs []any
	for rows.Next() {
		var (
			userID         int64
			tool, platform string
			created        sql.NullTime
		)
		if err := rows.Scan(&userID, &tool, &platform, &created); err != nil {
			return fmt.Errorf("scan tool_usage: %w", err)
		}
		doc := bson.M{"user_id": userID, "tool": tool, "platform": platform}
		if created.Valid {
			doc["created_at"] = created.Time.UTC()
		}
		docs = append(docs, doc)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read tool_usage: %w", err)
	}
	if err := insertMany(m.ctx, m.mongoCol(colToolUsage), docs); err != nil {
		return fmt.Errorf("insert tool_usage: %w", err)
	}
	return m.recordMongo("tool_usage", len(docs), colToolUsage)
}

func (m *etl) activities() error {
	rows, err := m.src.QueryContext(m.ctx,
		`SELECT id, user_id, type, description, occurred_at, source, created_at FROM activities`)
	if err != nil {
		return fmt.Errorf("read activities: %w", err)
	}
	defer rows.Close()

	var docs []any
	var maxID int64
	for rows.Next() {
		var (
			d                 mongoActivity
			occurred, created sql.NullTime
		)
		if err := rows.Scan(&d.ID, &d.UserID, &d.Type, &d.Description, &occurred, &d.Source, &created); err != nil {
			return fmt.Errorf("scan activity: %w", err)
		}
		if occurred.Valid {
			d.OccurredAt = occurred.Time.UTC()
		}
		if created.Valid {
			d.CreatedAt = created.Time.UTC()
		}
		if d.ID > maxID {
			maxID = d.ID
		}
		docs = append(docs, d)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read activities: %w", err)
	}
	if err := insertMany(m.ctx, m.mongoCol(colActivities), docs); err != nil {
		return fmt.Errorf("insert activities: %w", err)
	}
	if err := m.seedCounter(colActivities, maxID); err != nil {
		return err
	}
	return m.recordMongo("activities", len(docs), colActivities)
}

// traces migrates the traces table into Mongo, folding each trace's
// trace_scores row into the embedded `score` sub-document (nil when unscored).
func (m *etl) traces() error {
	scores, err := m.traceScores()
	if err != nil {
		return err
	}

	rows, err := m.src.QueryContext(m.ctx, `SELECT
		id, user_id, platform, input, output, model, prompt_tokens, completion_tokens, total_tokens,
		latency_ms, tool_count, tools_json, skills, steps_json, status, error, created_at
		FROM traces`)
	if err != nil {
		return fmt.Errorf("read traces: %w", err)
	}
	defer rows.Close()

	var docs []any
	var maxID int64
	for rows.Next() {
		var (
			d                               mongoTrace
			toolsJSON, skillsCSV, stepsJSON string
			created                         sql.NullTime
		)
		if err := rows.Scan(&d.ID, &d.UserID, &d.Platform, &d.Input, &d.Output, &d.Model,
			&d.PromptTokens, &d.CompletionTokens, &d.TotalTokens, &d.LatencyMs, &d.ToolCount,
			&toolsJSON, &skillsCSV, &stepsJSON, &d.Status, &d.Error, &created); err != nil {
			return fmt.Errorf("scan trace: %w", err)
		}
		if created.Valid {
			d.CreatedAt = created.Time.UTC()
		}
		if toolsJSON != "" {
			if err := json.Unmarshal([]byte(toolsJSON), &d.Tools); err != nil {
				return fmt.Errorf("parse tools_json for trace %d: %w", d.ID, err)
			}
		}
		if stepsJSON != "" {
			if err := json.Unmarshal([]byte(stepsJSON), &d.Steps); err != nil {
				return fmt.Errorf("parse steps_json for trace %d: %w", d.ID, err)
			}
		}
		if skillsCSV != "" {
			d.Skills = strings.Split(skillsCSV, ",")
		}
		if sc, ok := scores[d.ID]; ok {
			s := sc
			d.Score = &s
		}
		if d.ID > maxID {
			maxID = d.ID
		}
		docs = append(docs, d)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read traces: %w", err)
	}
	if err := insertMany(m.ctx, m.mongoCol(colTraces), docs); err != nil {
		return fmt.Errorf("insert traces: %w", err)
	}
	if err := m.seedCounter(colTraces, maxID); err != nil {
		return err
	}
	return m.recordMongo("traces", len(docs), colTraces)
}

// traceScores loads every trace_scores row keyed by trace_id so traces() can
// embed them without a per-trace query.
func (m *etl) traceScores() (map[int64]mongoScore, error) {
	rows, err := m.src.QueryContext(m.ctx,
		`SELECT trace_id, accuracy, helpfulness, safety, overall, rationale, judge_model, created_at FROM trace_scores`)
	if err != nil {
		return nil, fmt.Errorf("read trace_scores: %w", err)
	}
	defer rows.Close()

	out := map[int64]mongoScore{}
	for rows.Next() {
		var (
			sc      mongoScore
			created sql.NullTime
		)
		if err := rows.Scan(&sc.TraceID, &sc.Accuracy, &sc.Helpfulness, &sc.Safety, &sc.Overall, &sc.Rationale, &sc.JudgeModel, &created); err != nil {
			return nil, fmt.Errorf("scan trace_score: %w", err)
		}
		if created.Valid {
			sc.CreatedAt = created.Time.UTC()
		}
		out[sc.TraceID] = sc
	}
	return out, rows.Err()
}

// seedCounter upserts the Mongo counters document for a collection so future
// nextSeq calls continue past the largest migrated id.
func (m *etl) seedCounter(col string, maxID int64) error {
	_, err := m.mongoCol(colCounters).UpdateOne(m.ctx,
		bson.M{"_id": col},
		bson.M{"$set": bson.M{"seq": maxID}},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("seed counter %s: %w", col, err)
	}
	return nil
}

// --- helpers ---

// record captures a Postgres table's [source, dest] counts into the report.
func (m *etl) record(name string, src int, table string) error {
	dst, err := m.pgCount(table)
	if err != nil {
		return err
	}
	m.report.Counts[name] = [2]int{src, dst}
	return nil
}

// recordMongo captures a Mongo collection's [source, dest] counts.
func (m *etl) recordMongo(name string, src int, col string) error {
	dst, err := m.mongoCount(col)
	if err != nil {
		return err
	}
	m.report.Counts[name] = [2]int{src, dst}
	return nil
}

// nullTime returns the underlying time (for a valid sql.NullTime) or nil, so
// pgx writes a real timestamp or SQL NULL respectively.
func nullTime(t sql.NullTime) any {
	if t.Valid {
		return t.Time.UTC()
	}
	return nil
}

// insertMany inserts a batch of documents, tolerating an empty batch.
func insertMany(ctx context.Context, col *mongo.Collection, docs []any) error {
	if len(docs) == 0 {
		return nil
	}
	_, err := col.InsertMany(ctx, docs)
	return err
}
