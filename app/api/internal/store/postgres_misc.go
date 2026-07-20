package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// --- OAuth Tokens ---

func (s *PostgresStore) SaveToken(ctx context.Context, service string, tokenData []byte) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO oauth_tokens (service, token_data, updated_at) VALUES ($1, $2, $3)
		 ON CONFLICT(service) DO UPDATE SET token_data = excluded.token_data, updated_at = excluded.updated_at`,
		service, tokenData, time.Now().UTC(),
	)
	return err
}

func (s *PostgresStore) GetToken(ctx context.Context, service string) ([]byte, error) {
	var data []byte
	err := s.pool.QueryRow(ctx,
		`SELECT token_data FROM oauth_tokens WHERE service = $1`, service,
	).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return data, err
}

// --- Settings ---

func (s *PostgresStore) GetSetting(ctx context.Context, key string) ([]byte, error) {
	var value []byte
	err := s.pool.QueryRow(ctx,
		`SELECT value FROM settings WHERE key = $1`, key,
	).Scan(&value)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return value, err
}

func (s *PostgresStore) SetSetting(ctx context.Context, key string, value []byte) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO settings (key, value, updated_at) VALUES ($1, $2, $3)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, time.Now().UTC(),
	)
	return err
}

func (s *PostgresStore) GetAllSettings(ctx context.Context) (map[string][]byte, error) {
	rows, err := s.pool.Query(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, fmt.Errorf("get all settings: %w", err)
	}
	defer rows.Close()

	settings := make(map[string][]byte)
	for rows.Next() {
		var key string
		var value []byte
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("scan setting: %w", err)
		}
		settings[key] = value
	}
	return settings, rows.Err()
}

// --- Model Prices ---

func (s *PostgresStore) ListModelPrices(ctx context.Context) ([]ModelPrice, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT model, input_per_1m, output_per_1m FROM model_prices ORDER BY model ASC`)
	if err != nil {
		return nil, fmt.Errorf("list model prices: %w", err)
	}
	defer rows.Close()

	var out []ModelPrice
	for rows.Next() {
		var p ModelPrice
		if err := rows.Scan(&p.Model, &p.InputPer1M, &p.OutputPer1M); err != nil {
			return nil, fmt.Errorf("scan model price: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *PostgresStore) UpsertModelPrice(ctx context.Context, p ModelPrice) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO model_prices (model, input_per_1m, output_per_1m, updated_at)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT(model) DO UPDATE SET
		   input_per_1m = excluded.input_per_1m,
		   output_per_1m = excluded.output_per_1m,
		   updated_at = excluded.updated_at`,
		strings.TrimSpace(p.Model), p.InputPer1M, p.OutputPer1M, time.Now().UTC())
	return err
}

func (s *PostgresStore) DeleteModelPrice(ctx context.Context, model string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM model_prices WHERE model = $1`, strings.TrimSpace(model))
	return err
}
