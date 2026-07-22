package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/authctx"
	"github.com/jackc/pgx/v5"
)

func (s *PostgresStore) ListMountains(ctx context.Context, userID int64) ([]Mountain, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name FROM hike_mountains WHERE user_id = $1 AND ($2 = 0 OR project_id = $2) ORDER BY name ASC`, userID, authctx.ProjectID(ctx))
	if err != nil {
		return nil, fmt.Errorf("list mountains: %w", err)
	}
	defer rows.Close()
	var out []Mountain
	for rows.Next() {
		var m Mountain
		if err := rows.Scan(&m.ID, &m.Name); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *PostgresStore) CreateMountain(ctx context.Context, userID int64, name string) (*Mountain, error) {
	var id int64
	err := s.pool.QueryRow(ctx,
		`INSERT INTO hike_mountains (user_id, project_id, name, created_at) VALUES ($1, $2, $3, $4) RETURNING id`,
		userID, authctx.ProjectID(ctx), name, time.Now().UTC()).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("create mountain: %w", err)
	}
	return &Mountain{ID: id, Name: name}, nil
}

func (s *PostgresStore) ListTracks(ctx context.Context, userID, mountainID int64) ([]HikeTrack, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, mountain_id, name FROM hike_tracks WHERE user_id = $1 AND mountain_id = $2 AND ($3 = 0 OR project_id = $3) ORDER BY name ASC`,
		userID, mountainID, authctx.ProjectID(ctx))
	if err != nil {
		return nil, fmt.Errorf("list tracks: %w", err)
	}
	defer rows.Close()
	var out []HikeTrack
	for rows.Next() {
		var t HikeTrack
		if err := rows.Scan(&t.ID, &t.MountainID, &t.Name); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *PostgresStore) CreateTrack(ctx context.Context, userID, mountainID int64, name string) (*HikeTrack, error) {
	var id int64
	err := s.pool.QueryRow(ctx,
		`INSERT INTO hike_tracks (user_id, project_id, mountain_id, name, created_at) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		userID, authctx.ProjectID(ctx), mountainID, name, time.Now().UTC()).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("create track: %w", err)
	}
	return &HikeTrack{ID: id, MountainID: mountainID, Name: name}, nil
}

func (s *PostgresStore) ListHikers(ctx context.Context, userID int64) ([]Hiker, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, nicknames FROM hike_participants WHERE user_id = $1 AND ($2 = 0 OR project_id = $2) ORDER BY name ASC`, userID, authctx.ProjectID(ctx))
	if err != nil {
		return nil, fmt.Errorf("list hikers: %w", err)
	}
	defer rows.Close()
	var out []Hiker
	for rows.Next() {
		var h Hiker
		if err := rows.Scan(&h.ID, &h.Name, &h.Nicknames); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *PostgresStore) CreateHiker(ctx context.Context, userID int64, name string) (*Hiker, error) {
	var id int64
	err := s.pool.QueryRow(ctx,
		`INSERT INTO hike_participants (user_id, project_id, name, created_at) VALUES ($1, $2, $3, $4) RETURNING id`,
		userID, authctx.ProjectID(ctx), name, time.Now().UTC()).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("create hiker: %w", err)
	}
	return &Hiker{ID: id, Name: name}, nil
}

func (s *PostgresStore) UpdateHiker(ctx context.Context, userID, id int64, name string, nicknames []string) (*Hiker, error) {
	if nicknames == nil {
		nicknames = []string{}
	}
	var h Hiker
	err := s.pool.QueryRow(ctx,
		`UPDATE hike_participants SET name = $1, nicknames = $2
		 WHERE id = $3 AND user_id = $4 AND ($5 = 0 OR project_id = $5)
		 RETURNING id, name, nicknames`,
		strings.TrimSpace(name), nicknames, id, userID, authctx.ProjectID(ctx)).
		Scan(&h.ID, &h.Name, &h.Nicknames)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update hiker: %w", err)
	}
	return &h, nil
}

