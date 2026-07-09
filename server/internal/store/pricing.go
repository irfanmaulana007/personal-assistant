package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (s *SQLiteStore) ListModelPrices(ctx context.Context) ([]ModelPrice, error) {
	rows, err := s.db.QueryContext(ctx,
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

func (s *SQLiteStore) UpsertModelPrice(ctx context.Context, p ModelPrice) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO model_prices (model, input_per_1m, output_per_1m, updated_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(model) DO UPDATE SET
		   input_per_1m = excluded.input_per_1m,
		   output_per_1m = excluded.output_per_1m,
		   updated_at = excluded.updated_at`,
		strings.TrimSpace(p.Model), p.InputPer1M, p.OutputPer1M, time.Now().UTC())
	return err
}

func (s *SQLiteStore) DeleteModelPrice(ctx context.Context, model string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM model_prices WHERE model = ?`, strings.TrimSpace(model))
	return err
}
