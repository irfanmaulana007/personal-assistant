package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/irfanmaulana007/personal-assistant/server/internal/authctx"
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
		`SELECT id, name FROM hike_participants WHERE user_id = $1 AND ($2 = 0 OR project_id = $2) ORDER BY name ASC`, userID, authctx.ProjectID(ctx))
	if err != nil {
		return nil, fmt.Errorf("list hikers: %w", err)
	}
	defer rows.Close()
	var out []Hiker
	for rows.Next() {
		var h Hiker
		if err := rows.Scan(&h.ID, &h.Name); err != nil {
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