func (s *PostgresStore) MergeHikers(ctx context.Context, userID, targetID, sourceID int64) (*Hiker, error) {
	projectID := authctx.ProjectID(ctx)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("merge hikers: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	// Load both participants (scoped to the user/project) so we can fold the
	// source's name + nicknames into the target and confirm ownership.
	var target, source Hiker
	if err := tx.QueryRow(ctx,
		`SELECT id, name, nicknames FROM hike_participants
		 WHERE id = $1 AND user_id = $2 AND ($3 = 0 OR project_id = $3)`, targetID, userID, projectID).
		Scan(&target.ID, &target.Name, &target.Nicknames); err != nil {
		return nil, fmt.Errorf("merge hikers: load target: %w", err)
	}
	if err := tx.QueryRow(ctx,
		`SELECT id, name, nicknames FROM hike_participants
		 WHERE id = $1 AND user_id = $2 AND ($3 = 0 OR project_id = $3)`, sourceID, userID, projectID).
		Scan(&source.ID, &source.Name, &source.Nicknames); err != nil {
		return nil, fmt.Errorf("merge hikers: load source: %w", err)
	}

	// Reassign the source's hike links to the target, skipping hikes the target
	// is already on (the join table's PK forbids duplicate (hike, participant)),
	// then drop any leftover source links.
	if _, err := tx.Exec(ctx,
		`UPDATE hike_hikers SET participant_id = $1
		 WHERE participant_id = $2
		   AND hike_id NOT IN (SELECT hike_id FROM hike_hikers WHERE participant_id = $1)`,
		targetID, sourceID); err != nil {
		return nil, fmt.Errorf("merge hikers: reassign hikes: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM hike_hikers WHERE participant_id = $1`, sourceID); err != nil {
		return nil, fmt.Errorf("merge hikers: drop source links: %w", err)
	}

	// Fold the absorbed name + its nicknames into the target's nickname list.
	merged := mergeNicknames(target.Name, target.Nicknames, source.Name, source.Nicknames)
	if _, err := tx.Exec(ctx,
		`UPDATE hike_participants SET nicknames = $1 WHERE id = $2`, merged, targetID); err != nil {
		return nil, fmt.Errorf("merge hikers: update nicknames: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM hike_participants WHERE id = $1`, sourceID); err != nil {
		return nil, fmt.Errorf("merge hikers: delete source: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("merge hikers: commit: %w", err)
	}
	target.Nicknames = merged
	return &target, nil
}

// mergeNicknames combines the target's existing nicknames with the absorbed
// source name and its nicknames, de-duplicated case-insensitively and excluding
// any value equal to the target's own canonical name. Returns a non-nil slice so
// the NOT NULL nicknames column is written as '{}' rather than NULL.
func mergeNicknames(targetName string, targetNicks []string, sourceName string, sourceNicks []string) []string {
	seen := map[string]bool{strings.ToLower(strings.TrimSpace(targetName)): true}
	out := []string{}
	add := func(vals ...string) {
		for _, v := range vals {
			key := strings.ToLower(strings.TrimSpace(v))
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, strings.TrimSpace(v))
		}
	}
	add(targetNicks...)
	add(sourceName)
	add(sourceNicks...)
	return out
}

func (s *PostgresStore) CreateHike(ctx context.Context, userID int64, h *Hike) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx,
		`INSERT INTO hikes (user_id, project_id, mountain_id, camped, up_track_id, down_track_id, days, nights, hiked_on, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING id`,
		userID, authctx.ProjectID(ctx), h.MountainID, h.Camped, h.UpTrackID, h.DownTrackID, h.Days, h.Nights, h.HikedOn.UTC(), time.Now().UTC()).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create hike: %w", err)
	}
	return id, nil
}

func (s *PostgresStore) AddHikeParticipant(ctx context.Context, hikeID, hikerID int64) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO hike_hikers (hike_id, participant_id) VALUES ($1, $2)
		 ON CONFLICT (hike_id, participant_id) DO NOTHING`, hikeID, hikerID)
	return err
}

// ClearHikeParticipants removes every participant link from a hike, so the UI
// can re-attach the edited set on an update.
func (s *PostgresStore) ClearHikeParticipants(ctx context.Context, hikeID int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM hike_hikers WHERE hike_id = $1`, hikeID)
	return err
}

