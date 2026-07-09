// Package pricing resolves per-model token costs, letting DB-stored rates
// override the built-in defaults in the llm package.
package pricing

import (
	"context"
	"strings"
	"sync"

	"github.com/irfanmaulana007/personal-assistant/server/internal/llm"
	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

// EffectivePrice is a resolved rate for a model, noting whether it came from a
// user override ("custom") or the built-in table ("builtin").
type EffectivePrice struct {
	Model       string  `json:"model"`
	InputPer1M  float64 `json:"input_per_1m"`
	OutputPer1M float64 `json:"output_per_1m"`
	Source      string  `json:"source"` // "custom" | "builtin"
}

// Service overlays DB model prices on top of the built-in defaults.
type Service struct {
	store store.Store
	mu    sync.RWMutex
	db    map[string]llm.ModelPrice // trimmed model -> override
}

// New builds the service and loads the current overrides.
func New(s store.Store) *Service {
	svc := &Service{store: s, db: map[string]llm.ModelPrice{}}
	_ = svc.Reload(context.Background())
	return svc
}

// Reload refreshes the DB override cache.
func (s *Service) Reload(ctx context.Context) error {
	list, err := s.store.ListModelPrices(ctx)
	if err != nil {
		return err
	}
	m := make(map[string]llm.ModelPrice, len(list))
	for _, p := range list {
		m[strings.TrimSpace(p.Model)] = llm.ModelPrice{InputPer1M: p.InputPer1M, OutputPer1M: p.OutputPer1M}
	}
	s.mu.Lock()
	s.db = m
	s.mu.Unlock()
	return nil
}

// Estimate returns the cost and whether a rate was known. DB overrides win;
// otherwise it falls back to the built-in llm table.
func (s *Service) Estimate(model string, promptTokens, completionTokens int) (float64, bool) {
	s.mu.RLock()
	p, ok := s.db[strings.TrimSpace(model)]
	s.mu.RUnlock()
	if ok {
		return llm.CostFor(p, promptTokens, completionTokens), true
	}
	return llm.EstimateCost(model, promptTokens, completionTokens)
}

// Effective returns the merged view (built-ins overlaid with overrides), sorted
// by model, for the settings UI.
func (s *Service) Effective(ctx context.Context) []EffectivePrice {
	merged := map[string]EffectivePrice{}
	for model, p := range llm.DefaultPrices() {
		merged[model] = EffectivePrice{Model: model, InputPer1M: p.InputPer1M, OutputPer1M: p.OutputPer1M, Source: "builtin"}
	}
	if list, err := s.store.ListModelPrices(ctx); err == nil {
		for _, p := range list {
			merged[strings.TrimSpace(p.Model)] = EffectivePrice{
				Model: strings.TrimSpace(p.Model), InputPer1M: p.InputPer1M, OutputPer1M: p.OutputPer1M, Source: "custom",
			}
		}
	}
	out := make([]EffectivePrice, 0, len(merged))
	for _, p := range merged {
		out = append(out, p)
	}
	return out
}