// UpdateHike edits a logged hike's core fields (participants are re-synced
// separately via ClearHikeParticipants + AddHikeParticipant). Scoped to the
// owning user and active project so one project can't edit another's hike.
func (s *PostgresStore) UpdateHike(ctx context.Context, userID, id int64, h *Hike) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE hikes SET mountain_id = $1, camped = $2, up_track_id = $3, down_track_id = $4,
		        days = $5, nights = $6, hiked_on = $7
		 WHERE id = $8 AND user_id = $9 AND ($10 = 0 OR project_id = $10)`,
		h.MountainID, h.Camped, h.UpTrackID, h.DownTrackID, h.Days, h.Nights, h.HikedOn.UTC(),
		id, userID, authctx.ProjectID(ctx))
	if err != nil {
		return fmt.Errorf("update hike: %w", err)
	}
	return nil
}

// DeleteHike removes a logged hike (and its participant links) for the owning
// user and active project. The participant links are cleared only after the
// project-scoped hike row is confirmed deleted, so a caller can never strand
// another project's junction rows.
func (s *PostgresStore) DeleteHike(ctx context.Context, userID, id int64) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM hikes WHERE id = $1 AND user_id = $2 AND ($3 = 0 OR project_id = $3)`,
		id, userID, authctx.ProjectID(ctx))
	if err != nil {
		return fmt.Errorf("delete hike: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	if _, err := s.pool.Exec(ctx, `DELETE FROM hike_hikers WHERE hike_id = $1`, id); err != nil {
		return fmt.Errorf("delete hike participants: %w", err)
	}
	return nil
}

// GetHike returns one of the user's logged hikes by id (with mountain, track,
// and participant detail), or (nil, nil) when no row matches. Used to confirm a
// just-logged hike actually persisted.
func (s *PostgresStore) GetHike(ctx context.Context, userID, id int64) (*HikeDetail, error) {
	var d HikeDetail
	var participants string
	err := s.pool.QueryRow(ctx,
		`SELECT h.id, h.mountain_id, h.camped, h.up_track_id, h.down_track_id, h.days, h.nights, h.hiked_on,
		        m.name, COALESCE(ut.name, ''), COALESCE(dt.name, ''),
		        COALESCE((SELECT string_agg(p.name, ', ') FROM hike_hikers hh
		                  JOIN hike_participants p ON p.id = hh.participant_id
		                  WHERE hh.hike_id = h.id), '')
		 FROM hikes h
		 JOIN hike_mountains m ON m.id = h.mountain_id
		 LEFT JOIN hike_tracks ut ON ut.id = h.up_track_id
		 LEFT JOIN hike_tracks dt ON dt.id = h.down_track_id
		 WHERE h.user_id = $1 AND h.id = $2 AND ($3 = 0 OR h.project_id = $3)`, userID, id, authctx.ProjectID(ctx)).
		Scan(&d.ID, &d.MountainID, &d.Camped, &d.UpTrackID, &d.DownTrackID, &d.Days, &d.Nights, &d.HikedOn,
			&d.Mountain, &d.UpTrack, &d.DownTrack, &participants)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get hike: %w", err)
	}
	if participants != "" {
		d.Participants = strings.Split(participants, ", ")
	}
	return &d, nil
}

func (s *PostgresStore) ListHikes(ctx context.Context, userID int64, limit int) ([]HikeDetail, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx,
		`SELECT h.id, h.mountain_id, h.camped, h.up_track_id, h.down_track_id, h.days, h.nights, h.hiked_on,
		        m.name, COALESCE(ut.name, ''), COALESCE(dt.name, ''),
		        COALESCE((SELECT string_agg(p.name, ', ') FROM hike_hikers hh
		                  JOIN hike_participants p ON p.id = hh.participant_id
		                  WHERE hh.hike_id = h.id), '')
		 FROM hikes h
		 JOIN hike_mountains m ON m.id = h.mountain_id
		 LEFT JOIN hike_tracks ut ON ut.id = h.up_track_id
		 LEFT JOIN hike_tracks dt ON dt.id = h.down_track_id
		 WHERE h.user_id = $1 AND ($2 = 0 OR h.project_id = $2)
		 ORDER BY h.hiked_on DESC LIMIT $3`, userID, authctx.ProjectID(ctx), limit)
	if err != nil {
		return nil, fmt.Errorf("list hikes: %w", err)
	}
	defer rows.Close()

	var out []HikeDetail
	for rows.Next() {
		var d HikeDetail
		var participants string
		if err := rows.Scan(&d.ID, &d.MountainID, &d.Camped, &d.UpTrackID, &d.DownTrackID, &d.Days, &d.Nights, &d.HikedOn,
			&d.Mountain, &d.UpTrack, &d.DownTrack, &participants); err != nil {
			return nil, fmt.Errorf("scan hike: %w", err)
		}
		if participants != "" {
			d.Participants = strings.Split(participants, ", ")
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
